package orchestrator

import (
	"context"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

func TestApplyCanonicalResultIgnoresStaleRunForTaskState(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo := newStaleRunRepositories(t)
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	orch := New(taskRepo, runRepo, WithClock(func() time.Time { return now }))
	currentRunID := "run-new"
	task := &model.Task{
		TaskID:       "task-stale-callback",
		DedupeKey:    "dedupe-stale-callback",
		Type:         "bugfix",
		Role:         "DeveloperWorker",
		Status:       model.TaskStatusRunning,
		CurrentRunID: &currentRunID,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	oldRun := &model.Run{
		RunID:      "run-old",
		TaskID:     "task-stale-callback",
		WorkerRole: "DeveloperWorker",
		Status:     model.RunStatusRunning,
	}
	if err := runRepo.Create(ctx, oldRun); err != nil {
		t.Fatalf("create old run: %v", err)
	}
	newRun := &model.Run{
		RunID:      "run-new",
		TaskID:     "task-stale-callback",
		WorkerRole: "DeveloperWorker",
		Status:     model.RunStatusRunning,
	}
	if err := runRepo.Create(ctx, newRun); err != nil {
		t.Fatalf("create new run: %v", err)
	}

	_, appliedRun, err := orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID:  "task-stale-callback",
		RunID:   "run-old",
		Status:  results.StatusSucceeded,
		Summary: "old run finished after retry started",
	})
	if err != nil {
		t.Fatalf("apply stale callback: %v", err)
	}

	loadedTask, err := taskRepo.GetByTaskID(ctx, "task-stale-callback")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if loadedTask.Status != model.TaskStatusRunning {
		t.Fatalf("expected stale callback to keep task running, got %s", loadedTask.Status)
	}
	if loadedTask.CurrentRunID == nil || *loadedTask.CurrentRunID != "run-new" {
		t.Fatalf("expected current run to remain run-new, got %v", loadedTask.CurrentRunID)
	}
	if appliedRun.Status != model.RunStatusSucceeded {
		t.Fatalf("expected stale run to be marked succeeded for audit, got %s", appliedRun.Status)
	}
	if appliedRun.ErrorMessage != "stale run result ignored for task state" {
		t.Fatalf("expected stale run audit message, got %q", appliedRun.ErrorMessage)
	}
}

func newStaleRunRepositories(t *testing.T) (*repository.TaskRepository, *repository.RunRepository) {
	t.Helper()

	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return repository.NewTaskRepository(db), repository.NewRunRepository(db)
}
