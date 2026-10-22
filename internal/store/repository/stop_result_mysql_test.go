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

func TestMySQLStopAndResultFinalizationArbitration(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	observer, _ := mysqlLockSession(t, admin)
	for _, stopFirst := range []bool{true, false} {
		name := "completion_first"
		if stopFirst {
			name = "stop_first"
		}
		t.Run(name, func(t *testing.T) {
			dsn := storetest.NewDatabase(t, admin, "stop_result")
			first, firstID := mysqlLockSession(t, dsn)
			second, secondID := mysqlLockSession(t, dsn)
			if err := store.Migrate(first, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.StopRequest{}, &model.ResultSubmission{}, &model.Dependency{}); err != nil {
				t.Fatal(err)
			}
			runID := "current"
			task := &model.Task{TaskID: "task", DedupeKey: "original", Type: "test", Role: "TesterWorker", Status: model.TaskStatusFinalizing, CurrentRunID: &runID}
			run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusFinalizing}
			for _, value := range []any{task, run, &model.ResultSubmission{TaskID: task.TaskID, RunID: runID, Source: "mcp", Role: task.Role, PayloadHash: "original", RawPayloadJSON: `{"original":true}`, CanonicalJSON: `{}`, Status: model.ResultSubmissionStatusProcessing, AttemptCount: 1}} {
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
			if err := first.Callback().Query().After("gorm:query").Register("hold_result_task_lock", func(tx *gorm.DB) {
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
				_, err := NewStopRepository(db).Stop(ctx, StopTarget{TaskID: task.TaskID}, "admin", "closed", 3)
				return err
			}
			complete := func(db *gorm.DB) error {
				outTask, outRun := *task, *run
				outTask.Status, outRun.Status = model.TaskStatusSucceeded, model.RunStatusSucceeded
				return NewTaskRepository(db).FinalizeResultAtomic(ctx, &outTask, &outRun, []ResultTaskExpansion{{Task: &model.Task{TaskID: "child", DedupeKey: "child", Type: "test", Role: "DeveloperWorker", Status: model.TaskStatusPending}}}, true)
			}
			winner, loser := stop, complete
			if !stopFirst {
				winner, loser = complete, stop
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
			wantError := ErrStopAlreadyTerminal
			wantTask, wantRun, wantSubmission, wantCount := model.TaskStatusSucceeded, model.RunStatusSucceeded, model.ResultSubmissionStatusSucceeded, int64(2)
			if stopFirst {
				wantError = ErrTaskInterrupted
				wantTask, wantRun, wantSubmission, wantCount = model.TaskStatusInterrupted, model.RunStatusInterrupted, model.ResultSubmissionStatusInterrupted, 1
			}
			if err := <-loserResult; !errors.Is(err, wantError) {
				t.Fatalf("losing operation crossed decision: %v", err)
			}
			currentTask, _ := NewTaskRepository(second).GetByTaskID(ctx, task.TaskID)
			currentRun, _ := NewRunRepository(second).GetByRunID(ctx, runID)
			currentSubmission, _ := NewResultSubmissionRepository(second).GetByRunID(ctx, runID)
			var count int64
			if err := second.Model(&model.Task{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if currentTask.Status != wantTask || currentRun.Status != wantRun || currentSubmission.Status != wantSubmission || count != wantCount || currentSubmission.PayloadHash != "original" || currentSubmission.RawPayloadJSON != `{"original":true}` {
				t.Fatalf("inconsistent arbitration: %s/%s/%s tasks=%d", currentTask.Status, currentRun.Status, currentSubmission.Status, count)
			}
		})
	}
}
