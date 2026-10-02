package orchestrator

import (
	"context"
	"errors"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
)

// Tester 派发失败与 runner 执行失败两条路径不经过结果应用,重试预算耗尽
// 落 blocked 后若无人工触点即无声消失。锁定:无结果停靠必须物化去重 IM
// 通知;重试未耗尽不打扰;非 tester 角色维持现状(统一通知收口挂账);
// 迟到失败撞上并发终态时不推进也不通知。

func testerFailureStopTask(taskID, runID, role string, attempts int) *model.Task {
	return &model.Task{
		TaskID: taskID, DedupeKey: "verify:" + taskID, Type: "verify_merge_request", Role: role,
		Status: model.TaskStatusRunning, CurrentRunID: &runID, AttemptCount: attempts,
	}
}

func TestRecordDispatchFailureNotifiesWhenTesterBudgetExhausted(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, effectRepo := newRecoveryRepositoriesWithEffects(t)
	orch := New(taskRepo, runRepo, WithEffectRepository(effectRepo))
	runID := "run-tester-dispatch-exhausted"
	task := testerFailureStopTask("task-tester-dispatch-exhausted", runID, "TesterWorker", 3)
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	orch.recordDispatchFailure(ctx, task, run, errors.New("workspace prepare failed"))

	advanced, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if advanced.Status != model.TaskStatusBlocked {
		t.Fatalf("exhausted tester must stop at blocked, got %s", advanced.Status)
	}
	effect, err := effectRepo.GetByDedupeKey(ctx, "human-required-im:"+task.TaskID+":"+runID)
	if err != nil {
		t.Fatalf("blocked tester must materialize a human notification effect: %v", err)
	}
	if effect.Kind != "send_im_message" || effect.Target != "default" || effect.RunID != runID {
		t.Fatalf("unexpected notification effect: %#v", effect)
	}
}

func TestRecordDispatchFailureStaysSilentWhileTesterRetriesRemain(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, effectRepo := newRecoveryRepositoriesWithEffects(t)
	orch := New(taskRepo, runRepo, WithEffectRepository(effectRepo))
	runID := "run-tester-dispatch-retry"
	task := testerFailureStopTask("task-tester-dispatch-retry", runID, "TesterWorker", 1)
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	orch.recordDispatchFailure(ctx, task, run, errors.New("workspace prepare failed"))

	advanced, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if advanced.Status != model.TaskStatusRetryPending {
		t.Fatalf("tester with remaining retries must go back to retry_pending, got %s", advanced.Status)
	}
	if _, err := effectRepo.GetByDedupeKey(ctx, "human-required-im:"+task.TaskID+":"+runID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("retry_pending stop must not notify: err=%v", err)
	}
}

func TestRecordDispatchFailureDoesNotNotifyNonTesterRoles(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, effectRepo := newRecoveryRepositoriesWithEffects(t)
	orch := New(taskRepo, runRepo, WithEffectRepository(effectRepo))
	runID := "run-reviewer-dispatch-exhausted"
	task := testerFailureStopTask("task-reviewer-dispatch-exhausted", runID, "ReviewerWorker", 3)
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	orch.recordDispatchFailure(ctx, task, run, errors.New("workspace prepare failed"))

	if _, err := effectRepo.GetByDedupeKey(ctx, "human-required-im:"+task.TaskID+":"+runID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("non-tester roles keep their current behavior, no notification expected: err=%v", err)
	}
}

func TestRecordReviewerRunnerFailureNotifiesWhenTesterBudgetExhausted(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, effectRepo := newRecoveryRepositoriesWithEffects(t)
	orch := New(taskRepo, runRepo, WithEffectRepository(effectRepo))
	runID := "run-tester-execution-exhausted"
	task := testerFailureStopTask("task-tester-execution-exhausted", runID, "TesterWorker", 3)
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	if err := orch.recordReviewerRunnerFailure(ctx, runID, "agent runner execution failed"); err != nil {
		t.Fatalf("record reviewer runner failure: %v", err)
	}

	advanced, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if advanced.Status != model.TaskStatusBlocked {
		t.Fatalf("exhausted tester must stop at blocked, got %s", advanced.Status)
	}
	if _, err := effectRepo.GetByDedupeKey(ctx, "human-required-im:"+task.TaskID+":"+runID); err != nil {
		t.Fatalf("blocked tester must materialize a human notification effect: %v", err)
	}
}

func TestRecordReviewerRunnerFailureStaysSilentForTerminalRuns(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, effectRepo := newRecoveryRepositoriesWithEffects(t)
	orch := New(taskRepo, runRepo, WithEffectRepository(effectRepo))
	runID := "run-tester-absorbed"
	task := testerFailureStopTask("task-tester-absorbed", runID, "TesterWorker", 3)
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusSucceeded}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	if err := orch.recordReviewerRunnerFailure(ctx, runID, "late failure after mcp acceptance"); err != nil {
		t.Fatalf("record reviewer runner failure: %v", err)
	}

	advanced, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if advanced.Status != model.TaskStatusRunning {
		t.Fatalf("terminal run must not pull the task back: got %s", advanced.Status)
	}
	if _, err := effectRepo.GetByDedupeKey(ctx, "human-required-im:"+task.TaskID+":"+runID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("absorbed late failure must not notify: err=%v", err)
	}
}
