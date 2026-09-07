package orchestrator

import (
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
)

func TestDefaultPolicyUsesThirtyFiveMinuteLease(t *testing.T) {
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	want := now.Add(35 * time.Minute)

	if got := DefaultPolicy().LeaseUntil(now); !got.Equal(want) {
		t.Fatalf("expected default lease until %s, got %s", want, got)
	}
	if got := (Policy{}).LeaseUntil(now); !got.Equal(want) {
		t.Fatalf("expected zero-value policy lease until %s, got %s", want, got)
	}
}

func TestStateMachineTransitions(t *testing.T) {
	sm := NewStateMachine()
	task := &model.Task{Status: model.TaskStatusPending}

	if err := sm.Transition(task, model.TaskStatusClaimed); err != nil {
		t.Fatalf("transition pending -> claimed: %v", err)
	}
	if task.Status != model.TaskStatusClaimed {
		t.Fatalf("expected claimed status, got %s", task.Status)
	}

	if err := sm.Transition(task, model.TaskStatusRunning); err != nil {
		t.Fatalf("transition claimed -> running: %v", err)
	}
	if task.Status != model.TaskStatusRunning {
		t.Fatalf("expected running status, got %s", task.Status)
	}

	if err := sm.Transition(task, model.TaskStatusSucceeded); err != nil {
		t.Fatalf("transition running -> succeeded: %v", err)
	}
	if task.Status != model.TaskStatusSucceeded {
		t.Fatalf("expected succeeded status, got %s", task.Status)
	}

	if err := sm.Transition(task, model.TaskStatusRunning); err == nil {
		t.Fatal("expected succeeded -> running transition to fail")
	}
}

func TestStateMachineAllowsHumanRequiredDeveloperResume(t *testing.T) {
	sm := NewStateMachine()
	if !sm.CanTransition(model.TaskStatusHumanRequired, model.TaskStatusPending) {
		t.Fatal("expected human_required task to be resumable to pending")
	}
}

func TestStateMachineAllowsWaitingUpstreamLoop(t *testing.T) {
	sm := NewStateMachine()
	task := &model.Task{Status: model.TaskStatusRunning}

	if err := sm.Transition(task, model.TaskStatusWaitingUpstream); err != nil {
		t.Fatalf("transition running -> waiting_upstream: %v", err)
	}
	if task.Status != model.TaskStatusWaitingUpstream {
		t.Fatalf("expected waiting_upstream status, got %s", task.Status)
	}

	if err := sm.Transition(task, model.TaskStatusPending); err != nil {
		t.Fatalf("transition waiting_upstream -> pending: %v", err)
	}
	if task.Status != model.TaskStatusPending {
		t.Fatalf("expected pending status, got %s", task.Status)
	}
}

func TestStateMachineAllowsFinalizingToTerminalOutcomes(t *testing.T) {
	sm := NewStateMachine()
	for _, terminal := range []model.TaskStatus{
		model.TaskStatusSucceeded,
		model.TaskStatusWaitingUpstream,
		model.TaskStatusRetryPending,
		model.TaskStatusBlocked,
		model.TaskStatusHumanRequired,
	} {
		task := &model.Task{Status: model.TaskStatusRunning}
		if err := sm.Transition(task, model.TaskStatusFinalizing); err != nil {
			t.Fatalf("running -> finalizing: %v", err)
		}
		if err := sm.Transition(task, terminal); err != nil {
			t.Fatalf("finalizing -> %s: %v", terminal, err)
		}
	}
}

func TestPolicyRetryDecision(t *testing.T) {
	policy := DefaultPolicy()

	task := &model.Task{AttemptCount: 1}
	if got := policy.NextFailureStatus(task); got != model.TaskStatusRetryPending {
		t.Fatalf("expected retry_pending, got %s", got)
	}

	task.AttemptCount = policy.MaxAttempts
	if got := policy.NextFailureStatus(task); got != model.TaskStatusBlocked {
		t.Fatalf("expected blocked, got %s", got)
	}
}
