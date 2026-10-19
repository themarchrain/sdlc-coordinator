package repository

import (
	"context"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/storetest"
)

// TestTaskAdvanceGuardedRejectsStaleSnapshot 两个连接持有同一任务的旧快照：
// 只有一个推进生效，败者的条件更新影响 0 行且不回滚胜者写入的列。
func TestTaskAdvanceGuardedRejectsStaleSnapshot(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, admin, "advance_guard")
	first, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open first: %v", err)
	}
	second, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open second: %v", err)
	}
	defer func() {
		if sqlDB, err := first.DB(); err == nil {
			_ = sqlDB.Close()
		}
		if sqlDB, err := second.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	if err := store.Migrate(first, &model.Task{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	repoFirst := NewTaskRepository(first)
	repoSecond := NewTaskRepository(second)

	task := &model.Task{TaskID: "task_guard", DedupeKey: "guard:1", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusPending, AttemptCount: 1}
	if err := repoFirst.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	// 两个连接各自读到同一 pending 快照。
	snapshotFirst, err := repoFirst.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("snapshot first: %v", err)
	}
	snapshotSecond, err := repoSecond.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("snapshot second: %v", err)
	}

	// 胜者：pending -> claimed（attempt_count 不变），推进生效。
	snapshotFirst.Status = model.TaskStatusClaimed
	applied, err := repoFirst.AdvanceGuarded(ctx, snapshotFirst, model.TaskStatusPending)
	if err != nil || !applied {
		t.Fatalf("winner advance: applied=%v err=%v", applied, err)
	}

	// 败者：用自己的旧快照（仍以为 pending）推进 -> 条件不满足，0 行，不覆盖。
	snapshotSecond.Status = model.TaskStatusRunning
	snapshotSecond.AttemptCount = 99
	applied, err = repoSecond.AdvanceGuarded(ctx, snapshotSecond, model.TaskStatusPending)
	if err != nil {
		t.Fatalf("loser advance: %v", err)
	}
	if applied {
		t.Fatal("stale advance applied against a moved task")
	}
	current, err := repoFirst.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if current.Status != model.TaskStatusClaimed {
		t.Fatalf("status = %s, want claimed (winner write clobbered)", current.Status)
	}
	if current.AttemptCount != 1 {
		t.Fatalf("attempt_count = %d, want 1 (loser clobbered the winner)", current.AttemptCount)
	}
}

// TestUpdateLastErrorIfStatusOnlyWhenStatusMatches 状态已并发前移时 last_error
// 回写被拒绝，不把错误挂到更新的状态上。
func TestUpdateLastErrorIfStatusOnlyWhenStatusMatches(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	db := storetest.OpenDatabase(t, admin, "last_error_guard")
	if err := store.Migrate(db, &model.Task{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	repo := NewTaskRepository(db)
	task := &model.Task{TaskID: "task_err", DedupeKey: "err:1", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusRetryPending}
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	applied, err := repo.UpdateLastErrorIfStatus(ctx, task.TaskID, model.TaskStatusRetryPending, "dispatch failed: workspace unavailable")
	if err != nil || !applied {
		t.Fatalf("guarded last error: applied=%v err=%v", applied, err)
	}
	// 状态推进后旧状态条件的回写被拒绝。
	applied, err = repo.UpdateLastErrorIfStatus(ctx, task.TaskID, model.TaskStatusRunning, "stale reason")
	if err != nil {
		t.Fatalf("stale last error: %v", err)
	}
	if applied {
		t.Fatal("last error applied against a non-matching status")
	}
	current, err := repo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if current.LastError != "dispatch failed: workspace unavailable" {
		t.Fatalf("last_error = %q, want the first write preserved", current.LastError)
	}
}

// TestUpdateDispatchProjectionDoesNotClobberStatus 派发期投影回写（外部系统/
// 外部 ID/input_json）是列级更新：run 状态在回写窗口内被并发推进时不回滚。
func TestUpdateInputJSONDoesNotClobberStatus(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	db := storetest.OpenDatabase(t, admin, "input_json_guard")
	if err := store.Migrate(db, &model.Run{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	repo := NewRunRepository(db)
	now := time.Now().UTC().Truncate(time.Microsecond)
	run := &model.Run{RunID: "run_guard", TaskID: "task_x", WorkerRole: "DeveloperWorker", Status: model.RunStatusPending, StartedAt: &now}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	// 并发方把 run 推进到 running。
	if err := db.Exec("UPDATE runs SET status = ? WHERE run_id = ?", model.RunStatusRunning, run.RunID).Error; err != nil {
		t.Fatalf("advance run: %v", err)
	}
	if err := repo.UpdateDispatchProjection(ctx, run.RunID, "agent-runner", run.RunID, `{"objective":"x"}`); err != nil {
		t.Fatalf("update dispatch projection: %v", err)
	}
	current, err := repo.GetByRunID(ctx, run.RunID)
	if err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if current.Status != model.RunStatusRunning {
		t.Fatalf("status = %s, want running preserved by column update", current.Status)
	}
	if current.InputJSON != `{"objective":"x"}` {
		t.Fatalf("input_json = %q", current.InputJSON)
	}
	if current.ExternalSystem != "agent-runner" || current.ExternalID != run.RunID {
		t.Fatalf("dispatch projection lost: system=%q id=%q", current.ExternalSystem, current.ExternalID)
	}
}
