package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"

	"gorm.io/gorm"
)

func actionAdmissionFixture(t *testing.T, kind model.ActionKind) (*gorm.DB, *ActionAdmissionRepository, ActionAdmissionRequest) {
	t.Helper()
	db, _, task := stopFixture(t, model.TaskStatusRunning)
	run, _ := bindStopExecution(t, db, task, model.RunStatusRunning)
	if err := store.Migrate(db, &model.WorkItemLease{}); err != nil {
		t.Fatal(err)
	}
	request := ActionAdmissionRequest{TaskID: task.TaskID, RunID: run.RunID, Kind: kind, ActionKey: "action", Attempt: 1, LeaseOwner: "owner", LeaseGeneration: 1}
	scope := ""
	switch kind {
	case model.ActionExecutionCreate:
		scope = "execution-create:action"
	case model.ActionGitCommit, model.ActionGitPush:
		scope = "developer-delivery:" + run.RunID
	case model.ActionWriteback:
		scope = "codehost-writeback:action"
	case model.ActionIMMessage:
		scope = "effect:action"
	case model.ActionAutoMerge:
		request.ActionKey = task.TaskID
		scope = "merge-gate:" + task.TaskID
	}
	if err := db.Create(&model.WorkItemLease{Scope: scope, Owner: request.LeaseOwner, Generation: 1, OperationID: "operation", LeaseUntil: time.Now().Add(time.Hour), AcquiredAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	return db, NewActionAdmissionRepository(db), request
}

func TestStopWinsActionAdmission(t *testing.T) {
	for _, kind := range []model.ActionKind{model.ActionExecutionCreate, model.ActionGitCommit, model.ActionGitPush, model.ActionWriteback, model.ActionIMMessage, model.ActionAutoMerge} {
		t.Run(string(kind), func(t *testing.T) {
			db, admissions, request := actionAdmissionFixture(t, kind)
			ctx := context.Background()
			if _, err := NewStopRepository(db).Stop(ctx, StopTarget{TaskID: request.TaskID}, "admin", "closed", 3); err != nil {
				t.Fatal(err)
			}
			if _, err := admissions.AdmitAction(ctx, request); !errors.Is(err, ErrTaskInterrupted) {
				t.Fatalf("stopped %s gained admission: %v", kind, err)
			}
			var count int64
			if err := db.Model(&model.ActionAdmission{}).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("stop created admission: count=%d err=%v", count, err)
			}
		})
	}
}

func TestAdmissionWinsStop(t *testing.T) {
	db, admissions, request := actionAdmissionFixture(t, model.ActionWriteback)
	ctx := context.Background()
	admitted, err := admissions.AdmitAction(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if admitted.AdmissionID == "" || admitted.AdmittedAt.IsZero() || admitted.LeaseGeneration != 1 || admitted.TaskID != request.TaskID || admitted.ActionKey != request.ActionKey {
		t.Fatalf("missing admission identity: %+v", admitted)
	}
	if _, err := NewStopRepository(db).Stop(ctx, StopTarget{TaskID: request.TaskID}, "admin", "", 3); err != nil {
		t.Fatal(err)
	}
	var persisted model.ActionAdmission
	if err := db.Where("admission_id = ?", admitted.AdmissionID).First(&persisted).Error; err != nil || persisted.AdmissionID != admitted.AdmissionID {
		t.Fatalf("stop erased in-flight fact: %+v %v", persisted, err)
	}
}

func TestAdmissionRejectsStaleGeneration(t *testing.T) {
	for _, mutation := range []string{"owner", "generation", "expiry", "run"} {
		t.Run(mutation, func(t *testing.T) {
			db, admissions, request := actionAdmissionFixture(t, model.ActionGitPush)
			want := ErrActionLeaseLost
			switch mutation {
			case "owner":
				request.LeaseOwner = "old-owner"
			case "generation":
				request.LeaseGeneration++
			case "expiry":
				if err := db.Model(&model.WorkItemLease{}).Where("owner = ?", request.LeaseOwner).Update("lease_until", time.Now().Add(-time.Minute)).Error; err != nil {
					t.Fatal(err)
				}
			case "run":
				request.RunID = "previous-run"
				want = ErrStopNotCurrentRun
			}
			if _, err := admissions.AdmitAction(context.Background(), request); !errors.Is(err, want) {
				t.Fatalf("invalid execution right gained admission: %v", err)
			}
		})
	}
}

func TestAdmissionFailsClosed(t *testing.T) {
	db, admissions, request := actionAdmissionFixture(t, model.ActionIMMessage)
	sqlDB, _ := db.DB()
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := admissions.AdmitAction(context.Background(), request); err == nil || errors.Is(err, ErrTaskInterrupted) || errors.Is(err, ErrActionLeaseLost) {
		t.Fatalf("database failure must remain a system error, got %v", err)
	}
}

func TestDuplicateAdmissionDoesNotAuthorizeResend(t *testing.T) {
	db, admissions, request := actionAdmissionFixture(t, model.ActionIMMessage)
	ctx := context.Background()
	if _, err := admissions.AdmitAction(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err := admissions.AdmitAction(ctx, request); !errors.Is(err, ErrActionAlreadyAdmitted) {
		t.Fatalf("duplicate admission became a send permit: %v", err)
	}
	request.Attempt++
	if _, err := admissions.AdmitAction(ctx, request); !errors.Is(err, ErrActionOutcomeUnknown) {
		t.Fatalf("new attempt bypassed unknown outcome: %v", err)
	}
	var count int64
	if err := db.Model(&model.ActionAdmission{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("unexpected admission count %d err=%v", count, err)
	}
}

func TestAdmissionAllowsSuccessNotificationsAndCleanup(t *testing.T) {
	for _, cleanup := range []bool{false, true} {
		name := "normal_success"
		if cleanup {
			name = "execution_cleanup"
		}
		t.Run(name, func(t *testing.T) {
			db, admissions, request := actionAdmissionFixture(t, model.ActionIMMessage)
			if err := db.Model(&model.Task{}).Where("task_id = ?", request.TaskID).Update("status", model.TaskStatusSucceeded).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&model.Run{}).Where("run_id = ?", request.RunID).Update("status", model.RunStatusSucceeded).Error; err != nil {
				t.Fatal(err)
			}
			if cleanup {
				if _, err := NewStopRepository(db).Stop(context.Background(), StopTarget{ExecutionID: "execution-stop"}, "admin", "", 3); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := admissions.AdmitAction(context.Background(), request); err != nil {
				t.Fatalf("legitimate completed-task notice blocked: %v", err)
			}
		})
	}
}

func TestAdmissionAcceptsEachKind(t *testing.T) {
	for _, kind := range []model.ActionKind{model.ActionExecutionCreate, model.ActionGitCommit, model.ActionGitPush, model.ActionWriteback, model.ActionIMMessage, model.ActionAutoMerge} {
		t.Run(string(kind), func(t *testing.T) {
			_, admissions, request := actionAdmissionFixture(t, kind)
			if _, err := admissions.AdmitAction(context.Background(), request); err != nil {
				t.Fatalf("valid %s action rejected: %v", kind, err)
			}
		})
	}
}

func TestAdmissionInsertRollback(t *testing.T) {
	db, admissions, request := actionAdmissionFixture(t, model.ActionWriteback)
	injected := errors.New("admission storage failure")
	if err := db.Callback().Create().Before("gorm:create").Register("fail_admission", func(tx *gorm.DB) {
		if tx.Statement.Table == "action_admissions" {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := admissions.AdmitAction(context.Background(), request); !errors.Is(err, injected) {
		t.Fatalf("lost storage error: %v", err)
	}
	var count int64
	if err := db.Model(&model.ActionAdmission{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("failed insert left a permit: %d err=%v", count, err)
	}
}

func TestAdmissionNonDeliveryProofIsIdempotentAndFenced(t *testing.T) {
	_, admissions, request := actionAdmissionFixture(t, model.ActionWriteback)
	ctx := context.Background()
	admission, err := admissions.AdmitAction(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := admissions.ConfirmNotSent(ctx, admission); err != nil {
			t.Fatalf("non-delivery proof is not idempotent: %v", err)
		}
	}
	stale := *admission
	stale.LeaseOwner = "other-owner"
	if err := admissions.ConfirmNotSent(ctx, &stale); !errors.Is(err, ErrActionLeaseLost) {
		t.Fatalf("foreign owner changed admission outcome: %v", err)
	}
	request.Attempt++
	if _, err := admissions.AdmitAction(ctx, request); err != nil {
		t.Fatalf("confirmed non-delivery did not allow a new permit: %v", err)
	}
}
