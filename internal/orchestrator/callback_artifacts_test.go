package orchestrator

import (
	"context"
	"errors"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

func TestApplyCanonicalResultMaterializesArtifacts(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, artifactRepo := newCallbackArtifactRepositories(t)
	orch := New(taskRepo, runRepo, WithArtifactRepository(artifactRepo))
	createCallbackArtifactFixture(t, ctx, taskRepo, runRepo, "task-callback-artifacts", "run-callback-artifacts")

	_, _, err := orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID:  "task-callback-artifacts",
		RunID:   "run-callback-artifacts",
		Status:  results.StatusSucceeded,
		Summary: "created MR and passed tests",
		Artifacts: map[string]any{
			"mr_url":       "https://gitlab.example/group/project/-/merge_requests/1",
			"branch":       "feature/fix-framework",
			"test_results": map[string]any{"passed": true},
		},
	})
	if err != nil {
		t.Fatalf("apply callback: %v", err)
	}

	artifacts, err := artifactRepo.ListByRunID(ctx, "run-callback-artifacts")
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(artifacts) != 3 {
		t.Fatalf("expected three artifacts, got %d", len(artifacts))
	}

	byName := artifactsByName(artifacts)
	if byName["mr_url"].Kind != "merge_request" {
		t.Fatalf("expected merge_request artifact, got %s", byName["mr_url"].Kind)
	}
	if byName["mr_url"].URL != "https://gitlab.example/group/project/-/merge_requests/1" {
		t.Fatalf("unexpected MR URL: %s", byName["mr_url"].URL)
	}
	if byName["branch"].Kind != "branch" {
		t.Fatalf("expected branch artifact, got %s", byName["branch"].Kind)
	}
	if byName["test_results"].Kind != "test_result" {
		t.Fatalf("expected test_result artifact, got %s", byName["test_results"].Kind)
	}
}

func TestApplyCanonicalResultMaterializesArtifactsIdempotently(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, artifactRepo := newCallbackArtifactRepositories(t)
	orch := New(taskRepo, runRepo, WithArtifactRepository(artifactRepo))
	createCallbackArtifactFixture(t, ctx, taskRepo, runRepo, "task-callback-artifacts-idempotent", "run-callback-artifacts-idempotent")

	payload := results.CanonicalResult{
		TaskID: "task-callback-artifacts-idempotent",
		RunID:  "run-callback-artifacts-idempotent",
		Status: results.StatusSucceeded,
		Artifacts: map[string]any{
			"mr_url": "https://gitlab.example/group/project/-/merge_requests/1",
		},
	}
	if _, _, err := orch.ApplyCanonicalResult(ctx, payload); err != nil {
		t.Fatalf("first apply callback: %v", err)
	}
	payload.Artifacts["mr_url"] = "https://gitlab.example/group/project/-/merge_requests/2"
	if _, _, err := orch.ApplyCanonicalResult(ctx, payload); err != nil {
		t.Fatalf("second apply callback: %v", err)
	}

	artifacts, err := artifactRepo.ListByRunID(ctx, "run-callback-artifacts-idempotent")
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected one artifact after repeated callback, got %d", len(artifacts))
	}
	if artifacts[0].URL != "https://gitlab.example/group/project/-/merge_requests/2" {
		t.Fatalf("expected upserted URL, got %s", artifacts[0].URL)
	}
}

func TestApplyCanonicalResultArtifactFailureDoesNotAdvanceTaskOrRun(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, _ := newCallbackArtifactRepositories(t)
	orch := New(taskRepo, runRepo, WithArtifactRepository(failingArtifactRepository{}))
	createCallbackArtifactFixture(t, ctx, taskRepo, runRepo, "task-callback-artifacts-fail", "run-callback-artifacts-fail")

	_, _, err := orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID: "task-callback-artifacts-fail",
		RunID:  "run-callback-artifacts-fail",
		Status: results.StatusSucceeded,
		Artifacts: map[string]any{
			"mr_url": "https://gitlab.example/group/project/-/merge_requests/1",
		},
	})
	if err == nil {
		t.Fatal("expected artifact write failure")
	}

	task, err := taskRepo.GetByTaskID(ctx, "task-callback-artifacts-fail")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.Status != model.TaskStatusRunning {
		t.Fatalf("expected task to remain running, got %s", task.Status)
	}

	run, err := runRepo.GetByRunID(ctx, "run-callback-artifacts-fail")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != model.RunStatusRunning {
		t.Fatalf("expected run to remain running, got %s", run.Status)
	}
	if run.OutputJSON != "" {
		t.Fatalf("expected run output json to stay empty, got %s", run.OutputJSON)
	}
}

type failingArtifactRepository struct{}

func (f failingArtifactRepository) UpsertByIdentity(ctx context.Context, artifact *model.Artifact) error {
	return errors.New("artifact repository unavailable")
}

func newCallbackArtifactRepositories(t *testing.T) (*repository.TaskRepository, *repository.RunRepository, *repository.ArtifactRepository) {
	t.Helper()

	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Artifact{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return repository.NewTaskRepository(db), repository.NewRunRepository(db), repository.NewArtifactRepository(db)
}

func createCallbackArtifactFixture(
	t *testing.T,
	ctx context.Context,
	taskRepo *repository.TaskRepository,
	runRepo *repository.RunRepository,
	taskID string,
	runID string,
) {
	t.Helper()

	currentRunID := runID
	repositoryID := fixtureRepositoryID
	task := &model.Task{
		TaskID:       taskID,
		DedupeKey:    "dedupe-" + taskID,
		Type:         "bugfix",
		Role:         "DeveloperWorker",
		RepositoryID: &repositoryID,
		Status:       model.TaskStatusRunning,
		CurrentRunID: &currentRunID,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task fixture: %v", err)
	}

	run := &model.Run{
		RunID:      runID,
		TaskID:     taskID,
		WorkerRole: "DeveloperWorker",
		Status:     model.RunStatusRunning,
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("create run fixture: %v", err)
	}
}
