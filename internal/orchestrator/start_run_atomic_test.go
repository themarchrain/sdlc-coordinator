package orchestrator

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

func TestStartRunRollsBackClaimWhenRunCreationFails(t *testing.T) {
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tasks, runs := repository.NewTaskRepository(db), repository.NewRunRepository(db)
	task := &model.Task{TaskID: "atomic-task", DedupeKey: "atomic", Type: "develop_from_issue", Role: "DeveloperWorker", AttemptCount: 2}
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := runs.Create(ctx, &model.Run{RunID: "existing-run", TaskID: "other-task", WorkerRole: task.Role}); err != nil {
		t.Fatal(err)
	}
	_, _, err = New(tasks, runs).StartRun(ctx, task.TaskID, &model.Run{RunID: "existing-run", WorkerRole: task.Role})
	if err == nil {
		t.Fatal("duplicate Run identity must reject start")
	}
	current, err := tasks.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != model.TaskStatusPending || current.CurrentRunID != nil || current.LeaseUntil != nil || current.AttemptCount != 2 {
		t.Fatalf("failed Run creation leaked a claim: %+v", current)
	}
}
