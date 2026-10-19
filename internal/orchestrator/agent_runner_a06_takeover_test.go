package orchestrator

import (
	"context"
	"errors"
	"testing"

	"sdlc-coordinator/internal/model"
)

type failFirstExecutionProjectionUpdate struct {
	AgentExecutionProjectionRepository
	failed bool
}

func (r *failFirstExecutionProjectionUpdate) Update(ctx context.Context, execution *model.AgentExecution) error {
	if !r.failed {
		r.failed = true
		return errors.New("simulated coordinator exit before execution projection")
	}
	return r.AgentExecutionProjectionRepository.Update(ctx, execution)
}

func TestA06TakeoverReusesExecutionStartedBeforeCoordinatorExit(t *testing.T) {
	w := newIntentWorld(t)
	ctx := context.Background()

	coordinatorA := NewAgentRunnerCoordinator(w.tasks, w.runs,
		&failFirstExecutionProjectionUpdate{AgentExecutionProjectionRepository: w.executions}, w.runnerA)
	coordinatorA.setTenantRunnerRouter(w.router)
	if _, err := coordinatorA.Start(ctx, w.task, w.run, w.request); err == nil {
		t.Fatal("first coordinator should stop when it cannot persist the remote execution projection")
	}
	if w.runnerA.execution == nil || w.runnerA.createCalls != 1 {
		t.Fatalf("remote execution was not started exactly once: execution=%+v create calls=%d", w.runnerA.execution, w.runnerA.createCalls)
	}

	coordinatorB := NewAgentRunnerCoordinator(w.tasks, w.runs, w.executions, w.runnerA)
	coordinatorB.setTenantRunnerRouter(w.router)
	projection, err := coordinatorB.Start(ctx, w.task, w.run, w.request)
	if err != nil {
		t.Fatalf("takeover coordinator could not recover the original execution: %v", err)
	}
	if projection.Status != model.AgentExecutionStatusRunning || projection.RunnerEndpoint != "runner-a" {
		t.Fatalf("takeover projection = %+v, want running execution on runner-a", projection)
	}
	if w.runnerA.createCalls != 1 || w.runnerA.getCalls != 1 {
		t.Fatalf("takeover must read the original execution without creating another: creates=%d gets=%d", w.runnerA.createCalls, w.runnerA.getCalls)
	}
	if w.runnerB.createCalls != 0 || w.runnerB.getCalls != 0 {
		t.Fatalf("takeover must stay on the recorded runner: runner-b creates=%d gets=%d", w.runnerB.createCalls, w.runnerB.getCalls)
	}
	stored, err := w.executions.GetByRunID(ctx, w.run.RunID)
	if err != nil || stored.Status != model.AgentExecutionStatusRunning || stored.RunnerEndpoint != "runner-a" {
		t.Fatalf("recovered execution was not persisted: execution=%+v err=%v", stored, err)
	}
	var rows int64
	if err := w.db.Model(&model.AgentExecution{}).Count(&rows).Error; err != nil || rows != 1 {
		t.Fatalf("execution projection rows=%d err=%v, want one original row", rows, err)
	}
}
