//go:build linux

package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	runnerUID        = 10001
	runnerGID        = 10001
	agentUID         = 10002
	agentGID         = 10002
	agentRunLockPath = "/app/runner-data/operations/agent-cli.lock"
)

var allowedBinaries = []string{"/opt/codex/bin/codex", "/opt/claude/bin/claude"}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--check" {
		if err := checkPrivileges(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	code, err := launch(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

func launch(args []string) (int, error) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signals)

	flags := flag.NewFlagSet("agent-runner-cli-launcher", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	workspaceRoot := flags.String("workspace-root", "", "")
	workspace := flags.String("workspace", "", "")
	if err := flags.Parse(args); err != nil {
		return 1, err
	}
	commandArgs := flags.Args()
	if len(commandArgs) < 1 {
		return 1, errors.New("launcher requires -- followed by an allowed CLI command")
	}
	root, err := filepath.Abs(*workspaceRoot)
	if err != nil {
		return 1, fmt.Errorf("resolve workspace root: %w", err)
	}
	runRoot, err := resolveRunWorkspace(root, *workspace)
	if err != nil {
		return 1, err
	}
	binary, err := resolveAllowedBinary(commandArgs[0])
	if err != nil {
		return 1, err
	}
	lock, err := acquireAgentRunLock(agentRunLockPath, signals)
	if err != nil {
		return 1, err
	}
	defer releaseAgentRunLock(lock)
	if err := stopLingeringAgentProcesses(); err != nil {
		return 1, fmt.Errorf("clear previous Agent processes: %w", err)
	}
	if err := chownTree(runRoot, agentUID, agentGID); err != nil {
		restoreErr := restoreRunWorkspace(runRoot)
		return 1, errors.Join(fmt.Errorf("give agent ownership of run workspace: %w", err), restoreErr)
	}

	code, runErr := runAsAgent(runRoot, binary, commandArgs[1:], signals)
	if err := stopLingeringAgentProcesses(); err != nil {
		return 1, errors.Join(runErr, fmt.Errorf("stop remaining Agent processes: %w", err))
	}
	restoreErr := restoreRunWorkspace(runRoot)
	if restoreErr != nil {
		return 1, errors.Join(runErr, fmt.Errorf("return run workspace to runner: %w", restoreErr))
	}
	return code, runErr
}

func resolveRunWorkspace(workspaceRoot, workspace string) (string, error) {
	root, err := filepath.EvalSymlinks(workspaceRoot)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("make workspace root absolute: %w", err)
	}
	requested, err := filepath.Abs(workspace)
	if err != nil {
		return "", fmt.Errorf("resolve run workspace: %w", err)
	}
	realRun, err := filepath.EvalSymlinks(requested)
	if err != nil {
		return "", fmt.Errorf("resolve run workspace: %w", err)
	}
	realRun, err = filepath.Abs(realRun)
	if err != nil {
		return "", fmt.Errorf("make run workspace absolute: %w", err)
	}
	if filepath.Clean(requested) != filepath.Clean(realRun) {
		return "", errors.New("run workspace path contains a symlink")
	}
	rel, err := filepath.Rel(root, realRun)
	if err != nil || rel == "." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("run workspace is outside the configured workspace root")
	}
	parts := strings.Split(filepath.Clean(rel), string(filepath.Separator))
	if len(parts) != 2 || parts[0] != "runs" || parts[1] == "." || parts[1] == ".." {
		return "", errors.New("run workspace must be a direct child of the runs directory")
	}
	info, err := os.Stat(realRun)
	if err != nil {
		return "", fmt.Errorf("inspect run workspace: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("run workspace is not a directory")
	}
	return realRun, nil
}

func resolveAllowedBinary(binary string) (string, error) {
	resolved, err := exec.LookPath(binary)
	if err != nil {
		return "", fmt.Errorf("find CLI executable: %w", err)
	}
	resolved, err = filepath.EvalSymlinks(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve CLI executable: %w", err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("make CLI executable absolute: %w", err)
	}
	for _, allowed := range allowedBinaries {
		allowedPath, evalErr := filepath.EvalSymlinks(allowed)
		if evalErr != nil {
			continue
		}
		allowedPath, evalErr = filepath.Abs(allowedPath)
		if evalErr == nil && resolved == allowedPath {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("CLI executable %q is not allowed", resolved)
}

func runAsAgent(workspace, binary string, args []string, signals <-chan os.Signal) (int, error) {
	select {
	case <-signals:
		return 1, errors.New("launcher interrupted before starting Agent CLI")
	default:
	}
	command := exec.Command(binary, args...)
	command.Dir = workspace
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
		Credential: &syscall.Credential{
			Uid: uint32(agentUID),
			Gid: uint32(agentGID),
		},
	}
	if err := command.Start(); err != nil {
		return 1, fmt.Errorf("start CLI as agent: %w", err)
	}
	finished := make(chan struct{})
	go func(pid int) {
		select {
		case <-signals:
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		case <-finished:
		}
	}(command.Process.Pid)
	if err := verifyAgentProcess(command.Process.Pid); err != nil {
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		_ = command.Wait()
		close(finished)
		return 1, err
	}

	waitErr := command.Wait()
	close(finished)
	_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	if waitErr == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			if status.Signaled() {
				return 128 + int(status.Signal()), nil
			}
			return status.ExitStatus(), nil
		}
	}
	return 1, fmt.Errorf("wait for CLI: %w", waitErr)
}

func acquireAgentRunLock(path string, signals <-chan os.Signal) (*os.File, error) {
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open Agent run lock: %w", err)
	}
	for {
		err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			_ = lock.Close()
			return nil, fmt.Errorf("lock Agent run workspace: %w", err)
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-signals:
			timer.Stop()
			_ = lock.Close()
			return nil, errors.New("launcher interrupted while waiting for another Agent run")
		case <-timer.C:
		}
	}
}

func releaseAgentRunLock(lock *os.File) {
	_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	_ = lock.Close()
}

func stopLingeringAgentProcesses() error {
	deadline := time.Now().Add(3 * time.Second)
	for {
		entries, err := os.ReadDir("/proc")
		if err != nil {
			return fmt.Errorf("inspect process table: %w", err)
		}
		remaining := 0
		for _, entry := range entries {
			pid, err := strconv.Atoi(entry.Name())
			if err != nil {
				continue
			}
			status, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "status"))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return fmt.Errorf("inspect process %d: %w", pid, err)
			}
			uid, zombie := processIdentity(status)
			if uid != agentUID || zombie {
				continue
			}
			remaining++
			if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
				return fmt.Errorf("stop Agent process %d: %w", pid, err)
			}
		}
		if remaining == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%d Agent processes remain after SIGKILL", remaining)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func processIdentity(status []byte) (int, bool) {
	uid := -1
	zombie := false
	for _, line := range strings.Split(string(status), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch key {
		case "Uid":
			fields := strings.Fields(value)
			if len(fields) == 4 {
				uid, _ = strconv.Atoi(fields[0])
			}
		case "State":
			zombie = strings.HasPrefix(strings.TrimSpace(value), "Z")
		}
	}
	return uid, zombie
}

func verifyAgentProcess(pid int) error {
	status, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect CLI process identity: %w", err)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(status), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch key {
		case "Uid", "Gid", "Groups", "CapEff", "CapPrm", "CapInh":
			values[key] = strings.TrimSpace(value)
		}
	}
	for key, expectedID := range map[string]int{"Uid": agentUID, "Gid": agentGID} {
		ids := strings.Fields(values[key])
		if len(ids) != 4 {
			return fmt.Errorf("CLI process has malformed %s identity", key)
		}
		want := strconv.Itoa(expectedID)
		for _, id := range ids {
			if id != want {
				return fmt.Errorf("CLI process %s is not isolated to Agent %d", key, expectedID)
			}
		}
	}
	if strings.TrimSpace(values["Groups"]) != "" {
		return errors.New("CLI process retained supplementary groups")
	}
	for _, key := range []string{"CapEff", "CapPrm", "CapInh"} {
		if values[key] != "0000000000000000" {
			return fmt.Errorf("CLI process retained Linux capabilities in %s", key)
		}
	}
	return nil
}

func chownTree(root string, uid, gid int) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := os.Lchown(path, uid, gid); err != nil {
			return fmt.Errorf("change owner for %q: %w", path, err)
		}
		return nil
	})
}

func restoreRunWorkspace(root string) error {
	if err := chownTree(root, runnerUID, runnerGID); err != nil {
		return err
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return fmt.Errorf("restrict idle run workspace root: %w", err)
	}
	return nil
}

func checkPrivileges() error {
	if os.Getuid() != runnerUID || os.Getgid() != runnerGID {
		return fmt.Errorf("launcher must run as runner %d:%d", runnerUID, runnerGID)
	}
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return fmt.Errorf("read process privileges: %w", err)
	}
	var effective uint64
	var noNewPrivileges string
	for _, line := range strings.Split(string(status), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch key {
		case "CapEff":
			effective, err = strconv.ParseUint(strings.TrimSpace(value), 16, 64)
			if err != nil {
				return fmt.Errorf("parse effective capabilities: %w", err)
			}
		case "NoNewPrivs":
			noNewPrivileges = strings.TrimSpace(value)
		}
	}
	required := uint64(1<<0 | 1<<1 | 1<<5 | 1<<6 | 1<<7)
	if effective&required != required {
		return errors.New("CLI launcher lacks CAP_CHOWN, CAP_DAC_OVERRIDE, CAP_KILL, CAP_SETGID, or CAP_SETUID; check container capability bounds")
	}
	if noNewPrivileges != "0" {
		return errors.New("CLI launcher cannot gain file capabilities with no-new-privileges enabled")
	}
	return checkWorkspaceOwnershipRoundTrip()
}

func checkWorkspaceOwnershipRoundTrip() (resultErr error) {
	root, err := os.MkdirTemp("/app/workspaces", ".cli-launcher-check-")
	if err != nil {
		return fmt.Errorf("create launcher permission check workspace: %w", err)
	}
	childDir := filepath.Join(root, "locked")
	file := filepath.Join(childDir, "private")
	defer func() {
		_ = os.Chown(root, runnerUID, runnerGID)
		_ = os.Chmod(root, 0o700)
		_ = os.Chown(childDir, runnerUID, runnerGID)
		_ = os.Chmod(childDir, 0o700)
		_ = chownTree(root, runnerUID, runnerGID)
		if err := os.RemoveAll(root); resultErr == nil && err != nil {
			resultErr = fmt.Errorf("remove launcher permission check workspace: %w", err)
		}
	}()
	if err := os.Mkdir(childDir, 0o700); err != nil {
		return fmt.Errorf("create launcher permission check directory: %w", err)
	}
	if err := os.WriteFile(file, []byte("check"), 0o600); err != nil {
		return fmt.Errorf("create launcher permission check file: %w", err)
	}
	if err := os.Chmod(childDir, 0); err != nil {
		return fmt.Errorf("restrict launcher permission check directory: %w", err)
	}
	if err := chownTree(root, agentUID, agentGID); err != nil {
		return fmt.Errorf("test Agent workspace ownership: %w", err)
	}
	if err := chownTree(root, runnerUID, runnerGID); err != nil {
		return fmt.Errorf("test Runner workspace ownership return: %w", err)
	}
	if err := os.Chmod(childDir, 0o700); err != nil {
		return fmt.Errorf("restore launcher permission check directory mode: %w", err)
	}
	info, err := os.Stat(file)
	if err != nil {
		return fmt.Errorf("inspect launcher permission check file: %w", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != runnerUID || int(stat.Gid) != runnerGID || info.Mode().Perm() != 0o600 {
		return errors.New("launcher did not return 0600 workspace files to the Runner")
	}
	return nil
}
