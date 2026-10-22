package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/storetest"
)

func TestMySQLStopAndDispatchIntentArbitration(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	observer, _ := mysqlLockSession(t, admin)
	for _, stopFirst := range []bool{true, false} {
		t.Run(fmt.Sprint(stopFirst), func(t *testing.T) {
			dsn := storetest.NewDatabase(t, admin, "stop_dispatch")
			first, firstID := mysqlLockSession(t, dsn)
			second, secondID := mysqlLockSession(t, dsn)
			if err := store.Migrate(first, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.StopRequest{}, &model.ResultSubmission{}); err != nil {
				t.Fatal(err)
			}
			runID := "run"
			for _, row := range []any{&model.Task{TaskID: "task", DedupeKey: "task", Type: "test", Role: "DeveloperWorker", Status: model.TaskStatusRunning, CurrentRunID: &runID}, &model.Run{RunID: runID, TaskID: "task", Status: model.RunStatusRunning, RunnerEndpoint: "runner-a"}} {
				if err := first.Create(row).Error; err != nil {
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
			if err := first.Callback().Query().After("gorm:query").Register("hold_dispatch_task", func(tx *gorm.DB) {
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
				_, err := NewStopRepository(db).Stop(ctx, StopTarget{TaskID: "task"}, "admin", "", 3)
				return err
			}
			create := func(db *gorm.DB) error {
				return NewAgentExecutionRepository(db).CreateDispatchIntent(ctx, &model.AgentExecution{RunID: runID, ExecutionID: runID, RunnerEndpoint: "runner-a", Status: model.AgentExecutionStatusDispatching, RequestHash: "hash", CreateSpecJSON: `{}`})
			}
			winner, loser := stop, create
			if !stopFirst {
				winner, loser = create, stop
			}
			won, lost := make(chan error, 1), make(chan error, 1)
			go func() { won <- winner(first) }()
			select {
			case <-arrived:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			go func() { lost <- loser(second) }()
			waitMySQLLockWait(t, ctx, observer, secondID, firstID)
			close(release)
			if err := <-won; err != nil {
				t.Fatal(err)
			}
			err := <-lost
			if stopFirst && !errors.Is(err, ErrTaskNotClaimable) {
				t.Fatalf("late intent accepted: %v", err)
			}
			if !stopFirst && err != nil {
				t.Fatal(err)
			}
			var stopRecord model.StopRequest
			if err := first.First(&stopRecord).Error; err != nil {
				t.Fatal(err)
			}
			if !stopFirst && (stopRecord.ExecutionID != runID || stopRecord.RunnerEndpoint != "runner-a" || stopRecord.DeliveryStatus != model.StopDeliveryPending) {
				t.Fatalf("stop lost pending create target: %+v", stopRecord)
			}
		})
	}
}
