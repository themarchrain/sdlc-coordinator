package workspace_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/workspace"
)

func TestDeveloperFinalizerCommitsPushesAndReusesDelivery(t *testing.T) {
	origin, targetSHA, _ := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-finalize", RunID: "run-finalize", TaskType: "develop_from_issue",
		Project: "group/project", RepositoryURL: origin, TargetBranch: "master", DeliveryBranch: "agent/task-finalize",
	}
	if _, err := workspace.NewDeveloperPreparer().Prepare(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Join(root, "runs", request.RunID, "repo")
	if err := os.WriteFile(filepath.Join(repoRoot, "fix.go"), []byte("package fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finalizer := workspace.NewDeveloperFinalizer()
	delivery, err := finalizer.Finalize(context.Background(), workspace.DeveloperFinalizeRequest{
		WorkspaceRoot: root, TaskID: request.TaskID, RunID: request.RunID, Summary: "Implement requested fix",
		TaskType: request.TaskType, RepositoryURL: request.RepositoryURL, TargetBranch: request.TargetBranch,
		DeliveryBranch: request.DeliveryBranch, ExpectedInitialHeadSHA: targetSHA,
	})
	if err != nil {
		t.Fatalf("finalize developer delivery: %v", err)
	}
	if delivery.CommitSHA == "" || len(delivery.ChangedFiles) != 1 || delivery.ChangedFiles[0] != "fix.go" || strings.TrimSpace(delivery.DiffSummary) == "" {
		t.Fatalf("unexpected delivery: %#v", delivery)
	}
	if got := strings.TrimSpace(runGit(t, repoRoot, "status", "--porcelain")); got != "" {
		t.Fatalf("worktree is not clean: %q", got)
	}
	if got := strings.TrimSpace(runGit(t, origin, "rev-parse", "refs/heads/agent/task-finalize")); got != delivery.CommitSHA {
		t.Fatalf("remote branch does not match delivery: %s != %s", got, delivery.CommitSHA)
	}
	if parent := strings.TrimSpace(runGit(t, repoRoot, "rev-parse", "HEAD^")); parent != targetSHA {
		t.Fatalf("delivery commit parent mismatch: %s != %s", parent, targetSHA)
	}
	message := runGit(t, repoRoot, "log", "-1", "--format=%B")
	if !strings.Contains(message, "Coordinator-Run-ID: "+request.RunID) {
		t.Fatalf("missing run trailer: %s", message)
	}

	replayed, err := finalizer.Finalize(context.Background(), workspace.DeveloperFinalizeRequest{
		WorkspaceRoot: root, TaskID: request.TaskID, RunID: request.RunID, Summary: "Implement requested fix",
		TaskType: request.TaskType, RepositoryURL: request.RepositoryURL, TargetBranch: request.TargetBranch,
		DeliveryBranch: request.DeliveryBranch, ExpectedInitialHeadSHA: targetSHA,
	})
	if err != nil || replayed.CommitSHA != delivery.CommitSHA {
		t.Fatalf("delivery was not idempotent: first=%#v replay=%#v err=%v", delivery, replayed, err)
	}
	if count := strings.TrimSpace(runGit(t, repoRoot, "rev-list", "--count", targetSHA+"..HEAD")); count != "1" {
		t.Fatalf("finalization created duplicate commits: %s", count)
	}
}

func TestDeveloperFinalizerFastForwardsReworkBranch(t *testing.T) {
	origin, _, featureHead := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-rework-finalize", RunID: "run-rework-finalize", TaskType: model.TaskTypeReworkMergeRequest,
		Project: "group/project", RepositoryURL: origin, SourceBranch: "feature/review", TargetBranch: "master",
		DeliveryBranch: "feature/review", ExpectedHeadSHA: featureHead,
	}
	if _, err := workspace.NewDeveloperPreparer().Prepare(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Join(root, "runs", request.RunID, "repo")
	if err := os.WriteFile(filepath.Join(repoRoot, "rework.txt"), []byte("reworked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	delivery, err := workspace.NewDeveloperFinalizer().Finalize(context.Background(), workspace.DeveloperFinalizeRequest{
		WorkspaceRoot: root, TaskID: request.TaskID, RunID: request.RunID, Summary: "Apply review feedback",
		TaskType: request.TaskType, RepositoryURL: request.RepositoryURL, TargetBranch: request.TargetBranch,
		DeliveryBranch: request.DeliveryBranch, ExpectedInitialHeadSHA: featureHead,
	})
	if err != nil {
		t.Fatalf("finalize rework: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, origin, "rev-parse", "refs/heads/feature/review")); got != delivery.CommitSHA {
		t.Fatalf("rework branch was not updated: %s != %s", got, delivery.CommitSHA)
	}
	if parent := strings.TrimSpace(runGit(t, repoRoot, "rev-parse", "HEAD^")); parent != featureHead {
		t.Fatalf("rework was not a fast-forward child: %s != %s", parent, featureHead)
	}
}

func TestDeveloperFinalizerRejectsNoChangesAndRemoteDrift(t *testing.T) {
	origin, targetSHA, featureHead := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-conflict", RunID: "run-conflict", TaskType: "develop_from_issue",
		Project: "group/project", RepositoryURL: origin, TargetBranch: "master", DeliveryBranch: "agent/task-conflict",
	}
	if _, err := workspace.NewDeveloperPreparer().Prepare(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	finalizer := workspace.NewDeveloperFinalizer()
	_, err := finalizer.Finalize(context.Background(), workspace.DeveloperFinalizeRequest{
		WorkspaceRoot: root, TaskID: request.TaskID, RunID: request.RunID, Summary: "No changes",
		TaskType: request.TaskType, RepositoryURL: request.RepositoryURL, TargetBranch: request.TargetBranch,
		DeliveryBranch: request.DeliveryBranch, ExpectedInitialHeadSHA: targetSHA,
	})
	if !errors.Is(err, workspace.ErrNoDeveloperChanges) {
		t.Fatalf("expected no-change rejection, got %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "runs", request.RunID, "repo", "fix.go"), []byte("package fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, origin, "update-ref", "refs/heads/agent/task-conflict", featureHead)
	_, err = finalizer.Finalize(context.Background(), workspace.DeveloperFinalizeRequest{
		WorkspaceRoot: root, TaskID: request.TaskID, RunID: request.RunID, Summary: "Conflicting fix",
		TaskType: request.TaskType, RepositoryURL: request.RepositoryURL, TargetBranch: request.TargetBranch,
		DeliveryBranch: request.DeliveryBranch, ExpectedInitialHeadSHA: targetSHA,
	})
	if !errors.Is(err, workspace.ErrDeveloperDeliveryConflict) {
		t.Fatalf("expected remote drift conflict, got %v", err)
	}
	if got := strings.TrimSpace(runGit(t, origin, "rev-parse", "refs/heads/agent/task-conflict")); got != featureHead {
		t.Fatalf("conflicting remote branch was overwritten: %s", got)
	}
}

func TestDeveloperFinalizerRejectsMutatedManifestDeliveryMetadata(t *testing.T) {
	origin, targetSHA, _ := createReviewOrigin(t)
	attackerOrigin, _, _ := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-manifest-tamper", RunID: "run-manifest-tamper", TaskType: "develop_from_issue",
		Project: "group/project", RepositoryURL: origin, TargetBranch: "master", DeliveryBranch: "agent/task-manifest-tamper",
	}
	if _, err := workspace.NewDeveloperPreparer().Prepare(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Join(root, "runs", request.RunID, "repo")
	if err := os.WriteFile(filepath.Join(repoRoot, "fix.go"), []byte("package fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(root, "runs", request.RunID, workspace.ManifestFileName)
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest workspace.Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Repository.RepositoryURL = attackerOrigin
	manifest.Repository.DeliveryBranch = "agent/hijacked"
	raw, err = json.Marshal(&manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = workspace.NewDeveloperFinalizer().Finalize(context.Background(), workspace.DeveloperFinalizeRequest{
		WorkspaceRoot: root, TaskID: request.TaskID, RunID: request.RunID, Summary: "Implement requested fix",
		TaskType: request.TaskType, RepositoryURL: request.RepositoryURL, TargetBranch: request.TargetBranch,
		DeliveryBranch: request.DeliveryBranch, ExpectedInitialHeadSHA: targetSHA,
	})
	if !errors.Is(err, workspace.ErrWorkspaceConflict) {
		t.Fatalf("expected mutated manifest to be rejected, got %v", err)
	}
	command := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/agent/hijacked")
	command.Dir = attackerOrigin
	if err := command.Run(); err == nil {
		t.Fatal("mutated manifest caused Coordinator to push to an untrusted remote branch")
	}
}
