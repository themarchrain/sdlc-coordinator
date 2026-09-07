package orchestrator

import (
	"context"
	"errors"
	"testing"

	"sdlc-coordinator/internal/model"
)

type blockingReviewerCoordinator struct {
	entered chan struct{}
	release chan struct{}
	calls   int
}

func (f *blockingReviewerCoordinator) Start(_ context.Context, request ReviewerRunnerStartRequest) (*model.AgentExecution, error) {
	return &model.AgentExecution{RunID: request.Run.RunID, ExecutionID: request.Run.RunID, Status: model.AgentExecutionStatusAccepted, RequestHash: "request-hash"}, nil
}

func (f *blockingReviewerCoordinator) Reconcile(context.Context, string) (*ReviewerRunnerReconcileResult, error) {
	f.calls++
	if f.calls == 1 {
		f.entered <- struct{}{}
		<-f.release
	}
	return &ReviewerRunnerReconcileResult{Execution: &model.AgentExecution{RunID: "run-single-flight", ExecutionID: "run-single-flight", Status: model.AgentExecutionStatusRunning}}, nil
}

func TestTickIsSingleFlight(t *testing.T) {
	ctx := context.Background()
	tasks, runs := newTickRepositories(t)
	reviewer := &blockingReviewerCoordinator{entered: make(chan struct{}), release: make(chan struct{})}
	orch := New(tasks, runs, WithReviewerRunner(reviewer, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}))
	runID := "run-single-flight"
	task := reviewerRunnerTickTask("task-single-flight", model.TaskStatusRunning)
	task.CurrentRunID = &runID
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := runs.Create(ctx, &model.Run{
		RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning,
		ExternalSystem: agentRunnerExternalSystem, ExternalID: runID,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := orch.Tick(ctx, TickRequest{PollLimit: 10})
		done <- err
	}()
	<-reviewer.entered
	if _, err := orch.Tick(ctx, TickRequest{PollLimit: 10}); !errors.Is(err, ErrTickInProgress) {
		t.Fatalf("concurrent tick must be rejected with ErrTickInProgress, got %v", err)
	}
	close(reviewer.release)
	if err := <-done; err != nil {
		t.Fatalf("first tick failed: %v", err)
	}
	if _, err := orch.Tick(ctx, TickRequest{}); err != nil {
		t.Fatalf("tick must be accepted again after the previous one finished: %v", err)
	}
}

type flakyRunRepository struct {
	RunRepository
	failFor map[string]int
}

func (r *flakyRunRepository) GetByRunID(ctx context.Context, runID string) (*model.Run, error) {
	if r.failFor[runID] > 0 {
		r.failFor[runID]--
		return nil, errors.New("transient read failure")
	}
	return r.RunRepository.GetByRunID(ctx, runID)
}

func TestTickKeepsProcessingWhenRunRereadFails(t *testing.T) {
	ctx := context.Background()
	tasks, runs := newTickRepositories(t)
	reviewer := &fakeReviewerRunnerCoordinator{reconcileResult: &ReviewerRunnerReconcileResult{
		Execution: &model.AgentExecution{Status: model.AgentExecutionStatusRunning},
	}}
	flakyRuns := &flakyRunRepository{RunRepository: runs, failFor: map[string]int{"run-second": 1}}
	orch := New(tasks, flakyRuns, WithReviewerRunner(reviewer, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}))
	for _, runID := range []string{"run-first", "run-second"} {
		task := reviewerRunnerTickTask("task-"+runID, model.TaskStatusRunning)
		task.CurrentRunID = &runID
		if err := tasks.Create(ctx, task); err != nil {
			t.Fatalf("create task: %v", err)
		}
		if err := runs.Create(ctx, &model.Run{
			RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning,
			ExternalSystem: agentRunnerExternalSystem, ExternalID: runID,
		}); err != nil {
			t.Fatalf("create run: %v", err)
		}
	}

	result, err := orch.Tick(ctx, TickRequest{PollLimit: 10})
	if err != nil {
		t.Fatalf("one run re-read failure must not abort the tick: %v", err)
	}
	if len(result.PolledRuns) != 2 {
		t.Fatalf("both runs must be polled: %#v", result.PolledRuns)
	}
	if result.PolledRuns[0].Error != "" {
		t.Fatalf("first run must reconcile cleanly: %#v", result.PolledRuns[0])
	}
	if result.PolledRuns[1].Error == "" {
		t.Fatalf("second run re-read failure must be recorded on its own poll entry: %#v", result.PolledRuns)
	}
}
