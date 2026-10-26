package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/storetest"

	"gorm.io/gorm"
)

func TestMySQLStopAndActionAdmissionArbitration(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	observer, _ := mysqlLockSession(t, admin)
	for _, stopFirst := range []bool{true, false} {
		name := "admission_first"
		if stopFirst {
			name = "stop_first"
		}
		t.Run(name, func(t *testing.T) {
			dsn := storetest.NewDatabase(t, admin, "stop_admission")
			first, firstID := mysqlLockSession(t, dsn)
			second, secondID := mysqlLockSession(t, dsn)
			if err := store.Migrate(first, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.StopRequest{}, &model.ResultSubmission{}, &model.WorkItemLease{}, &model.ActionAdmission{}); err != nil {
				t.Fatal(err)
			}
			runID := "run"
			task := &model.Task{TaskID: "task", DedupeKey: "task", Type: "test", Role: "TesterWorker", Status: model.TaskStatusRunning, CurrentRunID: &runID}
			for _, value := range []any{task, &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}, &model.WorkItemLease{Scope: "codehost-writeback:action", Owner: "owner", Generation: 1, LeaseUntil: time.Now().Add(time.Hour), AcquiredAt: time.Now()}} {
				if err := first.Create(value).Error; err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			arrived, release := make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			if err := first.Callback().Query().After("gorm:query").Register("hold_admission_task_lock", func(tx *gorm.DB) {
				if _, locked := tx.Statement.Clauses["FOR"]; locked && tx.Statement.Table == "tasks" {
					close(arrived)
					select {
					case <-release:
					case <-ctx.Done():
						tx.AddError(ctx.Err())
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			stop := func(db *gorm.DB) error {
				_, err := NewStopRepository(db).Stop(ctx, StopTarget{TaskID: task.TaskID}, "admin", "", 3)
				return err
			}
			admit := func(db *gorm.DB) error {
				_, err := NewActionAdmissionRepository(db).AdmitAction(ctx, ActionAdmissionRequest{TaskID: task.TaskID, RunID: runID, Kind: model.ActionWriteback, ActionKey: "action", Attempt: 1, LeaseOwner: "owner", LeaseGeneration: 1})
				return err
			}
			winner, loser := stop, admit
			if !stopFirst {
				winner, loser = admit, stop
			}
			winnerResult, loserResult := make(chan error, 1), make(chan error, 1)
			go func() { winnerResult <- winner(first) }()
			select {
			case <-arrived:
			case <-ctx.Done():
				t.Fatal("first operation did not acquire Task lock")
			}
			go func() { loserResult <- loser(second) }()
			waitMySQLLockWait(t, ctx, observer, secondID, firstID)
			close(release)
			if err := <-winnerResult; err != nil {
				t.Fatal(err)
			}
			loserError := <-loserResult
			if stopFirst && !errors.Is(loserError, ErrTaskInterrupted) || !stopFirst && loserError != nil {
				t.Fatalf("unexpected losing operation: %v", loserError)
			}
			var count int64
			if err := second.Model(&model.ActionAdmission{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			wantCount := int64(1)
			if stopFirst {
				wantCount = 0
			}
			current, _ := NewTaskRepository(second).GetByTaskID(ctx, task.TaskID)
			if count != wantCount || current.Status != model.TaskStatusInterrupted {
				t.Fatalf("incorrect admission arbitration: count=%d task=%s", count, current.Status)
			}
		})
	}
}
