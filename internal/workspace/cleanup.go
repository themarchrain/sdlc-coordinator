package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func RemoveRunDirectory(workspaceRoot, runID string) error {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	runID = strings.TrimSpace(runID)
	if workspaceRoot == "" {
		return errors.New("workspace root is required")
	}
	if !safeRunIDPattern.MatchString(runID) {
		return errors.New("run id is not a safe path component")
	}
	return os.RemoveAll(filepath.Join(workspaceRoot, "runs", runID))
}
