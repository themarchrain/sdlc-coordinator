//go:build !linux

package runner

func secureOperationsDirectory(string) error { return nil }

func secureWorkspaceRunRoots(string) error { return nil }
