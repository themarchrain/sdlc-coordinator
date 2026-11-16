package repository

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"

	"gorm.io/gorm"
)

func openExitConfirmationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.ExecutionExitConfirmation{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func exitConfirmationFixture(t *testing.T, db *gorm.DB, taskID string, generation uint64) (*model.Task, *model.AgentExecution) {
	t.Helper()
	runID := "run-" + taskID
	task := &model.Task{
		TaskID: taskID, DedupeKey: "dedupe-" + taskID, Type: "bugfix", Role: "DeveloperWorker",
		Status: model.TaskStatusBlocked, RecoveryGeneration: generation, CurrentRunID: &runID, AttemptCount: 5,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	finished := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	execution := &model.AgentExecution{
		RunID: runID, ExecutionID: runID, Provider: "codex", Status: model.AgentExecutionStatusInterrupted,
		ErrorCode: "runner_restarted", RequestHash: "request-hash", FinishedAt: &finished,
	}
	if err := db.Create(execution).Error; err != nil {
		t.Fatal(err)
	}
	return task, execution
}

func exitConfirmationFor(task *model.Task, execution *model.AgentExecution, requestID string) *model.ExecutionExitConfirmation {
	return &model.ExecutionExitConfirmation{
		ConfirmationID:       "confirmation-" + requestID,
		TaskID:               task.TaskID,
		RunID:                execution.RunID,
		ExecutionID:          execution.ExecutionID,
		SourceGeneration:     task.RecoveryGeneration,
		ExecutionFingerprint: model.ExecutionExitFingerprint(execution),
		ExecutionStatus:      string(execution.Status),
		ExecutionErrorCode:   execution.ErrorCode,
		ExecutionFinishedAt:  execution.FinishedAt,
		ExecutionRequestHash: execution.RequestHash,
		RemoteState:          model.ExecutionExitRemoteTerminal,
		RequestID:            requestID,
		RequestedBy:          "tenant-admin-1",
		Reason:               "confirmed the old process is gone",
		ConfirmedAt:          time.Date(2026, 11, 16, 10, 5, 0, 0, time.UTC),
	}
}

func TestExecutionExitConfirmationRequiresMatchingFingerprint(t *testing.T) {
	db := openExitConfirmationDB(t)
	task, execution := exitConfirmationFixture(t, db, "task-exit-fingerprint", 1)
	stale := exitConfirmationFor(task, execution, "request-stale")
	if err := db.Model(&model.AgentExecution{}).Where("run_id = ?", execution.RunID).
		Updates(map[string]any{"status": model.AgentExecutionStatusFailed, "error_code": "provider_failed"}).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewExecutionExitConfirmationRepository(db)
	if _, _, err := repo.Create(context.Background(), stale); !errors.Is(err, ErrExecutionFingerprintChanged) {
		t.Fatalf("stale fingerprint error = %v", err)
	}
	var count int64
	if err := db.Model(&model.ExecutionExitConfirmation{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("stale confirmation was stored: count=%d err=%v", count, err)
	}
}

func TestExecutionExitConfirmationRequiresCurrentGeneration(t *testing.T) {
	db := openExitConfirmationDB(t)
	task, execution := exitConfirmationFixture(t, db, "task-exit-generation", 2)
	stale := exitConfirmationFor(task, execution, "request-generation")
	stale.SourceGeneration = 1
	if _, _, err := NewExecutionExitConfirmationRepository(db).Create(context.Background(), stale); !errors.Is(err, ErrRecoveryGenerationMismatch) {
		t.Fatalf("stale generation error = %v", err)
	}
}

func TestExecutionExitConfirmationRejectsNonInterruptedExecution(t *testing.T) {
	db := openExitConfirmationDB(t)
	task, execution := exitConfirmationFixture(t, db, "task-exit-running", 1)
	confirmation := exitConfirmationFor(task, execution, "request-running")
	execution.Status = model.AgentExecutionStatusRunning
	confirmation.ExecutionFingerprint = model.ExecutionExitFingerprint(execution)
	if err := db.Model(&model.AgentExecution{}).Where("run_id = ?", execution.RunID).
		Updates(map[string]any{"status": model.AgentExecutionStatusRunning}).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewExecutionExitConfirmationRepository(db).Create(context.Background(), confirmation); !errors.Is(err, ErrExecutionNotInterrupted) {
		t.Fatalf("running execution error = %v", err)
	}
}

func TestExecutionExitConfirmationRequestReplay(t *testing.T) {
	db := openExitConfirmationDB(t)
	task, execution := exitConfirmationFixture(t, db, "task-exit-replay", 1)
	repo := NewExecutionExitConfirmationRepository(db)
	confirmation := exitConfirmationFor(task, execution, "request-replay")
	first, replay, err := repo.Create(context.Background(), confirmation)
	if err != nil || replay {
		t.Fatalf("first confirmation: replay=%v err=%v", replay, err)
	}
	again, replay, err := repo.Create(context.Background(), confirmation)
	if err != nil || !replay || again.ConfirmationID != first.ConfirmationID {
		t.Fatalf("replay = %+v,%v,%v", again, replay, err)
	}
	changed := *confirmation
	changed.Reason = "another reason"
	if _, _, err := repo.Create(context.Background(), &changed); !errors.Is(err, ErrRecoveryRequestConflict) {
		t.Fatalf("changed replay error = %v", err)
	}
	var count int64
	if err := db.Model(&model.ExecutionExitConfirmation{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("replay duplicated the confirmation: count=%d err=%v", count, err)
	}
}

func TestExecutionExitConfirmationConcurrentConfirmsOneRecord(t *testing.T) {
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	first, err := store.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, db := range []*gorm.DB{first, second} {
			if sqlDB, err := db.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
	})
	if err := store.Migrate(first, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.ExecutionExitConfirmation{}); err != nil {
		t.Fatal(err)
	}
	task, execution := exitConfirmationFixture(t, first, "task-exit-race", 1)
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for index, db := range []*gorm.DB{first, second} {
		wg.Add(1)
		go func(index int, db *gorm.DB) {
			defer wg.Done()
			<-start
			requestID := "request-race-" + string(rune('a'+index))
			confirmation := exitConfirmationFor(task, execution, requestID)
			_, _, err := NewExecutionExitConfirmationRepository(db).Create(context.Background(), confirmation)
			errs <- err
		}(index, db)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent confirmation error = %v", err)
		}
	}
	var count int64
	if err := first.Model(&model.ExecutionExitConfirmation{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("confirmations stored = %d, err=%v, want 1", count, err)
	}
}
