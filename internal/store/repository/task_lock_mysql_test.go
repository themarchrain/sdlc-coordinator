package repository

import (
	"context"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/storetest"

	"gorm.io/gorm"
)

func mysqlLockSession(t *testing.T, dsn string) (*gorm.DB, int64) {
	t.Helper()
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	var id int64
	if err := db.Raw("SELECT CONNECTION_ID()").Scan(&id).Error; err != nil {
		t.Fatal(err)
	}
	return db, id
}

func waitMySQLLockWait(t *testing.T, ctx context.Context, observer *gorm.DB, waiting, blocking int64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		var count int64
		err := observer.WithContext(ctx).Raw(`SELECT COUNT(*)
FROM performance_schema.data_lock_waits w
JOIN performance_schema.threads requesting ON requesting.THREAD_ID = w.REQUESTING_THREAD_ID
JOIN performance_schema.threads blocking ON blocking.THREAD_ID = w.BLOCKING_THREAD_ID
WHERE requesting.PROCESSLIST_ID = ? AND blocking.PROCESSLIST_ID = ?`, waiting, blocking).Scan(&count).Error
		if err != nil {
			t.Fatalf("observe lock wait %d -> %d: %v", waiting, blocking, err)
		}
		if count > 0 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("lock wait %d -> %d was not observed: %v", waiting, blocking, ctx.Err())
		case <-ticker.C:
		}
	}
}

func TestMySQLEndpointPinSerializesTaskFinalizers(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	observer, _ := mysqlLockSession(t, admin)
	for _, test := range []struct {
		name string
		want model.TaskStatus
	}{
		{"merged", model.TaskStatusSucceeded},
		{"closed", model.TaskStatusBlocked},
		{"require_human", model.TaskStatusHumanRequired},
	} {
		t.Run(test.name, func(t *testing.T) {
			dsn := storetest.NewDatabase(t, admin, "pin_finalize")
			first, firstID := mysqlLockSession(t, dsn)
			second, secondID := mysqlLockSession(t, dsn)
			if err := store.Migrate(first, &model.Task{}, &model.Run{}, &model.AgentExecution{}); err != nil {
				t.Fatal(err)
			}
			task := &model.Task{TaskID: "finalize", DedupeKey: "rework", Type: model.TaskTypeReworkMergeRequest, Role: "DeveloperWorker", Status: model.TaskStatusRunning}
			if err := first.Create(task).Error; err != nil {
				t.Fatal(err)
			}
			run, _ := bindStopExecution(t, first, task, model.RunStatusRunning)
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
			if err := first.Callback().Query().Before("gorm:query").Register("hold_pin_before_run", func(tx *gorm.DB) {
				if _, locking := tx.Statement.Clauses["FOR"]; locking && tx.Statement.Table == "runs" {
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
			pinResult, finalizeResult := make(chan error, 1), make(chan error, 1)
			go func() { pinResult <- NewRunRepository(first).PinRunnerEndpointIfEmpty(ctx, run.RunID, "runner-a") }()
			select {
			case <-arrived:
			case <-ctx.Done():
				t.Fatal("pin did not reach the locked Task boundary")
			}
			go func() {
				repo := NewTaskRepository(second)
				if test.name == "require_human" {
					finalizeResult <- repo.BlockRunAndRequireHuman(ctx, run.RunID, "invalid result", time.Now().UTC())
					return
				}
				_, err := repo.FinalizeRework(ctx, task.DedupeKey, test.want, "", time.Now().UTC())
				finalizeResult <- err
			}()
			waitMySQLLockWait(t, ctx, observer, secondID, firstID)
			close(release)
			if err := <-pinResult; err != nil {
				t.Errorf("endpoint pin failed during task finalization: %v", err)
			}
			if err := <-finalizeResult; err != nil {
				t.Errorf("task finalization failed during endpoint pin: %v", err)
			}
			current, err := NewTaskRepository(second).GetByTaskID(ctx, task.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			currentRun, err := NewRunRepository(second).GetByRunID(ctx, run.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if current.Status != test.want || currentRun.Status != model.RunStatusBlocked || currentRun.RunnerEndpoint != "runner-a" {
				t.Fatalf("finalization or endpoint lost: task=%+v run=%+v", current, currentRun)
			}
		})
	}
}
