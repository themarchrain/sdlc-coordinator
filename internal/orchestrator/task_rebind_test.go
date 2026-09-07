package orchestrator

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/model"
)

func TestRebindTaskRestoresParkedUnboundTask(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo := newTickRepositories(t)
	orch := New(taskRepo, runRepo,
		WithRepositoryLookup(fakeRepositoryLookup{}),
	)

	task := &model.Task{
		TaskID: "task-parked", DedupeKey: "dedupe-parked", Type: "bugfix", Role: "DeveloperWorker",
		Status: model.TaskStatusBlocked, LastError: "repository not bound",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create parked task: %v", err)
	}

	rebound, err := orch.RebindTask(ctx, "task-parked", "repo-bound")
	if err != nil {
		t.Fatalf("rebind task: %v", err)
	}
	if rebound.Status != model.TaskStatusPending || rebound.LastError != "" {
		t.Fatalf("expected rebound task pending with cleared last_error, got status=%s last_error=%q", rebound.Status, rebound.LastError)
	}
	if rebound.RepositoryID == nil || *rebound.RepositoryID != "repo-bound" {
		t.Fatalf("expected repository_id repo-bound, got %v", rebound.RepositoryID)
	}
}

func TestRebindTaskRejectsUnknownRepository(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo := newTickRepositories(t)
	orch := New(taskRepo, runRepo,
		WithRepositoryLookup(fakeRepositoryLookup{}),
	)

	task := &model.Task{
		TaskID: "task-parked", DedupeKey: "dedupe-parked", Type: "bugfix", Role: "DeveloperWorker",
		Status: model.TaskStatusBlocked,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create parked task: %v", err)
	}
	if _, err := orch.RebindTask(ctx, "task-parked", "repo-unknown"); err == nil {
		t.Fatal("expected rebind to reject unknown repository")
	}
}
