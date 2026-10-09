package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

// 编排器日志形态:五处既有日志点换 zap 后,用 observer 锁死每条事件的
// 级别与结构化字段——防止后续改动悄悄丢字段或改级别(台账 5.5 风险对策)。

func requireSingleEntry(t *testing.T, logs *observer.ObservedLogs, level zapcore.Level, message string) observer.LoggedEntry {
	t.Helper()
	entries := logs.FilterLevelExact(level).FilterMessage(message).All()
	if len(entries) != 1 {
		t.Fatalf("expect exactly one %s entry %q, got %d", level, message, len(entries))
	}
	return entries[0]
}

// 失败回写自身也失败:Error 级别 + run_id/task_id/update_error/dispatch_cause。
func TestLogRecordDispatchFailureUpdateError(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	core, logs := observer.New(zapcore.DebugLevel)
	now := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
	tasks := repository.NewTaskRepository(db)
	failing := &failingWritebackRunRepository{
		RunRepository: repository.NewRunRepository(db),
		failFor:       map[string]bool{"run-log-write-error": true},
	}
	orch := New(tasks, failing, WithClock(func() time.Time { return now }), WithLogger(zap.New(core)))

	runID := "run-log-write-error"
	task := &model.Task{
		TaskID: "task-log-write-error", DedupeKey: "log-write-error", Type: "review_mr", Role: "ReviewerWorker",
		Status: model.TaskStatusRunning, CurrentRunID: &runID, AttemptCount: 1,
	}
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := failing.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	orch.recordDispatchFailure(ctx, task, run, errors.New("workspace prepare failed"))

	entry := requireSingleEntry(t, logs, zapcore.ErrorLevel, "record dispatch failure: update run failed")
	fields := entry.ContextMap()
	if fields["run_id"] != "run-log-write-error" || fields["task_id"] != "task-log-write-error" {
		t.Fatalf("missing run/task identity fields: %#v", fields)
	}
	if fields["update_error"] != "transient write failure" {
		t.Fatalf("update_error field mismatch: %#v", fields)
	}
	if fields["dispatch_cause"] != "workspace prepare failed" {
		t.Fatalf("dispatch_cause field mismatch: %#v", fields)
	}
}

// 并发终态跳过写回:Info 级别 + run_id——设计内的并发仲裁,非故障。
func TestLogSkipDispatchFailureWriteback(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	core, logs := observer.New(zapcore.DebugLevel)
	now := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
	tasks := repository.NewTaskRepository(db)
	runs := repository.NewRunRepository(db)
	orch := New(tasks, runs, WithClock(func() time.Time { return now }), WithLogger(zap.New(core)))

	runID := "run-log-concurrent"
	task := &model.Task{
		TaskID: "task-log-concurrent", DedupeKey: "log-concurrent", Type: "review_mr", Role: "ReviewerWorker",
		Status: model.TaskStatusSucceeded, CurrentRunID: &runID,
	}
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	stale := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := runs.Create(ctx, stale); err != nil {
		t.Fatalf("create run: %v", err)
	}
	finalizedAt := now.Add(-time.Minute)
	if err := runs.Update(ctx, &model.Run{
		ID: stale.ID, RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role,
		Status: model.RunStatusSucceeded, CompletedAt: &finalizedAt,
	}); err != nil {
		t.Fatalf("finalize run concurrently: %v", err)
	}

	orch.recordDispatchFailure(ctx, &model.Task{TaskID: task.TaskID, Status: model.TaskStatusRunning, CurrentRunID: &runID}, stale, errors.New("late failure"))

	entry := requireSingleEntry(t, logs, zapcore.InfoLevel, "skip dispatch failure writeback: run already finalized concurrently")
	if entry.ContextMap()["run_id"] != "run-log-concurrent" {
		t.Fatalf("run_id field mismatch: %#v", entry.ContextMap())
	}
}

// 可选 followup 拼装失败:Warn + followup/task_id/run_id/reason 四字段齐全,
// 失败时回退调用前的 payload(同一实例),不阻断主链。
func TestLogSkipOptionalFollowup(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	orch := &Orchestrator{logger: zap.New(core)}
	task := &model.Task{TaskID: "task-log-followup"}
	payload := results.CanonicalResult{TaskID: task.TaskID, RunID: "run-log-followup", Status: results.StatusSucceeded}

	enriched := orch.applyOptionalFollowup(
		"developer rework", task, "run-log-followup",
		payload, results.CanonicalResult{TaskID: task.TaskID, RunID: "run-log-followup", Status: results.StatusFailed}, errors.New("im target missing"),
	)

	// 失败回退调用前的 payload(Status 保持 Succeeded),不阻断主链。
	if enriched.Status != results.StatusSucceeded {
		t.Fatalf("followup failure must fall back to the pre-call payload: %#v", enriched)
	}
	entry := requireSingleEntry(t, logs, zapcore.WarnLevel, "skip optional followup")
	fields := entry.ContextMap()
	if fields["followup"] != "developer rework" || fields["task_id"] != "task-log-followup" || fields["run_id"] != "run-log-followup" {
		t.Fatalf("missing followup identity fields: %#v", fields)
	}
	if fields["reason"] != "im target missing" {
		t.Fatalf("reason field mismatch: %#v", fields)
	}
}

// 门禁逐任务瞬态延后(merge_gate.go:84):Warn + task_id/transient。
// 走真实 RunAutoMergeGate:平台读失败归 Transient → skipped 分支。
func TestLogMergeGateTaskDeferredTransient(t *testing.T) {
	ctx := context.Background()
	client := &fakeGateClient{readErr: errors.New("network unreachable"), transientReads: 1}
	core, logs := observer.New(zapcore.DebugLevel)
	orch, tasks, artifacts, _ := newGateOrchWithLogger(t, client, zap.New(core))
	gateFixture(t, ctx, tasks, artifacts)

	decisions, err := orch.RunAutoMergeGate(ctx, 5)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if len(decisions) != 1 || decisions[0].Decision != MergeGateDecisionSkipped {
		t.Fatalf("transient read failure must skip, got %#v", decisions)
	}

	entry := requireSingleEntry(t, logs, zapcore.WarnLevel, "auto merge gate deferred task to next tick")
	fields := entry.ContextMap()
	if fields["task_id"] != "task-gate-verify" {
		t.Fatalf("task_id field mismatch: %#v", fields)
	}
	if fields["transient"] != "read live merge request head: network unreachable" {
		t.Fatalf("transient field mismatch: %#v", fields)
	}
}

// 门禁扫描失败(tick.go 经 logGateScanFailure):Warn + error 字段。
func TestLogMergeGateScanFailure(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	orch := &Orchestrator{logger: zap.New(core)}

	orch.logGateScanFailure(errors.New("list gate candidates: disk I/O error"))

	entry := requireSingleEntry(t, logs, zapcore.WarnLevel, "auto merge gate scan deferred to next tick")
	if entry.ContextMap()["error"] != "list gate candidates: disk I/O error" {
		t.Fatalf("error field mismatch: %#v", entry.ContextMap())
	}
}

// 未传 logger 的最小装配:回退 Nop,构造与行为不受影响(台账 5.3 定案:
// 回退仅在测试/最小装配出现,生产装配点显式传 logger)。
func TestNewWithoutLoggerFallsBackToNop(t *testing.T) {
	orch := New(nil, nil)
	if orch.logger == nil {
		t.Fatal("nil logger must fall back to a non-nil logger")
	}
	orch.logger.Info("dropped entry")
}
