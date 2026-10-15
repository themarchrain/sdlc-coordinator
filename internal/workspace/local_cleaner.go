package workspace

import "context"

// LocalRunWorkspaceCleaner 是本机形态的运行工作区清理器（协调器与 Runner
// 共享文件系统时的既有语义）。
type LocalRunWorkspaceCleaner struct {
	WorkspaceRoot string
}

func (c LocalRunWorkspaceCleaner) RemoveRunWorkspace(_ context.Context, _, runID string) error {
	return RemoveRunDirectory(c.WorkspaceRoot, runID)
}
