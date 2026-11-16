package repository

import (
	"context"
	"errors"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"

	"gorm.io/gorm"
)

func openRebindDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.TaskRecovery{}, &model.StopRequest{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func rebindParkedTask(taskID string) *model.Task {
	repositoryID := "repo-original"
	return &model.Task{
		TaskID: taskID, DedupeKey: "dedupe-" + taskID, Type: "bugfix", Role: "DeveloperWorker",
		Status: model.TaskStatusBlocked, AttemptCount: 5, LastError: "repository not bound",
		RepositoryID: &repositoryID,
	}
}

func TestRebindGuardedCannotRollBackRecoveryGeneration(t *testing.T) {
	db := openRebindDB(t)
	task := rebindParkedTask("task-rebind-rollback")
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	// The admin page snapshots the task while it is still blocked in generation 0.
	stale, err := NewTaskRepository(db).GetByTaskID(context.Background(), task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	expect := RebindExpectationFromTask(stale)
	if _, _, err := NewTaskRecoveryRepository(db).Accept(context.Background(), task.TaskID, "member-1", recoveryCommand(task, "request-rebind", "hash-rebind"), recoveryProof(task)); err != nil {
		t.Fatalf("accept recovery: %v", err)
	}

	repo := NewTaskRepository(db)
	if _, err := repo.RebindGuarded(context.Background(), task.TaskID, "repo-rebound", expect); !errors.Is(err, ErrRecoveryRevisionChanged) {
		t.Fatalf("stale rebind error = %v", err)
	}
	current, err := repo.GetByTaskID(context.Background(), task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if current.RecoveryGeneration != 1 || current.Status != model.TaskStatusPending {
		t.Fatalf("stale rebind rolled back recovery: generation=%d status=%s", current.RecoveryGeneration, current.Status)
	}
	if current.RepositoryID == nil || *current.RepositoryID != "repo-original" {
		t.Fatalf("stale rebind changed repository: %v", current.RepositoryID)
	}
}

func TestRebindGuardedRequiresCurrentRunMatch(t *testing.T) {
	db := openRebindDB(t)
	task := rebindParkedTask("task-rebind-run")
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	expect := RebindExpectationFromTask(task)
	runID := "run-rebind-run"
	if err := db.Model(&model.Task{}).Where("task_id = ?", task.TaskID).Updates(map[string]any{"current_run_id": runID}).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewTaskRepository(db)
	if _, err := repo.RebindGuarded(context.Background(), task.TaskID, "repo-rebound", expect); !errors.Is(err, ErrRecoveryRevisionChanged) {
		t.Fatalf("rebind with stale current run error = %v", err)
	}
	current, err := repo.GetByTaskID(context.Background(), task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if current.CurrentRunID == nil || *current.CurrentRunID != runID {
		t.Fatalf("rebind cleared a newer current run: %v", current.CurrentRunID)
	}
}

func TestRebindGuardedOnlyTouchesBindingColumns(t *testing.T) {
	db := openRebindDB(t)
	task := rebindParkedTask("task-rebind-columns")
	task.RecoveryGeneration = 2
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewTaskRepository(db)
	rebound, err := repo.RebindGuarded(context.Background(), task.TaskID, "repo-bound", RebindExpectationFromTask(task))
	if err != nil {
		t.Fatalf("rebind guarded: %v", err)
	}
	if rebound.Status != model.TaskStatusPending || rebound.RecoveryGeneration != 2 || rebound.AttemptCount != 5 || rebound.LastError != "" {
		t.Fatalf("unexpected rebound row: %+v", rebound)
	}
	if rebound.RepositoryID == nil || *rebound.RepositoryID != "repo-bound" {
		t.Fatalf("repository not rebound: %v", rebound.RepositoryID)
	}
}

func TestRebindGuardedRejectsDisallowedStatus(t *testing.T) {
	db := openRebindDB(t)
	task := rebindParkedTask("task-rebind-running")
	task.Status = model.TaskStatusRunning
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := NewTaskRepository(db).RebindGuarded(context.Background(), task.TaskID, "repo-bound", RebindExpectationFromTask(task)); !errors.Is(err, ErrTaskNotClaimable) {
		t.Fatalf("running rebind error = %v", err)
	}
}
