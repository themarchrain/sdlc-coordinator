package repository

import (
	"context"
	"errors"
	"sdlc-coordinator/internal/model"
	"testing"
	"time"
)

func TestInterruptedExcludedFromActiveAndLeaseRenewal(t *testing.T) {
	db, stops, task := stopFixture(t, model.TaskStatusRunning)
	run, _ := bindStopExecution(t, db, task, model.RunStatusRunning)
	if _, err := stops.Stop(context.Background(), StopTarget{TaskID: task.TaskID}, "admin", "", 3); err != nil {
		t.Fatal(err)
	}
	tasks := NewTaskRepository(db)
	if err := tasks.RenewActiveLease(context.Background(), task.TaskID, run.RunID, time.Now().Add(time.Hour)); !errors.Is(err, ErrTaskLeaseNotRenewable) {
		t.Fatalf("renewed stopped task: %v", err)
	}
	active, err := NewRunRepository(db).ListRunning(context.Background(), 10)
	if err != nil || len(active) != 0 {
		t.Fatalf("stopped run active: %+v %v", active, err)
	}
}

func TestReviewerSuccessorAfterStop(t *testing.T) {
	db, stops, task := stopFixture(t, model.TaskStatusRunning)
	if err := db.Model(task).Updates(map[string]any{"role": "ReviewerWorker", "dedupe_key": "review:pr:old"}).Error; err != nil {
		t.Fatal(err)
	}
	next := &model.Task{TaskID: "next", DedupeKey: "review:pr:new", Type: "review_mr", Role: "ReviewerWorker", Status: model.TaskStatusPending}
	if err := db.Create(next).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := stops.Stop(context.Background(), StopTarget{TaskID: task.TaskID}, "admin", "", 3); err != nil {
		t.Fatal(err)
	}
	active, err := NewTaskRepository(db).HasActiveReviewerTaskForDedupePrefix(context.Background(), "review:pr:", next.TaskID)
	if err != nil || active {
		t.Fatalf("stopped predecessor blocked successor: %v %v", active, err)
	}
}

func TestLegacyRunnerRunWithoutProjectionNeedsCancellation(t *testing.T) {
	db, stops, task := stopFixture(t, model.TaskStatusRunning)
	run := &model.Run{RunID: "legacy", TaskID: task.TaskID, Status: model.RunStatusRunning, ExternalSystem: "agent-runner", ExternalID: "legacy-execution", RunnerEndpoint: "original"}
	if err := db.Create(run).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(task).Update("current_run_id", run.RunID).Error; err != nil {
		t.Fatal(err)
	}
	decision, err := stops.Stop(context.Background(), StopTarget{TaskID: task.TaskID}, "admin", "", 3)
	if err != nil || decision.Stop.ConfirmationKind != "" || decision.Stop.DeliveryStatus != model.StopDeliveryPending || decision.Stop.ExecutionID != run.ExternalID {
		t.Fatalf("legacy remote target lost: %+v %v", decision, err)
	}
}

func TestCancelledProjectionRejectsStaleObservation(t *testing.T) {
	db, _, task := stopFixture(t, model.TaskStatusRunning)
	_, execution := bindStopExecution(t, db, task, model.RunStatusRunning)
	stale := *execution
	if err := db.Model(execution).Update("status", model.AgentExecutionStatusCancelled).Error; err != nil {
		t.Fatal(err)
	}
	if err := NewAgentExecutionRepository(db).Update(context.Background(), &stale); err != nil {
		t.Fatal(err)
	}
	current, err := NewAgentExecutionRepository(db).GetByRunID(context.Background(), execution.RunID)
	if err != nil || current.Status != model.AgentExecutionStatusCancelled {
		t.Fatalf("late observation revived execution: %+v %v", current, err)
	}
}
