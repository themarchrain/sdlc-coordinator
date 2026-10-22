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

func TestMySQLStopAndStartArbitration(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	observer, _ := mysqlLockSession(t, admin)
	for _, stopFirst := range []bool{true, false} {
		name := "start_first"
		if stopFirst {
			name = "stop_first"
		}
		t.Run(name, func(t *testing.T) {
			dsn := storetest.NewDatabase(t, admin, "stop_start")
			first, firstID := mysqlLockSession(t, dsn)
			second, secondID := mysqlLockSession(t, dsn)
			if err := store.Migrate(first, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.ResultSubmission{}, &model.StopRequest{}, &model.ActionAdmission{}); err != nil {
				t.Fatal(err)
			}
			task := &model.Task{TaskID: "arbitrate", DedupeKey: "arbitrate", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusPending}
			if err := first.Create(task).Error; err != nil {
				t.Fatal(err)
			}
			arrived, release := make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			if stopFirst {
				if err := first.Callback().Update().After("gorm:update").Register("hold_stop_commit", func(tx *gorm.DB) {
					if tx.Statement.Table == "tasks" {
						close(arrived)
						<-release
					}
				}); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := first.Callback().Create().Before("gorm:create").Register("hold_start_commit", func(tx *gorm.DB) {
					if tx.Statement.Table == "runs" {
						close(arrived)
						<-release
					}
				}); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			start := func(db *gorm.DB) error {
				now := time.Now().UTC()
				_, err := NewTaskRepository(db).StartRunAtomic(ctx, task.TaskID, &model.Run{RunID: "current", WorkerRole: task.Role}, now.Add(time.Hour), now)
				return err
			}
			stop := func(db *gorm.DB) error {
				_, err := NewStopRepository(db).Stop(ctx, StopTarget{TaskID: task.TaskID}, "admin", "", 3)
				return err
			}
			firstResult, secondResult := make(chan error, 1), make(chan error, 1)
			go func() {
				if stopFirst {
					firstResult <- stop(first)
				} else {
					firstResult <- start(first)
				}
			}()
			select {
			case <-arrived:
			case <-ctx.Done():
				t.Fatal("first transaction did not reach its write barrier")
			}
			go func() {
				if stopFirst {
					secondResult <- start(second)
				} else {
					secondResult <- stop(second)
				}
			}()
			waitMySQLLockWait(t, ctx, observer, secondID, firstID)
			close(release)
			if err := <-firstResult; err != nil {
				t.Fatal(err)
			}
			err := <-secondResult
			if stopFirst && !errors.Is(err, ErrTaskNotClaimable) {
				t.Fatalf("start crossed stop: %v", err)
			}
			if !stopFirst && err != nil {
				t.Fatalf("stop did not find committed Run: %v", err)
			}
			var runs []model.Run
			if err := second.Where("task_id = ?", task.TaskID).Find(&runs).Error; err != nil {
				t.Fatal(err)
			}
			if stopFirst && len(runs) != 0 {
				t.Fatal("stop-first created a Run")
			}
			if !stopFirst && (len(runs) != 1 || runs[0].Status != model.RunStatusInterrupted) {
				t.Fatalf("start-first escaped stop: %+v", runs)
			}
		})
	}
}

func TestMySQLStopAndRunnerRepinArbitration(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	observer, _ := mysqlLockSession(t, admin)
	for _, stopFirst := range []bool{true, false} {
		name := "repin_first"
		if stopFirst {
			name = "stop_first"
		}
		t.Run(name, func(t *testing.T) {
			dsn := storetest.NewDatabase(t, admin, "stop_repin")
			first, firstID := mysqlLockSession(t, dsn)
			second, secondID := mysqlLockSession(t, dsn)
			if err := store.Migrate(first, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.ResultSubmission{}, &model.StopRequest{}, &model.ActionAdmission{}); err != nil {
				t.Fatal(err)
			}
			task := &model.Task{TaskID: "repin", DedupeKey: "repin", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusRunning}
			if err := first.Create(task).Error; err != nil {
				t.Fatal(err)
			}
			run, execution := bindStopExecution(t, first, task, model.RunStatusRunning)
			arrived, release := make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			table := "agent_executions"
			if stopFirst {
				table = "tasks"
			}
			if err := first.Callback().Update().After("gorm:update").Register("hold_repin_arbitration", func(tx *gorm.DB) {
				if tx.Statement.Table == table {
					close(arrived)
					<-release
				}
			}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			repin := func(db *gorm.DB) error {
				return NewAgentExecutionRepository(db).UpdateRunnerEndpointIfUnchanged(ctx, run.RunID, execution.RunnerEndpoint, "runner-b")
			}
			stop := func(db *gorm.DB) error {
				_, err := NewStopRepository(db).Stop(ctx, StopTarget{TaskID: task.TaskID}, "admin", "", 3)
				return err
			}
			firstResult, secondResult := make(chan error, 1), make(chan error, 1)
			go func() {
				if stopFirst {
					firstResult <- stop(first)
				} else {
					firstResult <- repin(first)
				}
			}()
			select {
			case <-arrived:
			case <-ctx.Done():
				t.Fatal("first transaction did not reach endpoint barrier")
			}
			go func() {
				if stopFirst {
					secondResult <- repin(second)
				} else {
					secondResult <- stop(second)
				}
			}()
			waitMySQLLockWait(t, ctx, observer, secondID, firstID)
			close(release)
			if err := <-firstResult; err != nil {
				t.Fatal(err)
			}
			err := <-secondResult
			if stopFirst && !errors.Is(err, ErrTaskNotClaimable) {
				t.Fatalf("repin crossed stop: %v", err)
			}
			if !stopFirst && err != nil {
				t.Fatal(err)
			}
			var recorded model.StopRequest
			if err := second.Where("task_id = ?", task.TaskID).First(&recorded).Error; err != nil {
				t.Fatal(err)
			}
			want := "runner-b"
			if stopFirst {
				want = execution.RunnerEndpoint
			}
			var projection model.AgentExecution
			if err := second.Where("execution_id = ?", execution.ExecutionID).First(&projection).Error; err != nil {
				t.Fatal(err)
			}
			if recorded.RunnerEndpoint != want || projection.RunnerEndpoint != want {
				t.Fatalf("cancel target drifted: stop=%s execution=%s want=%s", recorded.RunnerEndpoint, projection.RunnerEndpoint, want)
			}
		})
	}
}
