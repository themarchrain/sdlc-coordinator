package workspace_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/workspace"
)

func TestDeveloperPreparerCreatesDeliveryBranchFromTarget(t *testing.T) {
	origin, targetSHA, _ := createReviewOrigin(t)
	root := t.TempDir()
	manifest, err := workspace.NewDeveloperPreparer().Prepare(context.Background(), workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-developer-1", RunID: "run-developer-1",
		TaskType: "develop_from_issue", Project: "group/project", RepositoryURL: origin,
		TargetBranch: "master", DeliveryBranch: "agent/task-developer-1",
	})
	if err != nil {
		t.Fatalf("prepare developer workspace: %v", err)
	}
	if manifest.Role != "DeveloperWorker" || manifest.Repository.InitialHeadSHA != targetSHA || manifest.Repository.DeliveryBranch != "agent/task-developer-1" {
		t.Fatalf("unexpected developer manifest: %#v", manifest)
	}
	repoRoot := filepath.Join(root, "runs", "run-developer-1", "repo")
	if got := strings.TrimSpace(runGit(t, repoRoot, "rev-parse", "HEAD")); got != targetSHA {
		t.Fatalf("expected target head %s, got %s", targetSHA, got)
	}
	if got := strings.TrimSpace(runGit(t, repoRoot, "branch", "--show-current")); got != "agent/task-developer-1" {
		t.Fatalf("unexpected local delivery branch %q", got)
	}
	if got := strings.TrimSpace(runGit(t, repoRoot, "remote", "get-url", "--push", "origin")); got != workspace.DisabledDeveloperPushURL {
		t.Fatalf("expected disabled developer push URL, got %q", got)
	}
}

func TestDeveloperPreparerUsesExactReworkSourceHead(t *testing.T) {
	origin, targetSHA, featureHead := createReviewOrigin(t)
	root := t.TempDir()
	manifest, err := workspace.NewDeveloperPreparer().Prepare(context.Background(), workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-rework-1", RunID: "run-rework-1",
		TaskType: model.TaskTypeReworkMergeRequest, Project: "group/project", RepositoryURL: origin,
		SourceBranch: "feature/review", TargetBranch: "master", DeliveryBranch: "feature/review", ExpectedHeadSHA: featureHead,
	})
	if err != nil {
		t.Fatalf("prepare rework workspace: %v", err)
	}
	if manifest.Repository.InitialHeadSHA != featureHead || manifest.Repository.TargetSHA != targetSHA || manifest.Repository.SourceBranch != "feature/review" {
		t.Fatalf("unexpected rework manifest: %#v", manifest.Repository)
	}
	repoRoot := filepath.Join(root, "runs", "run-rework-1", "repo")
	if got := strings.TrimSpace(runGit(t, repoRoot, "rev-parse", "HEAD")); got != featureHead {
		t.Fatalf("expected source head %s, got %s", featureHead, got)
	}
	if got := strings.TrimSpace(runGit(t, repoRoot, "branch", "--show-current")); got != "feature/review" {
		t.Fatalf("unexpected rework branch %q", got)
	}

	_, err = workspace.NewDeveloperPreparer().Prepare(context.Background(), workspace.DeveloperRequest{
		WorkspaceRoot: t.TempDir(), TaskID: "task-rework-drift", RunID: "run-rework-drift",
		TaskType: model.TaskTypeReworkMergeRequest, Project: "group/project", RepositoryURL: origin,
		SourceBranch: "feature/review", TargetBranch: "master", DeliveryBranch: "feature/review", ExpectedHeadSHA: targetSHA,
	})
	if !errors.Is(err, workspace.ErrHeadUnavailable) {
		t.Fatalf("expected source head drift rejection, got %v", err)
	}
}

func TestDeveloperPreparerReusesReadyWorkspaceAndRejectsUnknownDirectory(t *testing.T) {
	origin, _, _ := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-reuse", RunID: "run-reuse", TaskType: "develop_from_issue",
		Project: "group/project", RepositoryURL: origin, TargetBranch: "master", DeliveryBranch: "agent/task-reuse",
	}
	preparer := workspace.NewDeveloperPreparer()
	first, err := preparer.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := preparer.Prepare(context.Background(), request)
	if err != nil || second.PreparedAt != first.PreparedAt {
		t.Fatalf("ready workspace was not reused: first=%#v second=%#v err=%v", first, second, err)
	}

	unknownRoot := t.TempDir()
	runRoot := filepath.Join(unknownRoot, "runs", "run-unknown")
	if err := os.MkdirAll(runRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runRoot, "operator-note.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	request.WorkspaceRoot = unknownRoot
	request.RunID = "run-unknown"
	_, err = preparer.Prepare(context.Background(), request)
	if !errors.Is(err, workspace.ErrWorkspaceConflict) {
		t.Fatalf("expected unknown directory conflict, got %v", err)
	}
}
