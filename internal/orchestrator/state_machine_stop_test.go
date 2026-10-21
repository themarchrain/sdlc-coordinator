package orchestrator

import (
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
)

func TestStateMachineManualStop(t *testing.T) {
	machine := NewStateMachine()
	interrupted := model.TaskStatus("interrupted")
	stoppable := []model.TaskStatus{
		model.TaskStatusPending, model.TaskStatusClaimed, model.TaskStatusRunning,
		model.TaskStatusFinalizing, model.TaskStatusRetryPending, model.TaskStatusWaitingUpstream,
		model.TaskStatusWaitingMerge, model.TaskStatusHumanRequired, model.TaskStatusBlocked,
	}
	for _, from := range stoppable {
		t.Run(string(from), func(t *testing.T) {
			task := &model.Task{Status: from}
			if err := machine.Transition(task, interrupted); err != nil {
				t.Fatalf("manual stop rejected: %v", err)
			}
			if task.Status != interrupted {
				t.Fatalf("status = %s, want interrupted", task.Status)
			}
			if machine.CanTransition(interrupted, from) {
				t.Fatalf("interrupted task can return to %s", from)
			}
		})
	}
	for _, from := range []model.TaskStatus{model.TaskStatusSucceeded, "future_status"} {
		if machine.CanTransition(from, interrupted) {
			t.Errorf("manual stop must not rewrite %s", from)
		}
	}
	for _, to := range []model.TaskStatus{model.TaskStatusSucceeded, "future_status"} {
		if machine.CanTransition(interrupted, to) {
			t.Errorf("interrupted task can advance to %s", to)
		}
	}
	if err := machine.Transition(&model.Task{Status: interrupted}, interrupted); err != nil {
		t.Fatalf("repeated interruption must be idempotent: %v", err)
	}
}

func TestManualStopClearsTaskLeaseWithoutNewAttempt(t *testing.T) {
	now := time.Now().UTC()
	lease := now.Add(time.Hour)
	task := &model.Task{Status: model.TaskStatus("interrupted"), LeaseUntil: &lease, AttemptCount: 3}
	DefaultPolicy().Apply(task, model.TaskStatusRunning, task.Status, now)
	if task.LeaseUntil != nil {
		t.Fatal("interrupted task retained its active lease")
	}
	if task.AttemptCount != 3 {
		t.Fatalf("manual stop changed business attempts to %d", task.AttemptCount)
	}
}
