package workspace_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"sdlc-coordinator/internal/workspace"
)

func TestDeveloperPreparerKeepsRunRootPrivateInRunnerMode(t *testing.T) {
	origin, _, _ := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.DeveloperRequest{
		WorkspaceRoot: root, PrivateRunRoot: true, TaskID: "task-perm-1", RunID: "run-perm-1",
		TaskType: "develop_from_issue", Project: "group/project", RepositoryURL: origin,
		TargetBranch: "master", DeliveryBranch: "agent/task-perm-1",
	}
	_, err := workspace.NewDeveloperPreparer().Prepare(context.Background(), request)
	if err != nil {
		t.Fatalf("prepare developer workspace: %v", err)
	}
	runRoot := filepath.Join(root, "runs", request.RunID)
	assertPrivateRunRoot(t, runRoot)
	if err := os.Chmod(runRoot, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.NewDeveloperPreparer().Prepare(context.Background(), request); err != nil {
		t.Fatalf("reuse developer workspace: %v", err)
	}
	assertPrivateRunRoot(t, runRoot)
}

func TestReviewerPreparerKeepsRunRootPrivateInRunnerMode(t *testing.T) {
	origin, _, featureHead := createReviewOrigin(t)
	root := t.TempDir()
	_, err := workspace.NewReviewerPreparer().Prepare(context.Background(), workspace.ReviewerRequest{
		WorkspaceRoot: root, PrivateRunRoot: true, TaskID: "task-perm-r", RunID: "run-perm-r",
		Project: "group/project", RepositoryURL: origin,
		SourceBranch: "feature/review", TargetBranch: "master", HeadSHA: featureHead,
		RequestedReviewMode: "full",
	})
	if err != nil {
		t.Fatalf("prepare reviewer workspace: %v", err)
	}
	assertPrivateRunRoot(t, filepath.Join(root, "runs", "run-perm-r"))
}

func TestPreparerPreservesSharedRuntimePermissionsByDefault(t *testing.T) {
	origin, _, head := createReviewOrigin(t)
	root := t.TempDir()
	_, err := workspace.NewDeveloperPreparer().Prepare(context.Background(), workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-shared", RunID: "run-shared",
		TaskType: "develop_from_issue", Project: "group/project", RepositoryURL: origin,
		TargetBranch: "master", DeliveryBranch: "agent/task-shared",
	})
	if err != nil {
		t.Fatalf("prepare developer workspace: %v", err)
	}
	assertSharedRuntimeTree(t, filepath.Join(root, "runs", "run-shared"))

	_, err = workspace.NewReviewerPreparer().Prepare(context.Background(), workspace.ReviewerRequest{
		WorkspaceRoot: root, TaskID: "task-shared-review", RunID: "run-shared-review",
		Project: "group/project", RepositoryURL: origin,
		SourceBranch: "feature/review", TargetBranch: "master", HeadSHA: head,
		RequestedReviewMode: "full",
	})
	if err != nil {
		t.Fatalf("prepare reviewer workspace: %v", err)
	}
	assertSharedRuntimeTree(t, filepath.Join(root, "runs", "run-shared-review"))
}

func assertSharedRuntimeTree(t *testing.T, runRoot string) {
	t.Helper()
	for _, path := range []string{runRoot, filepath.Join(runRoot, "repo"), filepath.Join(runRoot, workspace.ManifestFileName), filepath.Join(runRoot, "repo", "README.md")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected published path %s: %v", path, err)
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	assertPosixMode(t, runRoot, 0o777)
	assertPosixMode(t, filepath.Join(runRoot, "repo"), 0o777)
	assertPosixMode(t, filepath.Join(runRoot, workspace.ManifestFileName), 0o666)
	assertPosixMode(t, filepath.Join(runRoot, "repo", "README.md"), 0o666)
}

func assertPrivateRunRoot(t *testing.T, runRoot string) {
	t.Helper()
	manifestPath := filepath.Join(runRoot, workspace.ManifestFileName)
	worktreeFile := filepath.Join(runRoot, "repo", "README.md")
	for _, path := range []string{runRoot, filepath.Join(runRoot, "repo"), manifestPath, worktreeFile} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected published path %s: %v", path, err)
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	assertPosixMode(t, runRoot, 0o700)
}

func assertPosixMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s: expected mode %o, got %o", path, want, got)
	}
}
