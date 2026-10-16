//go:build linux

package runner

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

const testCLILauncherPath = "/usr/local/bin/agent-runner-cli-launcher"

func TestNewServiceTightensExistingExecutionsDirectory(t *testing.T) {
	dataRoot := t.TempDir()
	executionsRoot := filepath.Join(dataRoot, "executions")
	if err := os.Mkdir(executionsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(executionsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService(dataRoot, nil); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(executionsRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("executions directory mode = %04o, want 0700", got)
	}
}

func TestNewWorkspaceOpsTightensRunnerOwnedOperationsDirectory(t *testing.T) {
	ops, _ := newWorkspaceOpsForTest(t, 0)
	info, err := os.Stat(ops.opsRoot)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() || int(stat.Gid) != os.Getegid() {
		t.Fatalf("operations directory owner = %v, want Runner %d:%d", info.Sys(), os.Geteuid(), os.Getegid())
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("operations directory mode = %04o, want 0700", got)
	}
}

func TestNewWorkspaceOpsRejectsOperationsDirectoryOwnedByAgent(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("changing directory ownership requires root")
	}
	dataRoot := t.TempDir()
	opsRoot := filepath.Join(dataRoot, "operations")
	if err := os.Mkdir(opsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(opsRoot, 10002, 10002); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWorkspaceOps(WorkspaceOpsConfig{DataRoot: dataRoot}); err == nil {
		t.Fatal("NewWorkspaceOps accepted an operations directory owned by Agent")
	}
}

func TestWorkspaceOpsPublishesRunnerOwnedPrivateRunRoot(t *testing.T) {
	t.Setenv(cliIsolationEnv, testCLILauncherPath)
	origin, _ := createSeedOrigin(t)
	ops, dataRoot := newWorkspaceOpsForTest(t, 0)
	if _, err := ops.PrepareWorkspace(context.Background(), developerPrepareRequest(origin, "run-private", "op-private")); err != nil {
		t.Fatal(err)
	}
	runRoot := filepath.Join(dataRoot, "workspaces", "runs", "run-private")
	info, err := os.Stat(runRoot)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() || int(stat.Gid) != os.Getegid() {
		t.Fatalf("run root owner = %v, want Runner %d:%d", info.Sys(), os.Geteuid(), os.Getegid())
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("run root mode = %04o, want 0700", got)
	}
}

func TestNewWorkspaceOpsHardensExistingIdleRunRootsOnly(t *testing.T) {
	t.Setenv(cliIsolationEnv, testCLILauncherPath)
	dataRoot := t.TempDir()
	workspaceRoot := filepath.Join(dataRoot, "workspaces")
	runRoot := filepath.Join(workspaceRoot, "runs", "legacy-run")
	if err := os.MkdirAll(runRoot, 0o777); err != nil {
		t.Fatal(err)
	}
	privateFile := filepath.Join(runRoot, "manifest.json")
	if err := os.WriteFile(privateFile, []byte("{}"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(privateFile, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(runRoot, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWorkspaceOps(WorkspaceOpsConfig{DataRoot: dataRoot, WorkspaceRoot: workspaceRoot}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(runRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("legacy idle run root mode = %04o, want 0700", got)
	}
	fileInfo, err := os.Stat(privateFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o666 {
		t.Fatalf("legacy run file mode changed to %04o, want unchanged 0666", got)
	}
}

func TestNewWorkspaceOpsRejectsUnsafeExistingRunRoots(t *testing.T) {
	t.Setenv(cliIsolationEnv, testCLILauncherPath)
	if os.Geteuid() != 0 {
		t.Skip("changing directory ownership requires root")
	}
	t.Run("symlink", func(t *testing.T) {
		dataRoot := t.TempDir()
		workspaceRoot := filepath.Join(dataRoot, "workspaces")
		runsRoot := filepath.Join(workspaceRoot, "runs")
		if err := os.MkdirAll(runsRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(t.TempDir(), filepath.Join(runsRoot, "linked-run")); err != nil {
			t.Skipf("create symlink: %v", err)
		}
		if _, err := NewWorkspaceOps(WorkspaceOpsConfig{DataRoot: dataRoot, WorkspaceRoot: workspaceRoot}); err == nil {
			t.Fatal("NewWorkspaceOps accepted a symlinked run root")
		}
	})
	t.Run("foreign owner", func(t *testing.T) {
		dataRoot := t.TempDir()
		workspaceRoot := filepath.Join(dataRoot, "workspaces")
		runRoot := filepath.Join(workspaceRoot, "runs", "foreign-run")
		if err := os.MkdirAll(runRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chown(runRoot, 10002, 10002); err != nil {
			t.Fatal(err)
		}
		if _, err := NewWorkspaceOps(WorkspaceOpsConfig{DataRoot: dataRoot, WorkspaceRoot: workspaceRoot}); err == nil {
			t.Fatal("NewWorkspaceOps accepted a run root not owned by Runner")
		}
	})
}

func TestNewWorkspaceOpsPreservesLegacySharedRunRoots(t *testing.T) {
	t.Setenv(cliIsolationEnv, "")
	dataRoot := t.TempDir()
	workspaceRoot := filepath.Join(dataRoot, "workspaces")
	runRoot := filepath.Join(workspaceRoot, "runs", "shared-run")
	if err := os.MkdirAll(runRoot, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(runRoot, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWorkspaceOps(WorkspaceOpsConfig{DataRoot: dataRoot, WorkspaceRoot: workspaceRoot}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(runRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o777 {
		t.Fatalf("legacy shared run root mode = %04o, want unchanged 0777", got)
	}
}
