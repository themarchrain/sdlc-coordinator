package orchestrator

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/store/storetest"

	"gorm.io/gorm"
)

type a18LockRow struct {
	ID    uint `gorm:"primaryKey"`
	Value int
}

func (a18LockRow) TableName() string { return "a18_lock_rows" }

type a18Runner struct{ starts int }

func (r *a18Runner) Start(_ context.Context, request DeveloperRunnerStartRequest) (*model.AgentExecution, error) {
	r.starts++
	return &model.AgentExecution{
		RunID: request.Run.RunID, ExecutionID: request.Run.RunID,
		Status: model.AgentExecutionStatusAccepted,
	}, nil
}

func (*a18Runner) Reconcile(context.Context, string) (*DeveloperRunnerReconcileResult, error) {
	return nil, nil
}

func openA18MySQL(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestA18MySQLDeadlockErrorIsRetryable(t *testing.T) {
	adminDSN := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, adminDSN, "a18_deadlock")
	first := openA18MySQL(t, dsn)
	second := openA18MySQL(t, dsn)
	admin := openA18MySQL(t, adminDSN)
	if err := first.AutoMigrate(&a18LockRow{}); err != nil {
		t.Fatalf("migrate lock rows: %v", err)
	}
	if err := first.Create(&[]a18LockRow{{ID: 1}, {ID: 2}}).Error; err != nil {
		t.Fatalf("seed lock rows: %v", err)
	}

	txFirst := first.Begin()
	txSecond := second.Begin()
	if txFirst.Error != nil || txSecond.Error != nil {
		t.Fatalf("begin transactions: first=%v second=%v", txFirst.Error, txSecond.Error)
	}
	t.Cleanup(func() {
		_ = txFirst.Rollback().Error
		_ = txSecond.Rollback().Error
	})
	if err := txFirst.Exec("UPDATE a18_lock_rows SET value = value + 1 WHERE id = 1").Error; err != nil {
		t.Fatalf("first transaction lock row 1: %v", err)
	}
	if err := txSecond.Exec("UPDATE a18_lock_rows SET value = value + 1 WHERE id = 2").Error; err != nil {
		t.Fatalf("second transaction lock row 2: %v", err)
	}
	var connectionID int64
	if err := txFirst.Raw("SELECT CONNECTION_ID()").Scan(&connectionID).Error; err != nil {
		t.Fatalf("read first transaction connection id: %v", err)
	}
	waiting := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		waiting <- txFirst.Exec("UPDATE a18_lock_rows SET value = value + 1 WHERE id = 2").Error
	}()
	<-started
	deadline := time.Now().Add(5 * time.Second)
	for {
		var count int64
		err := admin.Raw("SELECT COUNT(*) FROM information_schema.innodb_trx WHERE trx_mysql_thread_id = ? AND trx_state = 'LOCK WAIT'", connectionID).Scan(&count).Error
		if err != nil {
			t.Fatalf("observe lock wait: %v", err)
		}
		if count > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first transaction never entered a MySQL lock wait")
		}
		time.Sleep(20 * time.Millisecond)
	}
	secondErr := txSecond.Exec("UPDATE a18_lock_rows SET value = value + 1 WHERE id = 1").Error
	firstErr := <-waiting
	deadlockErr := secondErr
	if deadlockErr == nil {
		deadlockErr = firstErr
	}
	if !store.IsDeadlock(deadlockErr) || !store.IsRetryable(deadlockErr) {
		t.Fatalf("injected MySQL deadlock was not classified retryable: first=%v second=%v", firstErr, secondErr)
	}
}

func TestA18MySQLDispatchLockTimeoutDoesNotConsumeAttemptAndRetriesNextDispatch(t *testing.T) {
	adminDSN := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, adminDSN, "a18_lock_timeout")
	first := openA18MySQL(t, dsn)
	second := openA18MySQL(t, dsn)
	if err := store.Migrate(first, &model.Task{}, &model.Run{}); err != nil {
		t.Fatalf("migrate coordinator tables: %v", err)
	}
	repositoryID := "repo-a18"
	const taskID = "task-a18-lock-timeout"
	if err := repository.NewTaskRepository(first).Create(context.Background(), &model.Task{
		TaskID: taskID, DedupeKey: "dedupe-" + taskID, RepositoryID: &repositoryID,
		Type: "bugfix", Role: "DeveloperWorker", TargetRepo: "group/project", Status: model.TaskStatusPending,
		ContextJSON: `{"repos":[{"name":"project","access":"writable","branch":"main"}]}`,
	}); err != nil {
		t.Fatalf("seed dispatchable task: %v", err)
	}

	blocker := second.Begin()
	if blocker.Error != nil {
		t.Fatalf("begin blocker transaction: %v", blocker.Error)
	}
	if err := blocker.Raw("SELECT task_id FROM tasks WHERE task_id = ? FOR UPDATE", taskID).Scan(new(string)).Error; err != nil {
		t.Fatalf("lock dispatchable task from second instance: %v", err)
	}

	runner := &a18Runner{}
	settings := ReviewerRunnerSettings{
		ResolveRepository: func(context.Context, *model.Task) (*registeredrepo.RegisteredRepository, error) {
			return &registeredrepo.RegisteredRepository{
				RepositoryID: repositoryID, Provider: "gitlab", BaseURL: "https://gitlab.example",
				PathWithNamespace: "group/project", Token: "test-token",
			}, nil
		},
	}
	var timedOutOrchestrator *Orchestrator
	var dispatchErr error
	if err := first.Connection(func(connection *gorm.DB) error {
		if err := connection.Exec("SET SESSION innodb_lock_wait_timeout = 1").Error; err != nil {
			return err
		}
		timedOutOrchestrator = New(repository.NewTaskRepository(connection), repository.NewRunRepository(connection), WithDeveloperRunner(runner, settings))
		_, dispatchErr = timedOutOrchestrator.DispatchDeveloperTask(context.Background(), taskID)
		return nil
	}); err != nil {
		t.Fatalf("run dispatch on pinned MySQL connection: %v", err)
	}
	if !store.IsLockWaitTimeout(dispatchErr) || !store.IsRetryable(dispatchErr) {
		t.Fatalf("injected MySQL lock timeout was not surfaced as retryable: %v", dispatchErr)
	}
	if runner.starts != 0 {
		t.Fatalf("runner started after database claim failed: starts=%d", runner.starts)
	}
	if err := blocker.Rollback().Error; err != nil {
		t.Fatalf("release blocker transaction: %v", err)
	}

	stored, err := repository.NewTaskRepository(first).GetByTaskID(context.Background(), taskID)
	if err != nil {
		t.Fatalf("reload task after timeout: %v", err)
	}
	if stored.Status != model.TaskStatusPending || stored.AttemptCount != 0 || stored.CurrentRunID != nil {
		t.Fatalf("database contention changed task execution state: status=%s attempts=%d current_run=%v", stored.Status, stored.AttemptCount, stored.CurrentRunID)
	}

	retry := New(repository.NewTaskRepository(first), repository.NewRunRepository(first), WithDeveloperRunner(runner, settings))
	if _, err := retry.DispatchDeveloperTask(context.Background(), taskID); err != nil {
		t.Fatalf("dispatch after releasing lock: %v", err)
	}
	stored, err = repository.NewTaskRepository(first).GetByTaskID(context.Background(), taskID)
	if err != nil {
		t.Fatalf("reload task after retry: %v", err)
	}
	if runner.starts != 1 || stored.Status != model.TaskStatusRunning || stored.AttemptCount != 1 {
		t.Fatalf("next dispatch did not recover cleanly: starts=%d status=%s attempts=%d", runner.starts, stored.Status, stored.AttemptCount)
	}
}

func TestA18MySQLDisconnectDoesNotDispatch(t *testing.T) {
	adminDSN := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, adminDSN, "a18_disconnect")
	db := openA18MySQL(t, dsn)
	admin := openA18MySQL(t, adminDSN)
	if err := store.Migrate(db, &model.Task{}, &model.Run{}); err != nil {
		t.Fatalf("migrate coordinator tables: %v", err)
	}
	repositoryID := "repo-a18-disconnect"
	const taskID = "task-a18-disconnect"
	if err := repository.NewTaskRepository(db).Create(context.Background(), &model.Task{
		TaskID: taskID, DedupeKey: "dedupe-" + taskID, RepositoryID: &repositoryID,
		Type: "bugfix", Role: "DeveloperWorker", TargetRepo: "group/project", Status: model.TaskStatusPending,
		ContextJSON: `{"repos":[{"name":"project","access":"writable","branch":"main"}]}`,
	}); err != nil {
		t.Fatalf("seed dispatchable task: %v", err)
	}
	runner := &a18Runner{}
	var dispatchErr error
	if err := db.Connection(func(connection *gorm.DB) error {
		var connectionID int64
		if err := connection.Raw("SELECT CONNECTION_ID()").Scan(&connectionID).Error; err != nil {
			return err
		}
		if err := admin.Exec("KILL CONNECTION " + strconv.FormatInt(connectionID, 10)).Error; err != nil {
			return fmt.Errorf("kill dispatch connection: %w", err)
		}
		orch := New(repository.NewTaskRepository(connection), repository.NewRunRepository(connection), WithDeveloperRunner(runner, ReviewerRunnerSettings{}))
		_, dispatchErr = orch.DispatchDeveloperTask(context.Background(), taskID)
		return nil
	}); err != nil {
		t.Fatalf("run dispatch on disconnected MySQL connection: %v", err)
	}
	if !store.IsConnectionError(dispatchErr) || !store.IsRetryable(dispatchErr) {
		t.Fatalf("injected disconnect was not surfaced as retryable: %v", dispatchErr)
	}
	if runner.starts != 0 {
		t.Fatalf("runner started after database disconnect: starts=%d", runner.starts)
	}
	stored, err := repository.NewTaskRepository(db).GetByTaskID(context.Background(), taskID)
	if err != nil {
		t.Fatalf("reload task after disconnect: %v", err)
	}
	if stored.Status != model.TaskStatusPending || stored.AttemptCount != 0 || stored.CurrentRunID != nil {
		t.Fatalf("disconnect changed task execution state: status=%s attempts=%d current_run=%v", stored.Status, stored.AttemptCount, stored.CurrentRunID)
	}
}
