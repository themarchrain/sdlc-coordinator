//go:build linux

package runner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func secureOperationsDirectory(path string) error {
	return secureRunnerOwnedDirectory(path, 0o700)
}

func secureWorkspaceRunRoots(workspaceRoot string) error {
	if err := secureRunnerOwnedDirectory(workspaceRoot, 0o755); err != nil {
		return fmt.Errorf("secure workspace root: %w", err)
	}
	runsRoot := filepath.Join(workspaceRoot, "runs")
	if err := os.MkdirAll(runsRoot, 0o755); err != nil {
		return fmt.Errorf("create runs directory: %w", err)
	}
	if err := secureRunnerOwnedDirectory(runsRoot, 0o755); err != nil {
		return fmt.Errorf("secure runs directory: %w", err)
	}
	entries, err := os.ReadDir(runsRoot)
	if err != nil {
		return fmt.Errorf("read runs directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			return fmt.Errorf("runs entry %q is not a real directory", entry.Name())
		}
		if err := secureRunnerOwnedDirectory(filepath.Join(runsRoot, entry.Name()), 0o700); err != nil {
			return fmt.Errorf("secure idle run %q: %w", entry.Name(), err)
		}
	}
	return nil
}

func secureRunnerOwnedDirectory(path string, mode os.FileMode) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("operations path must be a real directory")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() || int(stat.Gid) != os.Getegid() {
		return fmt.Errorf("directory must be owned by Runner %d:%d", os.Geteuid(), os.Getegid())
	}
	return os.Chmod(path, mode)
}
