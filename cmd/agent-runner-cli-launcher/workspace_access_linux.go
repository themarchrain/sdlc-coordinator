//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const workspaceAccessLockPath = "/app/runner-data/operations/workspace-access.lock"

func withWorkspaceAccessLock(action func() error) error {
	lock, err := acquireAgentRunLock(workspaceAccessLockPath, nil)
	if err != nil {
		return err
	}
	defer releaseAgentRunLock(lock)
	return action()
}

func shareRunWorkspace(workspaceRoot, workspace string) error {
	if os.Getuid() != runnerUID || os.Getgid() != runnerGID {
		return errors.New("workspace access must be requested by Runner")
	}
	return withWorkspaceAccessLock(func() error {
		runRoot, err := resolveRunWorkspace(workspaceRoot, workspace)
		if err != nil {
			return err
		}
		for _, parent := range []string{filepath.Dir(runRoot), filepath.Dir(filepath.Dir(runRoot))} {
			if err := requireOwner(parent, runnerUID, runnerGID); err != nil {
				return fmt.Errorf("workspace access parent %q: %w", parent, err)
			}
		}
		info, err := os.Stat(runRoot)
		if err != nil {
			return err
		}
		stat := info.Sys().(*syscall.Stat_t)
		entries := []recoveryEntry{{path: runRoot, device: stat.Dev, inode: stat.Ino}}
		for _, name := range []string{"workspace-manifest.json", "repo"} {
			path := filepath.Join(runRoot, name)
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			if (name == "repo" && !info.IsDir()) || (name != "repo" && !info.Mode().IsRegular()) {
				return fmt.Errorf("workspace delivery path %q has an invalid type", path)
			}
			paths, err := inspectRunWorkspaceTree(path, stat.Dev)
			if err != nil {
				return err
			}
			entries = append(entries, paths...)
		}
		root, err := os.OpenFile(runRoot, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_DIRECTORY, 0)
		if err != nil {
			return err
		}
		defer root.Close()
		for _, entry := range entries {
			relative, err := filepath.Rel(runRoot, entry.path)
			if err != nil {
				return err
			}
			if entry.preserveMode || entry.hardLinked {
				continue
			}
			if err := grantRunnerGroupAccess(root, relative, entry); err != nil {
				return fmt.Errorf("grant Runner access to %q: %w", entry.path, err)
			}
		}
		return nil
	})
}

func grantRunnerGroupAccess(root *os.File, relative string, expected recoveryEntry) error {
	file, err := openWorkspaceEntry(root, relative)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return errors.New("workspace entry must be a regular file or directory")
	}
	stat := info.Sys().(*syscall.Stat_t)
	if stat.Dev != expected.device || stat.Ino != expected.inode {
		return errors.New("workspace entry changed during access grant")
	}
	if int(stat.Uid) == runnerUID && int(stat.Gid) == runnerGID {
		return nil
	}
	if !((int(stat.Uid) == runnerUID && int(stat.Gid) == runnerGID) || (int(stat.Uid) == agentUID && (int(stat.Gid) == agentGID || int(stat.Gid) == runnerGID))) {
		return errors.New("workspace entry owner changed during access grant")
	}
	if info.Mode().IsRegular() && stat.Nlink != 1 {
		return nil
	}
	mode := info.Mode().Perm() | 0o060
	if info.IsDir() {
		mode |= 0o010
	}
	if err := file.Chown(-1, runnerGID); err != nil {
		return err
	}
	return file.Chmod(mode)
}

func openWorkspaceEntry(root *os.File, relative string) (*os.File, error) {
	fd, err := syscall.Dup(int(root.Fd()))
	if err != nil {
		return nil, err
	}
	parts := strings.Split(relative, string(filepath.Separator))
	for index, part := range parts {
		flags := syscall.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_NONBLOCK | syscall.O_CLOEXEC
		if index < len(parts)-1 {
			flags |= syscall.O_DIRECTORY
		}
		next, err := syscall.Openat(fd, part, flags, 0)
		_ = syscall.Close(fd)
		if err != nil {
			return nil, err
		}
		fd = next
	}
	return os.NewFile(uintptr(fd), relative), nil
}
