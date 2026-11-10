package repository

import (
	"context"
	"errors"
	"sync"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"

	"gorm.io/gorm"
)

func TestRecoverBlockedTaskAtomic(t *testing.T) {
	db := openRecoveryAcceptDB(t)
	task := recoveryAcceptTask("task-recover-atomic")
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewTaskRecoveryRepository(db)
	proof := recoveryProof(task)
	command := recoveryCommand(task, "request-atomic", "hash-atomic")
	record, replay, err := repo.Accept(context.Background(), task.TaskID, "member-1", command, proof)
	if err != nil {
		t.Fatalf("accept recovery: %v", err)
	}
	if replay || record == nil || record.SourceGeneration != 0 || record.TargetGeneration != 1 || record.AttemptCountBefore != 5 || record.CycleMaxAttempts != 3 {
		t.Fatalf("unexpected recovery record: %+v replay=%v", record, replay)
	}
	current, err := NewTaskRepository(db).GetByTaskID(context.Background(), task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != model.TaskStatusPending || current.RecoveryGeneration != 1 || current.RetryCycleBaseCount != 5 || current.RetryCycleMaxAttempts != 3 || current.CurrentRunID != nil || current.LeaseUntil != nil || current.LastError != "" {
		t.Fatalf("task recovery state = %+v", current)
	}
	var runs int64
	if err := db.Model(&model.Run{}).Where("task_id = ?", task.TaskID).Count(&runs).Error; err != nil || runs != 0 {
		t.Fatalf("recovery created runs: count=%d err=%v", runs, err)
	}
}

func TestRecoveryAuditFailureRollsBack(t *testing.T) {
	db := openRecoveryAcceptDB(t)
	task := recoveryAcceptTask("task-recover-rollback")
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	callbackName := "fail_task_recovery_audit"
	if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "task_recoveries" {
			tx.AddError(errors.New("audit insert failed"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callbackName) })
	repo := NewTaskRecoveryRepository(db)
	if _, _, err := repo.Accept(context.Background(), task.TaskID, "member-1", recoveryCommand(task, "request-rollback", "hash-rollback"), recoveryProof(task)); err == nil {
		t.Fatal("audit failure must surface")
	}
	current, err := NewTaskRepository(db).GetByTaskID(context.Background(), task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != model.TaskStatusBlocked || current.RecoveryGeneration != 0 || current.AttemptCount != 5 {
		t.Fatalf("audit failure changed task: %+v", current)
	}
	if _, err := repo.GetByRequest(context.Background(), task.TaskID, "request-rollback"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("audit row survived rollback: %v", err)
	}
}

func TestRecoveryRequiresUnknownAckAndReason(t *testing.T) {
	db := openRecoveryAcceptDB(t)
	task := recoveryAcceptTask("task-recover-risk")
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewTaskRecoveryRepository(db)
	command := recoveryCommand(task, "request-risk", "hash-risk")
	command.Classification = "unknown"
	if _, _, err := repo.Accept(context.Background(), task.TaskID, "member-1", command, recoveryProof(task)); !errors.Is(err, ErrRecoveryRiskRequired) {
		t.Fatalf("missing risk acknowledgement error = %v", err)
	}
	command.RiskAcknowledged = true
	command.Reason = ""
	if _, _, err := repo.Accept(context.Background(), task.TaskID, "member-1", command, recoveryProof(task)); !errors.Is(err, ErrRecoveryRiskRequired) {
		t.Fatalf("blank reason error = %v", err)
	}
	current, _ := NewTaskRepository(db).GetByTaskID(context.Background(), task.TaskID)
	if current.Status != model.TaskStatusBlocked || current.RecoveryGeneration != 0 {
		t.Fatalf("risk validation changed task: %+v", current)
	}
}

func TestFatalCannotForceRecover(t *testing.T) {
	db := openRecoveryAcceptDB(t)
	task := recoveryAcceptTask("task-recover-fatal")
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewTaskRecoveryRepository(db)
	command := recoveryCommand(task, "request-fatal", "hash-fatal")
	command.Classification = "fatal"
	if _, _, err := repo.Accept(context.Background(), task.TaskID, "member-1", command, recoveryProof(task)); !errors.Is(err, ErrRecoveryNotEligible) {
		t.Fatalf("fatal recovery error = %v", err)
	}
}

func TestRecoveryRequestReplayDoesNotResetBudget(t *testing.T) {
	db := openRecoveryAcceptDB(t)
	task := recoveryAcceptTask("task-recover-replay")
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewTaskRecoveryRepository(db)
	command := recoveryCommand(task, "request-replay", "hash-replay")
	proof := recoveryProof(task)
	first, replay, err := repo.Accept(context.Background(), task.TaskID, "member-1", command, proof)
	if err != nil || replay {
		t.Fatalf("first acceptance: record=%+v replay=%v err=%v", first, replay, err)
	}
	replayed, replay, err := repo.Accept(context.Background(), task.TaskID, "member-1", command, proof)
	if err != nil || !replay || replayed.RecoveryID != first.RecoveryID {
		t.Fatalf("replay = %+v,%v,%v", replayed, replay, err)
	}
	current, _ := NewTaskRepository(db).GetByTaskID(context.Background(), task.TaskID)
	if current.RecoveryGeneration != 1 || current.RetryCycleMaxAttempts != 3 {
		t.Fatalf("replay reset budget: %+v", current)
	}
	changed := command
	changed.Reason = "different reason"
	if _, _, err := repo.Accept(context.Background(), task.TaskID, "member-1", changed, proof); !errors.Is(err, ErrRecoveryRequestConflict) {
		t.Fatalf("different params error = %v", err)
	}
}

func TestRecoveryABARejectsOldRevision(t *testing.T) {
	db := openRecoveryAcceptDB(t)
	task := recoveryAcceptTask("task-recover-aba")
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewTaskRecoveryRepository(db)
	proof := recoveryProof(task)
	if _, _, err := repo.Accept(context.Background(), task.TaskID, "member-1", recoveryCommand(task, "request-aba-1", "hash-aba-1"), proof); err != nil {
		t.Fatal(err)
	}
	next := recoveryCommand(task, "request-aba-2", "hash-aba-2")
	if _, _, err := repo.Accept(context.Background(), task.TaskID, "member-1", next, proof); !errors.Is(err, ErrRecoveryStateChanged) {
		t.Fatalf("old page after pending error = %v", err)
	}
	if err := db.Model(&model.Task{}).Where("task_id = ?", task.TaskID).Updates(map[string]any{"status": model.TaskStatusBlocked, "recovery_generation": 1, "attempt_count": 6}).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.Accept(context.Background(), task.TaskID, "member-1", recoveryCommand(task, "request-aba-3", "hash-aba-3"), proof); !errors.Is(err, ErrRecoveryRevisionChanged) {
		t.Fatalf("ABA old revision error = %v", err)
	}
}

func TestRecoveryConcurrentRequestsOneWinner(t *testing.T) {
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
	if err := store.Migrate(first, &model.Task{}, &model.Run{}, &model.TaskRecovery{}, &model.StopRequest{}); err != nil {
		t.Fatal(err)
	}
	task := recoveryAcceptTask("task-recover-race")
	if err := first.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	proof := recoveryProof(task)
	start := make(chan struct{})
	type result struct {
		record *model.TaskRecovery
		replay bool
		err    error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for index, db := range []*gorm.DB{first, second} {
		wg.Add(1)
		go func(index int, db *gorm.DB) {
			defer wg.Done()
			<-start
			repo := NewTaskRecoveryRepository(db)
			command := recoveryCommand(task, "request-race-"+string(rune('a'+index)), "hash-race-"+string(rune('a'+index)))
			record, replay, err := repo.Accept(context.Background(), task.TaskID, "member-"+string(rune('a'+index)), command, proof)
			results <- result{record: record, replay: replay, err: err}
		}(index, db)
	}
	close(start)
	wg.Wait()
	close(results)
	wins := 0
	for result := range results {
		if result.err == nil && !result.replay {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("successful recovery accepts = %d, want 1", wins)
	}
	count, err := NewTaskRecoveryRepository(first).GetByRequest(context.Background(), task.TaskID, "request-race-a")
	if err == nil && count != nil {
		return
	}
	if _, err := NewTaskRecoveryRepository(first).GetByRequest(context.Background(), task.TaskID, "request-race-b"); err != nil {
		t.Fatalf("winner audit missing: %v", err)
	}
}

func TestLegacyResumeDoesNotBypassRecoveryGeneration(t *testing.T) {
	db := openRecoveryAcceptDB(t)
	human := &model.Task{
		TaskID: "task-recover-resume-human", DedupeKey: "dedupe-resume-human", Type: "bugfix", Role: "DeveloperWorker",
		Status: model.TaskStatusHumanRequired, RecoveryGeneration: 2,
	}
	rework := &model.Task{
		TaskID: "task-recover-resume-rework", DedupeKey: "dedupe-resume-rework", Type: "rework_merge_request", Role: "DeveloperWorker",
		Status: model.TaskStatusBlocked, RecoveryGeneration: 3,
	}
	if err := db.Create(human).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(rework).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewTaskRepository(db)
	if err := repo.ResumeHumanRequired(context.Background(), human.DedupeKey, `{}`); !errors.Is(err, ErrTaskNotClaimable) {
		t.Fatalf("legacy human resume bypassed generation: %v", err)
	}
	if err := repo.ResumeRework(context.Background(), rework.DedupeKey, `{}`); !errors.Is(err, ErrTaskNotClaimable) {
		t.Fatalf("legacy rework resume bypassed generation: %v", err)
	}
}

func TestRecoveryVersusStop(t *testing.T) {
	db := openRecoveryAcceptDB(t)
	stops := NewStopRepository(db)
	blocked := recoveryAcceptTask("task-recover-stop-first")
	if err := db.Create(blocked).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := stops.Stop(context.Background(), StopTarget{TaskID: blocked.TaskID}, "admin", "manual stop", 3); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewTaskRecoveryRepository(db).Accept(context.Background(), blocked.TaskID, "member-1", recoveryCommand(blocked, "request-stop-first", "hash-stop-first"), recoveryProof(blocked)); !errors.Is(err, ErrRecoveryStateChanged) {
		t.Fatalf("stop-first recovery error = %v", err)
	}

	accepted := recoveryAcceptTask("task-recover-stop-second")
	if err := db.Create(accepted).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewTaskRecoveryRepository(db).Accept(context.Background(), accepted.TaskID, "member-1", recoveryCommand(accepted, "request-stop-second", "hash-stop-second"), recoveryProof(accepted)); err != nil {
		t.Fatal(err)
	}
	if _, err := stops.Stop(context.Background(), StopTarget{TaskID: accepted.TaskID}, "admin", "manual stop after recovery", 3); err != nil {
		t.Fatal(err)
	}
	current, _ := NewTaskRepository(db).GetByTaskID(context.Background(), accepted.TaskID)
	if current.Status != model.TaskStatusInterrupted {
		t.Fatalf("stop after recovery status = %s", current.Status)
	}
}

func openRecoveryAcceptDB(t *testing.T) *gorm.DB {
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

func recoveryAcceptTask(taskID string) *model.Task {
	return &model.Task{
		TaskID: taskID, DedupeKey: "dedupe-" + taskID, Type: "bugfix", Role: "DeveloperWorker",
		Status: model.TaskStatusBlocked, AttemptCount: 5, LastError: "previous failure",
	}
}

func recoveryProof(task *model.Task) RecoveryDBProof {
	return RecoveryDBProof{
		TaskID: task.TaskID, TaskVersion: RecoveryTaskVersion(task), RecoveryGeneration: task.RecoveryGeneration,
		AttemptCount: task.AttemptCount,
	}
}

func recoveryCommand(task *model.Task, requestID, requestHash string) RecoveryCommand {
	return RecoveryCommand{
		RequestID: requestID, RequestHash: requestHash, BlockRevision: RecoveryBlockRevision(task, ""),
		Reason: "retry after transient failure", ExpectedGeneration: task.RecoveryGeneration, CycleMaxAttempts: 3,
		Classification: "transient", FailureSnapshotJSON: `{"version":1,"code":"provider_rate_limited"}`,
	}
}
