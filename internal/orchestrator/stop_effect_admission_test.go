package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"

	"gorm.io/gorm"
)

func externalAdmissionFixture(t *testing.T) (*gorm.DB, *Orchestrator, *model.Task, *model.Run) {
	t.Helper()
	db, orch, task, run := stoppedResultFixture(t)
	if err := store.Migrate(db, &model.ActionAdmission{}, &model.WorkItemLease{}, &model.Writeback{}); err != nil {
		t.Fatal(err)
	}
	WithActionAdmissions(repository.NewActionAdmissionRepository(db))(orch)
	WithWorkItemLeases(repository.NewWorkItemLeaseRepository(db))(orch)
	WithWritebackRepository(repository.NewWritebackRepository(db))(orch)
	return db, orch, task, run
}

type stopBoundaryEffectExecutor struct {
	calls int
	after func() error
}

func (e *stopBoundaryEffectExecutor) ExecuteEffect(context.Context, *model.Effect) (string, error) {
	e.calls++
	if e.after != nil {
		if err := e.after(); err != nil {
			return "", err
		}
	}
	return `{"message_id":"sent"}`, nil
}

func admissionEffect(t *testing.T, db *gorm.DB, task *model.Task, run *model.Run, key string, status model.EffectStatus) *model.Effect {
	t.Helper()
	effect := &model.Effect{TaskID: task.TaskID, RunID: run.RunID, Kind: "send_im_message", DedupeKey: key, Target: "default", RequestJSON: `{"body":"done"}`, Status: status}
	if err := db.Create(effect).Error; err != nil {
		t.Fatal(err)
	}
	return effect
}

func TestStopBeforeIMAdmission(t *testing.T) {
	db, orch, task, run := externalAdmissionFixture(t)
	effect := admissionEffect(t, db, task, run, "message", model.EffectStatusPending)
	executor := &stopBoundaryEffectExecutor{}
	orch.effectExecutor = executor
	ctx := context.Background()
	if _, err := orch.StopTask(ctx, task.TaskID, StopActor{UserID: "admin"}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := orch.ExecutePendingEffects(ctx, 20); err != nil {
		t.Fatal(err)
	}
	stored, _ := orch.effects.GetByDedupeKey(ctx, effect.DedupeKey)
	if executor.calls != 0 || stored.Status != model.EffectStatusSkipped || stored.ErrorMessage != "task_interrupted" {
		t.Fatalf("stopped message was not skipped: calls=%d effect=%+v", executor.calls, stored)
	}
}

func TestStopBetweenBatchActions(t *testing.T) {
	db, orch, task, run := externalAdmissionFixture(t)
	first := admissionEffect(t, db, task, run, "first", model.EffectStatusPending)
	second := admissionEffect(t, db, task, run, "second", model.EffectStatusPending)
	ctx := context.Background()
	executor := &stopBoundaryEffectExecutor{after: func() error {
		_, err := orch.StopTask(ctx, task.TaskID, StopActor{UserID: "admin"}, "after first send")
		return err
	}}
	orch.effectExecutor = executor
	if _, err := orch.ExecutePendingEffects(ctx, 20); err != nil {
		t.Fatal(err)
	}
	storedFirst, _ := orch.effects.GetByDedupeKey(ctx, first.DedupeKey)
	storedSecond, _ := orch.effects.GetByDedupeKey(ctx, second.DedupeKey)
	if executor.calls != 1 || storedFirst.Status != model.EffectStatusSucceeded || storedSecond.Status != model.EffectStatusSkipped {
		t.Fatalf("batch reused send permission: calls=%d statuses=%s/%s", executor.calls, storedFirst.Status, storedSecond.Status)
	}
}

func TestCompletedTaskNotificationStillSends(t *testing.T) {
	for _, cleanup := range []bool{false, true} {
		name := "success"
		if cleanup {
			name = "cleanup"
		}
		t.Run(name, func(t *testing.T) {
			db, orch, task, run := externalAdmissionFixture(t)
			if err := db.Model(task).Update("status", model.TaskStatusSucceeded).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(run).Update("status", model.RunStatusSucceeded).Error; err != nil {
				t.Fatal(err)
			}
			if cleanup {
				if err := db.Create(&model.AgentExecution{RunID: run.RunID, ExecutionID: run.RunID, Provider: "codex", Status: model.AgentExecutionStatusRunning, RequestHash: "hash"}).Error; err != nil {
					t.Fatal(err)
				}
				if _, err := orch.StopExecution(context.Background(), run.RunID, StopActor{UserID: "admin"}, ""); err != nil {
					t.Fatal(err)
				}
			}
			admissionEffect(t, db, task, run, "success", model.EffectStatusPending)
			executor := &stopBoundaryEffectExecutor{}
			orch.effectExecutor = executor
			if _, err := orch.ExecutePendingEffects(context.Background(), 20); err != nil || executor.calls != 1 {
				t.Fatalf("success notice lost: calls=%d err=%v", executor.calls, err)
			}
			var admissions int64
			if err := db.Model(&model.ActionAdmission{}).Count(&admissions).Error; err != nil || admissions != 1 {
				t.Fatalf("notice did not obtain one permit: count=%d err=%v", admissions, err)
			}
		})
	}
}

func TestStoppedEffectRetryRejected(t *testing.T) {
	db, orch, task, run := externalAdmissionFixture(t)
	effect := admissionEffect(t, db, task, run, "failed", model.EffectStatusFailed)
	ctx := context.Background()
	if _, err := orch.StopTask(ctx, task.TaskID, StopActor{UserID: "admin"}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := orch.RetryEffect(ctx, effect.DedupeKey); !errors.Is(err, repository.ErrTaskInterrupted) {
		t.Fatalf("retry admitted stopped message: %v", err)
	}
}

func TestUnknownIMCannotBorrowReplayException(t *testing.T) {
	db, orch, task, run := externalAdmissionFixture(t)
	effect := admissionEffect(t, db, task, run, "unknown", model.EffectStatusFailed)
	if err := db.Create(&model.ActionAdmission{AdmissionID: "previous", TaskID: task.TaskID, RunID: run.RunID, Kind: model.ActionIMMessage, ActionKey: effect.EffectID, Attempt: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := orch.RetryEffect(context.Background(), effect.DedupeKey); !errors.Is(err, repository.ErrActionOutcomeUnknown) {
		t.Fatalf("unknown delivery was made retryable: %v", err)
	}
}

type disabledThenSendExecutor struct {
	disabled bool
	sent     int
}

func (e *disabledThenSendExecutor) ExecuteEffect(context.Context, *model.Effect) (string, error) {
	if e.disabled {
		return "", ErrEffectExecutorDisabled
	}
	e.sent++
	return `{"message_id":"sent"}`, nil
}

func TestAdmissionDisabledMessageCanResumeWithoutUnknownDelivery(t *testing.T) {
	db, orch, task, run := externalAdmissionFixture(t)
	effect := admissionEffect(t, db, task, run, "disabled", model.EffectStatusPending)
	executor := &disabledThenSendExecutor{disabled: true}
	orch.effectExecutor = executor
	if _, err := orch.ExecutePendingEffects(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	executor.disabled = false
	if _, err := orch.ExecutePendingEffects(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	stored, _ := orch.effects.GetByDedupeKey(context.Background(), effect.DedupeKey)
	if executor.sent != 1 || stored.Status != model.EffectStatusSucceeded {
		t.Fatalf("known unsent notification became unknown: sent=%d effect=%+v", executor.sent, stored)
	}
}

func TestStopProducesNoExternalNotification(t *testing.T) {
	db, orch, task, _ := externalAdmissionFixture(t)
	for range 2 {
		if _, err := orch.StopTask(context.Background(), task.TaskID, StopActor{UserID: "admin"}, ""); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []any{&model.Effect{}, &model.Writeback{}} {
		var count int64
		if err := db.Model(value).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("manual stop generated an external action: %T count=%d err=%v", value, count, err)
		}
	}
}

func TestUnknownIMFailureAndCrashRemainUnverified(t *testing.T) {
	for _, crash := range []bool{false, true} {
		name := "network_failure"
		if crash {
			name = "legacy_crash"
		}
		t.Run(name, func(t *testing.T) {
			db, orch, task, run := externalAdmissionFixture(t)
			effect := admissionEffect(t, db, task, run, "uncertain", model.EffectStatusPending)
			if crash {
				if err := db.Model(effect).Updates(map[string]any{"status": model.EffectStatusRunning, "attempt_count": 1, "updated_at": time.Now().Add(-time.Hour)}).Error; err != nil {
					t.Fatal(err)
				}
				if _, err := orch.RecoverStaleEffects(context.Background(), 20); err != nil {
					t.Fatal(err)
				}
			} else {
				orch.effectExecutor = &fakeEffectExecutor{errors: map[string]error{"uncertain": errors.New("connection reset after sending")}}
				if _, err := orch.ExecutePendingEffects(context.Background(), 20); err != nil {
					t.Fatal(err)
				}
			}
			stored, _ := orch.effects.GetByDedupeKey(context.Background(), effect.DedupeKey)
			if stored.Status != model.EffectStatusDeliveryUnverified {
				t.Fatalf("uncertain send became retryable: %+v", stored)
			}
		})
	}
}
