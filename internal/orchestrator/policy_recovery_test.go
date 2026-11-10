package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"sdlc-coordinator/internal/failure"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
)

func TestRecoveryCycleBudget(t *testing.T) {
	policy := Policy{MaxAttempts: 3}
	task := &model.Task{
		RecoveryGeneration:    1,
		RetryCycleBaseCount:   5,
		RetryCycleMaxAttempts: 3,
		AttemptCount:          5,
	}
	if got := policy.AttemptsRemaining(task); got != 3 {
		t.Fatalf("initial remaining = %d, want 3", got)
	}
	task.AttemptCount = 6
	if got := policy.AttemptsRemaining(task); got != 2 {
		t.Fatalf("after one run remaining = %d, want 2", got)
	}
	task.AttemptCount = 7
	if got := policy.AttemptsRemaining(task); got != 1 {
		t.Fatalf("after lease recovery remaining = %d, want 1", got)
	}
	task.AttemptCount = 8
	if got := policy.AttemptsRemaining(task); got != 0 {
		t.Fatalf("exhausted remaining = %d, want 0", got)
	}

	task.AttemptCount = 6
	policy.MaxAttempts = 99
	if got := policy.AttemptsRemaining(task); got != 2 {
		t.Fatalf("global max changed current cycle budget: %d, want 2", got)
	}

	corrupt := &model.Task{RecoveryGeneration: 1, RetryCycleBaseCount: 5, RetryCycleMaxAttempts: 3, AttemptCount: 4}
	if got := policy.AttemptsRemaining(corrupt); got != 0 {
		t.Fatalf("corrupt baseline produced extra budget: %d", got)
	}
	negative := &model.Task{RecoveryGeneration: 1, RetryCycleBaseCount: 5, RetryCycleMaxAttempts: -1, AttemptCount: 5}
	if got := policy.AttemptsRemaining(negative); got != 0 {
		t.Fatalf("negative max produced extra budget: %d", got)
	}
	legacy := &model.Task{AttemptCount: 3}
	if got := (Policy{MaxAttempts: 3}).AttemptsRemaining(legacy); got != 0 {
		t.Fatalf("legacy exhausted remaining = %d, want 0", got)
	}
}

func TestRecoveredRunFatalStopsWithoutRetry(t *testing.T) {
	for _, test := range []struct {
		name string
		code string
	}{
		{name: "fatal", code: failure.CodeUnsupportedSchema},
		{name: "prerequisite", code: failure.CodeInvalidCredentials},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, taskRepo, runRepo, orch, task, run := recoveredFailureFixture(t)
			execution := failedExecutionWithInfo(t, run.RunID, test.code, "recovered run failed")

			if err := orch.recordReviewerRunnerFailure(ctx, run.RunID, execution); err != nil {
				t.Fatalf("record recovered run failure: %v", err)
			}
			stored, err := taskRepo.GetByTaskID(ctx, task.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Status != model.TaskStatusBlocked {
				t.Fatalf("status = %s, want blocked", stored.Status)
			}
			if stored.AttemptCount != 6 {
				t.Fatalf("attempt count = %d, want 6", stored.AttemptCount)
			}
			if !strings.Contains(stored.LastError, "recovered run failed") {
				t.Fatalf("new failure was not retained: %q", stored.LastError)
			}
			refreshedRun, err := runRepo.GetByRunID(ctx, run.RunID)
			if err != nil || refreshedRun.Status != model.RunStatusFailed {
				t.Fatalf("original run was not failed: %+v err=%v", refreshedRun, err)
			}
		})
	}
}

func TestRecoveredRunTransientAndUnknownUseRemainingBudget(t *testing.T) {
	for _, test := range []struct {
		name string
		code string
	}{
		{name: "transient", code: failure.CodeProviderRateLimited},
		{name: "unknown", code: failure.CodeProviderFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, taskRepo, _, orch, task, run := recoveredFailureFixture(t)
			execution := failedExecutionWithInfo(t, run.RunID, test.code, "retryable failure")
			if err := orch.recordReviewerRunnerFailure(ctx, run.RunID, execution); err != nil {
				t.Fatalf("record recovered run failure: %v", err)
			}
			stored, err := taskRepo.GetByTaskID(ctx, task.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Status != model.TaskStatusRetryPending {
				t.Fatalf("status = %s, want retry_pending", stored.Status)
			}
		})
	}
}

func TestLeaseRecoveryUsesCycleBudget(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	taskRepo, runRepo := newRecoveryRepositories(t)
	orch := New(taskRepo, runRepo, WithClock(func() time.Time { return now }), WithPolicy(Policy{LeaseDuration: time.Minute, MaxAttempts: 3}))

	withBudget := &model.Task{
		TaskID: "task-recovery-budget-one", DedupeKey: "dedupe-recovery-budget-one", Type: "bugfix", Role: "DeveloperWorker",
		Status: model.TaskStatusRunning, LeaseUntil: &expired, AttemptCount: 7,
		RecoveryGeneration: 1, RetryCycleBaseCount: 5, RetryCycleMaxAttempts: 3,
	}
	exhausted := &model.Task{
		TaskID: "task-recovery-budget-zero", DedupeKey: "dedupe-recovery-budget-zero", Type: "bugfix", Role: "DeveloperWorker",
		Status: model.TaskStatusRunning, LeaseUntil: &expired, AttemptCount: 8,
		RecoveryGeneration: 1, RetryCycleBaseCount: 5, RetryCycleMaxAttempts: 3,
	}
	for _, task := range []*model.Task{withBudget, exhausted} {
		if err := taskRepo.Create(ctx, task); err != nil {
			t.Fatal(err)
		}
	}

	recovered, err := orch.RecoverTask(ctx, withBudget.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != model.TaskStatusRetryPending || recovered.AttemptCount != 8 {
		t.Fatalf("unexpected budget recovery: %+v", recovered)
	}
	human, err := orch.RecoverTask(ctx, exhausted.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if human.Status != model.TaskStatusHumanRequired || human.AttemptCount != 9 {
		t.Fatalf("unexpected exhausted recovery: %+v", human)
	}
}

func TestNextExecutionFailureStatusKeepsLegacyBehavior(t *testing.T) {
	policy := Policy{MaxAttempts: 3}
	legacy := &model.Task{AttemptCount: 1}
	if got := policy.NextExecutionFailureStatus(legacy, &failure.Info{Version: failure.VersionV1, Source: failure.SourceProvider, Code: failure.CodeUnsupportedSchema}); got != model.TaskStatusRetryPending {
		t.Fatalf("legacy fatal changed status: %s", got)
	}
	recovered := &model.Task{RecoveryGeneration: 1, RetryCycleBaseCount: 5, RetryCycleMaxAttempts: 3, AttemptCount: 5}
	if got := policy.NextExecutionFailureStatus(recovered, &failure.Info{Version: failure.VersionV1, Source: failure.SourceProvider, Code: failure.CodeUnsupportedSchema}); got != model.TaskStatusBlocked {
		t.Fatalf("recovered fatal status = %s, want blocked", got)
	}
}

func recoveredFailureFixture(t *testing.T) (context.Context, *repository.TaskRepository, *repository.RunRepository, *Orchestrator, *model.Task, *model.Run) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	taskRepo, runRepo := newRecoveryRepositories(t)
	runID := model.NewID("run")
	task := &model.Task{
		TaskID: "task-recovered-failure", DedupeKey: "dedupe-recovered-failure", Type: "bugfix", Role: "DeveloperWorker",
		Status: model.TaskStatusRunning, CurrentRunID: &runID, AttemptCount: 6,
		RecoveryGeneration: 1, RetryCycleBaseCount: 5, RetryCycleMaxAttempts: 3,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatal(err)
	}
	orch := New(taskRepo, runRepo, WithClock(func() time.Time { return now }), WithPolicy(Policy{MaxAttempts: 3}))
	return ctx, taskRepo, runRepo, orch, task, run
}

func failedExecutionWithInfo(t *testing.T, runID, code, message string) *model.AgentExecution {
	t.Helper()
	info := failure.Info{Version: failure.VersionV1, Stage: failure.StageProvider, Source: failure.SourceProvider, Code: code, Summary: message}
	raw, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	return &model.AgentExecution{RunID: runID, ExecutionID: runID, Status: model.AgentExecutionStatusFailed, ErrorCode: "provider_failed", ErrorMessage: message, FailureJSON: string(raw)}
}
