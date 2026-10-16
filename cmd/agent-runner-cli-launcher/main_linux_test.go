//go:build linux

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

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
