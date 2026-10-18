package orchestrator

import (
	"context"
	"os"
	"path/filepath"

	"sdlc-coordinator/internal/workspace"
)

type fixtureRunnerWorkspace struct{ root string }

func (f fixtureRunnerWorkspace) PublishTaskBundle(_ context.Context, _, runID string, files map[string][]byte) (string, error) {
	root := filepath.Join(f.root, "runs", runID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), content, 0o600); err != nil {
			return "", err
		}
	}
	return root, nil
}

func (f fixtureRunnerWorkspace) RemoveRunWorkspace(_ context.Context, _, runID string) error {
	return workspace.RemoveRunDirectory(f.root, runID)
}
