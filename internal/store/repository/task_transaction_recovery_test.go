package repository

import (
	"context"
	"errors"
	"strings"
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

func TestStartRunRefusesExhaustedRecoveryCycle(t *testing.T) {
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
		TaskID: "task-start-exhausted", DedupeKey: "dedupe-start-exhausted", Type: "bugfix", Role: "DeveloperWorker",
		Status: model.TaskStatusPending, AttemptCount: 8,
		RecoveryGeneration: 1, RetryCycleBaseCount: 5, RetryCycleMaxAttempts: 3,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	run := &model.Run{RunID: "run-start-exhausted", WorkerRole: task.Role}
	if _, err := NewTaskRepository(db).StartRunAtomic(ctx, task.TaskID, run, now.Add(time.Hour), now); !errors.Is(err, ErrRecoveryBudgetExhausted) {
		t.Fatalf("start with an exhausted cycle error = %v", err)
	}
	var runs int64
	if err := db.Model(&model.Run{}).Where("task_id = ?", task.TaskID).Count(&runs).Error; err != nil || runs != 0 {
		t.Fatalf("exhausted cycle still created a run: count=%d err=%v", runs, err)
	}
	current, err := NewTaskRepository(db).GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if current.AttemptCount != 8 {
		t.Fatalf("refused start changed the attempt count: %+v", current)
	}
	// 留在 pending 会被下一次 Tick 反复拾取并再次拒绝，等于活锁。
	if current.Status != model.TaskStatusBlocked {
		t.Fatalf("exhausted cycle left the task in %s: %+v", current.Status, current)
	}
	if !strings.Contains(current.LastError, "额度") {
		t.Fatalf("exhausted cycle left no operator-facing reason: %q", current.LastError)
	}
}

func TestStartRunAllowsTheLastRecoveryCycleAttempt(t *testing.T) {
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
		TaskID: "task-start-last", DedupeKey: "dedupe-start-last", Type: "bugfix", Role: "DeveloperWorker",
		Status: model.TaskStatusPending, AttemptCount: 7,
		RecoveryGeneration: 1, RetryCycleBaseCount: 5, RetryCycleMaxAttempts: 3,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	run := &model.Run{RunID: "run-start-last", WorkerRole: task.Role}
	current, err := NewTaskRepository(db).StartRunAtomic(ctx, task.TaskID, run, now.Add(time.Hour), now)
	if err != nil {
		t.Fatalf("last cycle attempt was refused: %v", err)
	}
	if current.AttemptCount != 8 {
		t.Fatalf("attempt count = %d, want 8", current.AttemptCount)
	}
}
