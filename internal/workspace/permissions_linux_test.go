//go:build linux

package workspace

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

const agentWorkspaceProbeEnv = "SDLC_AGENT_WORKSPACE_PERMISSION_PROBE"

func TestAgentUIDProbeCannotReadIdleRun(t *testing.T) {
	path := os.Getenv(agentWorkspaceProbeEnv)
	if path == "" {
		return
	}
	if _, err := os.ReadFile(path); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Agent read idle run file: err = %v, want permission denied", err)
	}
}

func TestRestrictedIdleRunIsRunnerOwnedAndNotTraversableByAgent(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("changing to the Agent UID requires root")
	}

	root, err := os.MkdirTemp("/tmp", "workspace-permission-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	runRoot := filepath.Join(root, "runs", "idle-run")
	if err := os.MkdirAll(runRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(runRoot, "private.txt")
	if err := os.WriteFile(secret, []byte("private"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(runRoot, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := restrictRunWorkspaceRoot(runRoot); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(runRoot)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() || int(stat.Gid) != os.Getegid() {
		t.Fatalf("idle run owner = %v, want current Runner identity %s:%s", info.Sys(), strconv.Itoa(os.Geteuid()), strconv.Itoa(os.Getegid()))
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("idle run mode = %04o, want 0700", got)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestAgentUIDProbeCannotReadIdleRun$")
	cmd.Env = append(os.Environ(), agentWorkspaceProbeEnv+"="+secret)
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 10002, Gid: 10002}}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Agent UID could traverse idle run: %v\n%s", err, output)
	}
}
