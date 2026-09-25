package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

// 派发失败回写与并发终态的竞争：派发前存在分钟级的工作区准备窗口，MCP
// 回传通道可能已把该 run 终态化（succeeded）且任务已随结果推进。迟到的
// 失败回写持有派发前的旧快照，不得把库中的终态回滚成 failed，也不得把
// 已推进的任务回拉。
func TestRecordDispatchFailureKeepsConcurrentlyFinalizedRunIntact(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
	tasks := repository.NewTaskRepository(db)
	runs := repository.NewRunRepository(db)
	orch := New(tasks, runs, WithClock(func() time.Time { return now }))

	runID := "run-dispatch-late"
	task := &model.Task{
		TaskID: "task-dispatch-late", DedupeKey: "dispatch-late", Type: "review_mr", Role: "ReviewerWorker",
		Status: model.TaskStatusSucceeded, CurrentRunID: &runID,
	}
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	// 派发失败的现场：处理器手里的 run 对象在库中曾处于 running，主键
	// 已被 Create 回填（与真实派发路径一致，Save 才会走整行更新）。
	stale := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := runs.Create(ctx, stale); err != nil {
		t.Fatalf("create run: %v", err)
	}
	// 并发方先落终态：run 已 succeeded，任务已随结果推进到 succeeded 且
	// LastError 清空。
	finalizedAt := now.Add(-time.Minute)
	if err := runs.Update(ctx, &model.Run{
		ID: stale.ID, RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role,
		Status: model.RunStatusSucceeded, OutputJSON: `{"verdict":"approved"}`, CompletedAt: &finalizedAt,
	}); err != nil {
		t.Fatalf("finalize run concurrently: %v", err)
	}

	staleTask := &model.Task{TaskID: task.TaskID, Status: model.TaskStatusRunning, CurrentRunID: &runID, AttemptCount: 1}
	orch.recordDispatchFailure(ctx, staleTask, stale, errors.New("workspace prepare failed"))

	persisted, err := runs.GetByRunID(ctx, runID)
	if err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if persisted.Status != model.RunStatusSucceeded || persisted.ErrorMessage != "" || persisted.OutputJSON != `{"verdict":"approved"}` {
		t.Fatalf("late dispatch failure rolled back the terminal run: %#v", persisted)
	}
	if persisted.CompletedAt == nil || !persisted.CompletedAt.Equal(finalizedAt) {
		t.Fatalf("late dispatch failure overwrote CompletedAt: %#v", persisted.CompletedAt)
	}
	advanced, err := tasks.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if advanced.Status != model.TaskStatusSucceeded || advanced.LastError != "" {
		t.Fatalf("late dispatch failure pulled the advanced task back: %#v", advanced)
	}
}

// 护栏：run 仍在途（running）时，失败回写照常把 run 置为 failed 并推进
// 任务，生效路径的行为与既有语义保持一致。
func TestRecordDispatchFailureFailsInFlightRunAndAdvancesTask(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
	tasks := repository.NewTaskRepository(db)
	runs := repository.NewRunRepository(db)
	orch := New(tasks, runs, WithClock(func() time.Time { return now }))

	runID := "run-dispatch-inflight"
	task := &model.Task{
		TaskID: "task-dispatch-inflight", DedupeKey: "dispatch-inflight", Type: "review_mr", Role: "ReviewerWorker",
		Status: model.TaskStatusRunning, CurrentRunID: &runID, AttemptCount: 1,
	}
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := runs.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	orch.recordDispatchFailure(ctx, task, run, errors.New("provider rejected execution"))

	persisted, err := runs.GetByRunID(ctx, runID)
	if err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if persisted.Status != model.RunStatusFailed || persisted.ErrorMessage != "provider rejected execution" || persisted.CompletedAt == nil {
		t.Fatalf("in-flight run was not marked failed: %#v", persisted)
	}
	advanced, err := tasks.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if advanced.Status != model.TaskStatusRetryPending || advanced.LastError != "provider rejected execution" {
		t.Fatalf("task was not advanced by the dispatch failure: %#v", advanced)
	}
}

type failingWritebackRunRepository struct {
	RunRepository
	failFor map[string]bool
}

func (r *failingWritebackRunRepository) UpdateFailedIfInFlight(ctx context.Context, runID, message string, completedAt time.Time) (bool, error) {
	if r.failFor[runID] {
		return false, errors.New("transient write failure")
	}
	return r.RunRepository.UpdateFailedIfInFlight(ctx, runID, message, completedAt)
}

// 存储抖动分支：条件更新报错时失败回写整体放弃，只记日志——任务不推进、
// LastError 不写，在途 run 交给后续 tick 的对账与租约恢复收敛。
func TestRecordDispatchFailureGivesUpOnWriteError(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
	tasks := repository.NewTaskRepository(db)
	runs := repository.NewRunRepository(db)
	failing := &failingWritebackRunRepository{RunRepository: runs, failFor: map[string]bool{"run-dispatch-write-error": true}}
	orch := New(tasks, failing, WithClock(func() time.Time { return now }))

	runID := "run-dispatch-write-error"
	task := &model.Task{
		TaskID: "task-dispatch-write-error", DedupeKey: "dispatch-write-error", Type: "review_mr", Role: "ReviewerWorker",
		Status: model.TaskStatusRunning, CurrentRunID: &runID, AttemptCount: 1,
	}
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := runs.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	orch.recordDispatchFailure(ctx, task, run, errors.New("workspace prepare failed"))

	persisted, err := runs.GetByRunID(ctx, runID)
	if err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if persisted.Status != model.RunStatusRunning || persisted.ErrorMessage != "" {
		t.Fatalf("write failure must leave the run untouched: %#v", persisted)
	}
	reloaded, err := tasks.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if reloaded.Status != model.TaskStatusRunning || reloaded.LastError != "" {
		t.Fatalf("write failure must not advance the task: %#v", reloaded)
	}
}
