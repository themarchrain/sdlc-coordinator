package repository

import (
	"context"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
)

func TestStartRunCopiesRecoveryGeneration(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	task := &model.Task{
		TaskID: "task-start-generation", DedupeKey: "dedupe-start-generation", Type: "bugfix", Role: "DeveloperWorker",
		Status: model.TaskStatusPending, AttemptCount: 5,
		RecoveryGeneration: 4, RetryCycleBaseCount: 5, RetryCycleMaxAttempts: 3,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	run := &model.Run{RunID: "run-start-generation", WorkerRole: task.Role, RecoveryGeneration: 99}
	current, err := NewTaskRepository(db).StartRunAtomic(ctx, task.TaskID, run, now.Add(time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	if run.RecoveryGeneration != 4 {
		t.Fatalf("run recovery generation = %d, want 4", run.RecoveryGeneration)
	}
	if current.AttemptCount != 6 || current.RecoveryGeneration != 4 || current.RetryCycleBaseCount != 5 || current.RetryCycleMaxAttempts != 3 {
		t.Fatalf("task recovery fields changed during start: %+v", current)
	}
}
