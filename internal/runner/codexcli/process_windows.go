//go:build windows

package codexcli

import (
	"os"
	"os/exec"
	"syscall"
)

const createNewProcessGroup = 0x00000200

func configureProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup}
}

func commandInvocation(spec commandSpec) (string, []string, bool, error) {
	return spec.Binary, spec.Args, false, nil
}

func terminateProcessTree(process *os.Process, _ bool) error {
	if process == nil {
		return nil
	}
	return process.Kill()
}
