//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestLaunchProvidesWritablePrivateHomeForAgentTools(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("launching as the Agent user requires root")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("Go is required for the Agent compilation check")
	}
	base, err := os.MkdirTemp("", "agent-home-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	if err := os.Chmod(base, 0o755); err != nil {
		t.Fatal(err)
	}
	runnerHome := filepath.Join(base, "runner-home")
	if err := os.Mkdir(runnerHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(runnerHome, runnerUID, runnerGID); err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(base, "cache")
	if err := os.Mkdir(cacheRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(cacheRoot, agentUID, agentGID); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_RUNNER_CACHE_ROOT", cacheRoot)
	for name, value := range map[string]string{
		"HOME": runnerHome, "USER": "runner", "LOGNAME": "runner",
		"XDG_CACHE_HOME": filepath.Join(runnerHome, ".cache"),
		"GOCACHE":        filepath.Join(runnerHome, ".cache", "go-build"),
		"GOPATH":         filepath.Join(runnerHome, "go"),
		"GOMODCACHE":     filepath.Join(runnerHome, "go", "pkg", "mod"),
		"GO111MODULE":    "on", "GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local", "GOWORK": "off",
		"COORDINATOR_RUN_TOKEN": "fixture-token",
	} {
		t.Setenv(name, value)
	}
	previousBinaries := allowedBinaries
	shellPath, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	shellData, err := os.ReadFile(shellPath)
	if err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(base, "sh")
	if err := os.WriteFile(shell, shellData, 0o755); err != nil {
		t.Fatal(err)
	}
	allowedBinaries = []string{shell}
	t.Cleanup(func() { allowedBinaries = previousBinaries })
	if err := os.MkdirAll(filepath.Dir(agentRunLockPath), 0o700); err != nil {
		t.Fatal(err)
	}
	workspaceRoot := filepath.Join(base, "workspaces")
	for _, tc := range []struct {
		name string
		code int
	}{
		{name: "success"},
		{name: "failure", code: 42},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runRoot := filepath.Join(workspaceRoot, "runs", tc.name)
			module := filepath.Join(runRoot, "repos", "smoke")
			if err := os.MkdirAll(module, 0o755); err != nil {
				t.Fatal(err)
			}
			for name, content := range map[string]string{
				"go.mod":        "module smoke\n\ngo 1.22\n",
				"smoke.go":      "package smoke\nfunc sum(a, b int) int { return a + b }\n",
				"smoke_test.go": "package smoke\nimport \"testing\"\nfunc TestSum(t *testing.T) { if sum(2, 3) != 5 { t.Fatal(\"wrong sum\") } }\n",
			} {
				if err := os.WriteFile(filepath.Join(module, name), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			script := `set -eu
printf '%s\n' "$HOME" > agent-home.path
if [ "$1" = success ]; then (cd repos/smoke && go test -count=1 ./...); fi
test "$(id -u)" = 10002
test "$(id -g)" = 10002
test "$USER" = agent
test "$LOGNAME" = agent
test "$COORDINATOR_RUN_TOKEN" = fixture-token
test "$(stat -c %a "$HOME")" = 700
case "$HOME" in "$PWD"/*) ;; *) exit 1 ;; esac
for directory in "$HOME" "$XDG_CACHE_HOME"; do
    case "$directory" in "$HOME"|"$HOME"/*) ;; *) exit 1 ;; esac
    mkdir -p "$directory"
    printf check > "$directory/write-check"
done
for directory in "$GOCACHE" "$GOPATH" "$GOMODCACHE"; do
    case "$directory" in "$AGENT_RUNNER_CACHE_ROOT"/*) ;; *) exit 1 ;; esac
    mkdir -p "$directory"
    printf check > "$directory/write-check"
done
exit "$2"
`
			code, err := launch([]string{"--workspace-root", workspaceRoot, "--workspace", runRoot, "--", shell, "-c", script, "agent-home-check", tc.name, strconv.Itoa(tc.code)})
			if err != nil || code != tc.code {
				t.Fatalf("Agent tool execution = %d, %v; want %d, nil", code, err, tc.code)
			}
			data, err := os.ReadFile(filepath.Join(runRoot, "agent-home.path"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(strings.TrimSpace(string(data))); !os.IsNotExist(err) {
				t.Fatalf("Agent home must be removed after execution: %v", err)
			}
			assertOwner(t, runRoot, runnerUID, runnerGID)
			if modeOf(t, runRoot) != 0o700 {
				t.Fatal("completed run workspace must remain private to Runner")
			}
		})
	}
}

func TestResolveRunWorkspaceRequiresDirectRealRunDirectory(t *testing.T) {
	root := t.TempDir()
	runRoot := filepath.Join(root, "runs", "run-1")
	if err := os.MkdirAll(runRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := resolveRunWorkspace(root, runRoot)
	if err != nil || got != runRoot {
		t.Fatalf("resolve run workspace = %q, %v; want %q, nil", got, err, runRoot)
	}

	for _, workspace := range []string{
		filepath.Join(root, "runs"),
		filepath.Join(root, "outside"),
		filepath.Join(runRoot, "nested"),
	} {
		if _, err := resolveRunWorkspace(root, workspace); err == nil {
			t.Errorf("resolveRunWorkspace(%q) unexpectedly succeeded", workspace)
		}
	}
}

func TestProcessIdentityIgnoresExitedAgentAndOtherUsers(t *testing.T) {
	for _, tc := range []struct {
		status string
		uid    int
		zombie bool
	}{
		{status: "State:\tS (sleeping)\nUid:\t10002\t10002\t10002\t10002\n", uid: agentUID},
		{status: "State:\tZ (zombie)\nUid:\t10002\t10002\t10002\t10002\n", uid: agentUID, zombie: true},
		{status: "State:\tS (sleeping)\nUid:\t10001\t10001\t10001\t10001\n", uid: runnerUID},
	} {
		uid, zombie := processIdentity([]byte(tc.status))
		if uid != tc.uid || zombie != tc.zombie {
			t.Fatalf("process identity = %d, %t; want %d, %t", uid, zombie, tc.uid, tc.zombie)
		}
	}
}

func TestResolveRunWorkspaceRejectsSymlinkedRun(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "runs"), 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "runs", "run-1")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("create symlink: %v", err)
	}
	if _, err := resolveRunWorkspace(root, link); err == nil {
		t.Fatal("resolveRunWorkspace accepted a symlinked run directory")
	}
}

func TestChownTreeChangesRegularFilesAndSymlinksWithoutFollowingThem(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "private.json")
	if err := os.WriteFile(file, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("create symlink: %v", err)
	}

	uid, gid := os.Getuid(), os.Getgid()
	if os.Geteuid() == 0 {
		uid++
		gid++
	}
	if err := chownTree(root, uid, gid); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{root, file, link} {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || int(stat.Uid) != uid || int(stat.Gid) != gid {
			t.Errorf("owner(%q) = %v; want %d:%d", path, info.Sys(), uid, gid)
		}
	}
	outsideInfo, err := os.Stat(outside)
	if err != nil {
		t.Fatal(err)
	}
	outsideStat, ok := outsideInfo.Sys().(*syscall.Stat_t)
	if !ok || int(outsideStat.Uid) != os.Getuid() || int(outsideStat.Gid) != os.Getgid() {
		t.Fatalf("symlink target owner changed: %v", outsideInfo.Sys())
	}
	fileInfo, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("private file mode = %04o, want 0600", fileInfo.Mode().Perm())
	}
}

func TestAgentRunLockSerializesCLIAndHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-cli.lock")
	first, err := acquireAgentRunLock(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if first != nil {
			releaseAgentRunLock(first)
		}
	}()

	signals := make(chan os.Signal, 1)
	result := make(chan error, 1)
	go func() {
		lock, err := acquireAgentRunLock(path, signals)
		if lock != nil {
			releaseAgentRunLock(lock)
		}
		result <- err
	}()
	select {
	case err := <-result:
		t.Fatalf("second Agent run acquired the global lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	signals <- syscall.SIGTERM
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("cancelled Agent lock wait unexpectedly succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("Agent lock wait did not honor cancellation")
	}

	releaseAgentRunLock(first)
	first = nil
	second, err := acquireAgentRunLock(path, nil)
	if err != nil {
		t.Fatalf("acquire released Agent run lock: %v", err)
	}
	releaseAgentRunLock(second)
}

func TestRestoreRunWorkspaceReturnsOwnershipAndClosesRoot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("changing workspace ownership requires root")
	}
	root := t.TempDir()
	file := filepath.Join(root, "private")
	if err := os.WriteFile(file, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(root, agentUID, agentGID); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(file, agentUID, agentGID); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o777); err != nil {
		t.Fatal(err)
	}

	if err := restoreRunWorkspace(root); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != runnerUID || int(stat.Gid) != runnerGID || info.Mode().Perm() != 0o700 {
		t.Fatalf("restored idle run = owner %v mode %04o, want Runner %d:%d mode 0700", info.Sys(), info.Mode().Perm(), runnerUID, runnerGID)
	}
	fileInfo, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	fileStat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok || int(fileStat.Uid) != runnerUID || int(fileStat.Gid) != runnerGID || fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("restored run file = owner %v mode %04o, want Runner %d:%d mode 0600", fileInfo.Sys(), fileInfo.Mode().Perm(), runnerUID, runnerGID)
	}
}

func TestRecoverRunWorkspacesRestoresMixedOwnershipAndIsIdempotent(t *testing.T) {
	workspaceRoot, runRoot := newRecoveryWorkspace(t)
	file := filepath.Join(runRoot, "nested", "private")
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(runRoot, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(runRoot, agentUID, agentGID); err != nil {
		t.Fatal(err)
	}
	if err := chownTree(filepath.Join(runRoot, "nested"), agentUID, agentGID); err != nil {
		t.Fatal(err)
	}
	if err := recoverRunWorkspaces(workspaceRoot, filepath.Join(t.TempDir(), "agent-cli.lock")); err != nil {
		t.Fatalf("recover mixed ownership workspace: %v", err)
	}
	assertOwner(t, runRoot, runnerUID, runnerGID)
	if mode := modeOf(t, runRoot); mode != 0o700 {
		t.Fatalf("recovered run root mode = %04o, want 0700", mode)
	}
	assertOwner(t, filepath.Dir(file), runnerUID, runnerGID)
	assertOwner(t, file, runnerUID, runnerGID)
	if mode := modeOf(t, filepath.Dir(file)); mode&0o700 != 0o700 {
		t.Fatalf("recovered directory mode = %04o, want owner rwx", mode)
	}

	if err := recoverRunWorkspaces(workspaceRoot, filepath.Join(t.TempDir(), "agent-cli.lock")); err != nil {
		t.Fatalf("repeat recovery: %v", err)
	}
	assertOwner(t, runRoot, runnerUID, runnerGID)
	assertOwner(t, filepath.Dir(file), runnerUID, runnerGID)
	assertOwner(t, file, runnerUID, runnerGID)
}

func TestRecoverRunWorkspacesRejectsWritableWorkspaceParents(t *testing.T) {
	for _, target := range []string{"workspace root", "runs root"} {
		t.Run(target, func(t *testing.T) {
			workspaceRoot, runRoot := newRecoveryWorkspace(t)
			if err := os.Chown(runRoot, agentUID, agentGID); err != nil {
				t.Fatal(err)
			}
			path := workspaceRoot
			if target == "runs root" {
				path = filepath.Join(workspaceRoot, "runs")
			}
			if err := os.Chmod(path, 0o777); err != nil {
				t.Fatal(err)
			}
			if err := recoverRunWorkspaces(workspaceRoot, filepath.Join(t.TempDir(), "agent-cli.lock")); err == nil {
				t.Fatal("recovery accepted a group/world-writable workspace parent")
			}
			assertOwner(t, runRoot, agentUID, agentGID)
		})
	}
}

func TestRecoverRunWorkspacesRejectsUnsafeTreesWithoutPartialChown(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("changing workspace ownership requires root")
	}
	for _, kind := range []string{"hardlink", "third-party owner"} {
		t.Run(kind, func(t *testing.T) {
			workspaceRoot, runRoot := newRecoveryWorkspace(t)
			earlyFile := filepath.Join(runRoot, "a-agent-file")
			if err := os.WriteFile(earlyFile, []byte("secret"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chown(runRoot, agentUID, agentGID); err != nil {
				t.Fatal(err)
			}
			if err := os.Chown(earlyFile, agentUID, agentGID); err != nil {
				t.Fatal(err)
			}
			var externalPath string
			switch kind {
			case "hardlink":
				externalPath = filepath.Join(workspaceRoot, "outside")
				if err := os.WriteFile(externalPath, []byte("outside"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chown(externalPath, agentUID, agentGID); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(externalPath, filepath.Join(runRoot, "z-link")); err != nil {
					t.Fatal(err)
				}
			case "third-party owner":
				path := filepath.Join(runRoot, "z-foreign")
				if err := os.WriteFile(path, []byte("foreign"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chown(path, 10003, 10003); err != nil {
					t.Fatal(err)
				}
			}

			err := recoverRunWorkspaces(workspaceRoot, filepath.Join(t.TempDir(), "agent-cli.lock"))
			if err == nil {
				t.Fatal("recovery accepted an unsafe workspace tree")
			}
			assertOwner(t, runRoot, agentUID, agentGID)
			assertOwner(t, earlyFile, agentUID, agentGID)
			if externalPath != "" {
				assertOwner(t, externalPath, agentUID, agentGID)
			}
		})
	}
}

func TestRecoverRunWorkspacesRestoresFIFO(t *testing.T) {
	workspaceRoot, runRoot := newRecoveryWorkspace(t)
	fifo := filepath.Join(runRoot, "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(runRoot, agentUID, agentGID); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(fifo, agentUID, agentGID); err != nil {
		t.Fatal(err)
	}
	if err := recoverRunWorkspaces(workspaceRoot, filepath.Join(t.TempDir(), "agent-cli.lock")); err != nil {
		t.Fatal(err)
	}
	assertOwner(t, runRoot, runnerUID, runnerGID)
	assertOwner(t, fifo, runnerUID, runnerGID)
}

func TestRecoverRunWorkspacesRestoresSymlinkWithoutChangingTarget(t *testing.T) {
	workspaceRoot, runRoot := newRecoveryWorkspace(t)
	target := filepath.Join(workspaceRoot, "outside-target")
	content := []byte("outside content")
	if err := os.WriteFile(target, content, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(target, agentUID, agentGID); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(runRoot, "repo-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("create symlink: %v", err)
	}
	if err := os.Chown(runRoot, agentUID, agentGID); err != nil {
		t.Fatal(err)
	}
	if err := os.Lchown(link, agentUID, agentGID); err != nil {
		t.Fatal(err)
	}

	if err := recoverRunWorkspaces(workspaceRoot, filepath.Join(t.TempDir(), "agent-cli.lock")); err != nil {
		t.Fatalf("recover workspace with repository symlink: %v", err)
	}
	assertOwner(t, runRoot, runnerUID, runnerGID)
	assertOwner(t, link, runnerUID, runnerGID)
	assertOwner(t, target, agentUID, agentGID)
	if got := modeOf(t, target); got != 0o400 {
		t.Fatalf("symlink target mode = %04o, want unchanged 0400", got)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("symlink target content = %q, want %q", got, content)
	}
}

func TestRecoverRunWorkspacesPreflightsEveryRunBeforeChangingOwnership(t *testing.T) {
	workspaceRoot, firstRun := newRecoveryWorkspace(t)
	if err := os.Chown(firstRun, agentUID, agentGID); err != nil {
		t.Fatal(err)
	}
	secondRun := filepath.Join(workspaceRoot, "runs", "run-2")
	if err := os.Mkdir(secondRun, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(secondRun, agentUID, agentGID); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(workspaceRoot, "outside")
	if err := os.WriteFile(external, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(external, agentUID, agentGID); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(external, filepath.Join(secondRun, "linked")); err != nil {
		t.Fatal(err)
	}

	if err := recoverRunWorkspaces(workspaceRoot, filepath.Join(t.TempDir(), "agent-cli.lock")); err == nil {
		t.Fatal("recovery accepted a later unsafe run workspace")
	}
	assertOwner(t, firstRun, agentUID, agentGID)
	assertOwner(t, secondRun, agentUID, agentGID)
}

func TestRecoverRunWorkspacesRejectsActiveLauncherLock(t *testing.T) {
	workspaceRoot, runRoot := newRecoveryWorkspace(t)
	if err := os.Chown(runRoot, agentUID, agentGID); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(t.TempDir(), "agent-cli.lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	err = recoverRunWorkspaces(workspaceRoot, lockPath)
	if err == nil || !strings.Contains(err.Error(), "Agent CLI is active") {
		t.Fatalf("recovery error = %v, want active Agent CLI lock error", err)
	}
	assertOwner(t, runRoot, agentUID, agentGID)
}

func newRecoveryWorkspace(t *testing.T) (string, string) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("changing workspace ownership requires root")
	}
	base := t.TempDir()
	workspaceRoot := filepath.Join(base, "workspaces")
	runRoot := filepath.Join(workspaceRoot, "runs", "run-1")
	if err := os.MkdirAll(runRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{workspaceRoot, filepath.Join(workspaceRoot, "runs")} {
		if err := os.Chown(path, runnerUID, runnerGID); err != nil {
			t.Fatal(err)
		}
	}
	return workspaceRoot, runRoot
}

func assertOwner(t *testing.T, path string, uid, gid int) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != uid || int(stat.Gid) != gid {
		t.Fatalf("owner(%q) = %v, want %d:%d", path, info.Sys(), uid, gid)
	}
}

func modeOf(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestRecoverRunWorkspacesRejectsMissingAndSymlinkedRoots(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "missing")
	if err := recoverRunWorkspaces(missing, filepath.Join(t.TempDir(), "lock")); err == nil {
		t.Fatal("recovery accepted a missing workspace root")
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Skipf("create symlink: %v", err)
	}
	if err := recoverRunWorkspaces(link, filepath.Join(t.TempDir(), "lock")); err == nil {
		t.Fatal("recovery accepted a symlinked workspace root")
	}

	workspaceRoot, runRoot := newRecoveryWorkspace(t)
	outsideRun := filepath.Join(root, "outside-run")
	if err := os.Mkdir(outsideRun, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(runRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideRun, runRoot); err != nil {
		t.Skipf("create symlink: %v", err)
	}
	if err := recoverRunWorkspaces(workspaceRoot, filepath.Join(t.TempDir(), "lock")); err == nil {
		t.Fatal("recovery accepted a symlinked direct run root")
	}
	assertOwner(t, outsideRun, os.Getuid(), os.Getgid())
}

func TestRecoverRunWorkspacesAcceptsRelativeAndTrailingSlashRoots(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("changing workspace ownership requires root")
	}
	workspaceRoot, _ := newRecoveryWorkspace(t)
	if err := os.Chown(filepath.Join(workspaceRoot, "runs", "run-1"), agentUID, agentGID); err != nil {
		t.Fatal(err)
	}
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relativeRoot, err := filepath.Rel(workingDir, workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{relativeRoot, workspaceRoot + string(os.PathSeparator)} {
		if err := recoverRunWorkspaces(root, filepath.Join(t.TempDir(), "lock")); err != nil {
			t.Fatalf("recover workspace root %q: %v", root, err)
		}
	}
}
