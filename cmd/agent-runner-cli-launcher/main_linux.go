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
	if len(os.Args) == 3 && os.Args[1] == "--recover" {
		if err := checkPrivileges(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := recoverRunWorkspaces(os.Args[2], agentRunLockPath); err != nil {
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
	home, err := os.MkdirTemp(runRoot, ".agent-home-")
	if err != nil {
		return 1, fmt.Errorf("create private Agent home: %w", err)
	}
	defer os.RemoveAll(home)
	if err := chownTree(runRoot, agentUID, agentGID); err != nil {
		restoreErr := restoreRunWorkspace(runRoot)
		return 1, errors.Join(fmt.Errorf("give agent ownership of run workspace: %w", err), restoreErr)
	}

	code, runErr := runAsAgent(runRoot, home, binary, commandArgs[1:], signals)
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

func runAsAgent(workspace, home, binary string, args []string, signals <-chan os.Signal) (int, error) {
	select {
	case <-signals:
		return 1, errors.New("launcher interrupted before starting Agent CLI")
	default:
	}
	command := exec.Command(binary, args...)
	command.Dir = workspace
	command.Env = agentEnvironment(home)
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

func agentEnvironment(home string) []string {
	overrides := map[string]string{
		"HOME": home, "USER": "agent", "LOGNAME": "agent",
		"XDG_CACHE_HOME": filepath.Join(home, ".cache"),
		"GOCACHE":        filepath.Join(home, ".cache", "go-build"),
		"GOPATH":         filepath.Join(home, "go"),
		"GOMODCACHE":     filepath.Join(home, "go", "pkg", "mod"),
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if _, replaced := overrides[name]; !replaced {
			environment = append(environment, entry)
		}
	}
	for name, value := range overrides {
		environment = append(environment, name+"="+value)
	}
	return environment
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

func recoverRunWorkspaces(workspaceRoot, lockPath string) error {
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open Agent run lock for workspace recovery: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return errors.New("Agent CLI is active; cannot recover Runner workspaces")
		}
		return fmt.Errorf("lock Agent run workspace for recovery: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if err := stopLingeringAgentProcesses(); err != nil {
		return fmt.Errorf("stop Agent processes before workspace recovery: %w", err)
	}
	return recoverRunWorkspaceTrees(workspaceRoot)
}

func recoverRunWorkspaceTrees(workspaceRoot string) error {
	var err error
	workspaceRoot, err = filepath.Abs(workspaceRoot)
	if err != nil {
		return fmt.Errorf("resolve workspace root for recovery: %w", err)
	}
	workspaceInfo, err := os.Lstat(workspaceRoot)
	if err != nil {
		return fmt.Errorf("inspect workspace root for recovery: %w", err)
	}
	if !workspaceInfo.IsDir() {
		return errors.New("workspace root for recovery must be a real directory")
	}
	if err := requireOwner(workspaceRoot, runnerUID, runnerGID); err != nil {
		return fmt.Errorf("workspace root for recovery: %w", err)
	}
	workspaceStat, ok := workspaceInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("workspace root for recovery has an unsupported file identity")
	}
	runsRoot := filepath.Join(workspaceRoot, "runs")
	runsInfo, err := os.Lstat(runsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect runs directory for recovery: %w", err)
	}
	if !runsInfo.IsDir() {
		return errors.New("runs directory for recovery must be a real directory")
	}
	if err := requireOwner(runsRoot, runnerUID, runnerGID); err != nil {
		return fmt.Errorf("runs directory for recovery: %w", err)
	}
	runsStat, ok := runsInfo.Sys().(*syscall.Stat_t)
	if !ok || runsStat.Dev != workspaceStat.Dev {
		return errors.New("runs directory for recovery is on an unexpected filesystem")
	}
	entries, err := os.ReadDir(runsRoot)
	if err != nil {
		return fmt.Errorf("read runs directory for recovery: %w", err)
	}
	var paths []recoveryEntry
	for _, entry := range entries {
		runRoot := filepath.Join(runsRoot, entry.Name())
		info, err := os.Lstat(runRoot)
		if err != nil {
			return fmt.Errorf("inspect run workspace %q for recovery: %w", entry.Name(), err)
		}
		if !info.IsDir() {
			return fmt.Errorf("run workspace %q must be a real directory", entry.Name())
		}
		workspacePaths, err := inspectRunWorkspaceTree(runRoot, runsStat.Dev)
		if err != nil {
			return fmt.Errorf("recover run workspace %q: %w", entry.Name(), err)
		}
		paths = append(paths, workspacePaths...)
	}
	return restoreRunnerWorkspaceOwnership(paths)
}

type recoveryEntry struct {
	path         string
	mode         os.FileMode
	runRoot      bool
	preserveMode bool
}

func inspectRunWorkspaceTree(runRoot string, expectedDev uint64) ([]recoveryEntry, error) {
	paths := make([]recoveryEntry, 0, 32)
	if err := filepath.WalkDir(runRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		preserveMode := info.Mode()&(os.ModeSymlink|os.ModeNamedPipe|os.ModeSocket) != 0
		if !preserveMode && !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("special file is not allowed in a recoverable workspace: %q", path)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Dev != expectedDev {
			return fmt.Errorf("workspace entry is on an unexpected filesystem: %q", path)
		}
		if !((int(stat.Uid) == runnerUID && int(stat.Gid) == runnerGID) || (int(stat.Uid) == agentUID && int(stat.Gid) == agentGID)) {
			return fmt.Errorf("workspace entry %q has an untrusted owner %d:%d", path, stat.Uid, stat.Gid)
		}
		if info.Mode().IsRegular() && stat.Nlink != 1 {
			return fmt.Errorf("hard-linked file is not allowed in a recoverable workspace: %q", path)
		}
		paths = append(paths, recoveryEntry{path: path, mode: info.Mode(), runRoot: filepath.Clean(path) == filepath.Clean(runRoot), preserveMode: preserveMode})
		return nil
	}); err != nil {
		return nil, fmt.Errorf("preflight workspace tree: %w", err)
	}
	return paths, nil
}

func restoreRunnerWorkspaceOwnership(paths []recoveryEntry) error {
	for _, entry := range paths {
		if err := os.Lchown(entry.path, runnerUID, runnerGID); err != nil {
			return fmt.Errorf("return workspace entry %q to Runner: %w", entry.path, err)
		}
		if entry.preserveMode {
			continue
		}
		mode := entry.mode.Perm()
		if entry.runRoot {
			mode = 0o700
		} else if entry.mode.IsDir() {
			mode |= 0o700
		} else {
			mode |= 0o600
		}
		if err := os.Chmod(entry.path, mode); err != nil {
			return fmt.Errorf("restore Runner access to workspace entry %q: %w", entry.path, err)
		}
	}
	return nil
}

func requireOwner(path string, uid, gid int) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("path must be a real directory")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return errors.New("directory must not be group or world writable")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != uid || int(stat.Gid) != gid {
		return fmt.Errorf("directory must be owned by Runner %d:%d", uid, gid)
	}
	return nil
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
