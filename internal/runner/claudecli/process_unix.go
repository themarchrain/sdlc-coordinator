//go:build !windows

package claudecli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const cliLauncherPath = "/usr/local/bin/agent-runner-cli-launcher"

func commandInvocation(spec commandSpec) (string, []string, bool, error) {
	launcher := strings.TrimSpace(os.Getenv("AGENT_RUNNER_CLI_LAUNCHER"))
	if launcher == "" {
		return spec.Binary, spec.Args, false, nil
	}
	if filepath.Clean(launcher) != cliLauncherPath {
		return "", nil, false, errors.New("agent CLI launcher path is invalid")
	}
	workspaceRoot := strings.TrimSpace(os.Getenv("AGENT_RUNNER_WORKSPACE_ROOT"))
	if workspaceRoot == "" {
		return "", nil, false, errors.New("agent runner workspace root is required when CLI isolation is enabled")
	}
	args := []string{"--workspace-root", workspaceRoot, "--workspace", spec.Dir, "--", spec.Binary}
	args = append(args, spec.Args...)
	return launcher, args, true, nil
}

func configureProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminateProcessTree(process *os.Process, launcher bool) error {
	if process == nil {
		return nil
	}
	if launcher {
		if err := process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
		return nil
	}
	return syscall.Kill(-process.Pid, syscall.SIGKILL)
}
