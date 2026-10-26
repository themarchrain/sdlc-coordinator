package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store/repository"
)

type stopRunnerFunc func(context.Context, string, runner.CancellationRequest) (*runner.CancellationAck, error)

func (f stopRunnerFunc) CancelDurable(ctx context.Context, id string, r runner.CancellationRequest) (*runner.CancellationAck, error) {
	return f(ctx, id, r)
}

func reconcileStopWorld(t *testing.T) (*intentWorld, *Orchestrator, *model.StopRequest) {
	t.Helper()
	w := admissionIntentWorld(t)
	hash, _ := runner.RequestHash(w.request)
	if _, err := w.coord.persistIntent(context.Background(), w.run, w.request, "runner-a", hash); err != nil {
		t.Fatal(err)
	}
	stops := repository.NewStopRepository(w.db)
	decision, err := stops.Stop(context.Background(), repository.StopTarget{TaskID: w.task.TaskID}, "admin", "", 3)
	if err != nil {
		t.Fatal(err)
	}
	o := New(w.tasks, w.runs, WithStopRepository(stops), WithWorkItemLeases(repository.NewWorkItemLeaseRepository(w.db)), WithAgentExecutionLookup(w.executions))
	return w, o, decision.Stop
}

func TestStopUsesPinnedRunnerOnly(t *testing.T) {
	w, o, stop := reconcileStopWorld(t)
	calls := 0
	o.stopRunnerResolver = func(endpoint string) (StopRunnerClient, error) {
		if endpoint != "runner-a" {
			t.Fatalf("unfixed target: %s", endpoint)
		}
		return stopRunnerFunc(func(_ context.Context, id string, r runner.CancellationRequest) (*runner.CancellationAck, error) {
			calls++
			return &runner.CancellationAck{StopID: r.StopID, TaskID: r.TaskID, ExecutionID: id, CancelIntentPersisted: true, FutureCreateBlocked: true}, nil
		}), nil
	}
	if err := o.ReconcileStops(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	var stored model.StopRequest
	if err := w.db.Where("stop_id = ?", stop.StopID).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if calls != 1 || stored.DeliveryStatus != model.StopDeliveryConfirmed || stored.ConfirmationKind != model.StopConfirmationCreateBlocked {
		t.Fatalf("fixed cancellation not converged: calls=%d stop=%+v", calls, stored)
	}
	result, err := o.StopTask(context.Background(), w.task.TaskID, StopActor{UserID: "admin"}, "")
	if err != nil || result.ExitConfirmed {
		t.Fatalf("tombstone falsely claimed process exit: %+v %v", result, err)
	}
}

func TestStopLostAckRetriesSameDecision(t *testing.T) {
	w, o, stop := reconcileStopWorld(t)
	calls := 0
	o.stopRunnerResolver = func(string) (StopRunnerClient, error) {
		return stopRunnerFunc(func(_ context.Context, id string, r runner.CancellationRequest) (*runner.CancellationAck, error) {
			calls++
			if r.StopID != stop.StopID || id != stop.ExecutionID {
				t.Fatal("retry changed identity")
			}
			if calls == 1 {
				return nil, errors.New("ack lost")
			}
			return &runner.CancellationAck{StopID: r.StopID, TaskID: r.TaskID, ExecutionID: id, CancelIntentPersisted: true, FutureCreateBlocked: true}, nil
		}), nil
	}
	for i := 0; i < 2; i++ {
		if err := o.ReconcileStops(context.Background(), 20); err != nil {
			t.Fatal(err)
		}
		if err := w.db.Model(&model.StopRequest{}).Where("stop_id = ?", stop.StopID).Update("next_attempt_at", time.Now().UTC().Add(-time.Second)).Error; err != nil {
			t.Fatal(err)
		}
	}
	var stored model.StopRequest
	w.db.Where("stop_id = ?", stop.StopID).First(&stored)
	if calls != 2 || stored.ConfirmationKind != model.StopConfirmationCreateBlocked {
		t.Fatalf("retry did not converge: calls=%d stop=%+v", calls, stored)
	}
}

func TestLegacyRunner404IsNotConfirmation(t *testing.T) {
	w, o, stop := reconcileStopWorld(t)
	o.stopRunnerResolver = func(string) (StopRunnerClient, error) {
		return stopRunnerFunc(func(context.Context, string, runner.CancellationRequest) (*runner.CancellationAck, error) {
			return nil, &runner.HTTPError{StatusCode: 404}
		}), nil
	}
	if err := o.ReconcileStops(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	var stored model.StopRequest
	w.db.Where("stop_id = ?", stop.StopID).First(&stored)
	if stored.DeliveryStatus == model.StopDeliveryConfirmed || stored.ConfirmationKind != "" || stored.AttemptCount != 1 {
		t.Fatalf("404 fabricated proof: %+v", stored)
	}
}

func TestExitConfirmationRequiresMatchingRunnerEvidence(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(fmt.Sprint(valid), func(t *testing.T) {
			w, o, stop := reconcileStopWorld(t)
			o.stopRunnerResolver = func(string) (StopRunnerClient, error) {
				return stopRunnerFunc(func(_ context.Context, id string, r runner.CancellationRequest) (*runner.CancellationAck, error) {
					if !valid {
						id = "wrong-execution"
					}
					return &runner.CancellationAck{StopID: r.StopID, TaskID: r.TaskID, ExecutionID: id, CancelIntentPersisted: true, FutureCreateBlocked: true, ExecutionPresent: true, ExitConfirmed: true, Execution: &runner.Execution{ExecutionID: id, TaskID: r.TaskID, RequestHash: stopHash(w), Status: runner.StatusCancelled, FinishedAt: ptrTime(time.Now())}}, nil
				}), nil
			}
			if err := o.ReconcileStops(context.Background(), 20); err != nil {
				t.Fatal(err)
			}
			var stored model.StopRequest
			w.db.Where("stop_id = ?", stop.StopID).First(&stored)
			if (stored.ConfirmationKind == model.StopConfirmationExecutionExited) != valid {
				t.Fatalf("bad evidence decision: %+v", stored)
			}
		})
	}
}

func stopHash(w *intentWorld) string     { hash, _ := runner.RequestHash(w.request); return hash }
func ptrTime(value time.Time) *time.Time { return &value }

func TestStopTakeoverRejectsStaleAck(t *testing.T) {
	w, o, stop := reconcileStopWorld(t)
	o.stopRunnerResolver = func(string) (StopRunnerClient, error) {
		return stopRunnerFunc(func(_ context.Context, id string, r runner.CancellationRequest) (*runner.CancellationAck, error) {
			if err := w.db.Model(&model.WorkItemLease{}).Where("scope = ?", "stop:"+stop.StopID).Update("lease_until", time.Now().UTC().Add(-time.Second)).Error; err != nil {
				t.Fatal(err)
			}
			_, won, err := repository.NewWorkItemLeaseRepository(w.db).Acquire(context.Background(), "stop:"+stop.StopID, "other-coordinator", "stop-delivery", time.Minute)
			if err != nil || !won {
				t.Fatalf("takeover: %v", err)
			}
			return &runner.CancellationAck{StopID: r.StopID, TaskID: r.TaskID, ExecutionID: id, CancelIntentPersisted: true, FutureCreateBlocked: true}, nil
		}), nil
	}
	if err := o.ReconcileStops(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	var stored model.StopRequest
	w.db.Where("stop_id = ?", stop.StopID).First(&stored)
	if stored.ConfirmationKind != "" || stored.DeliveryStatus == model.StopDeliveryConfirmed {
		t.Fatalf("stale owner wrote confirmation: %+v", stored)
	}
}

func TestStopWakeSurvivesBrowserCancel(t *testing.T) {
	w, o, _ := reconcileStopWorld(t)
	called, release := make(chan struct{}), make(chan struct{})
	o.stopRunnerResolver = func(string) (StopRunnerClient, error) {
		return stopRunnerFunc(func(ctx context.Context, id string, r runner.CancellationRequest) (*runner.CancellationAck, error) {
			close(called)
			<-release
			if ctx.Err() != nil {
				t.Errorf("browser cancelled reconciliation: %v", ctx.Err())
			}
			return &runner.CancellationAck{StopID: r.StopID, TaskID: r.TaskID, ExecutionID: id, CancelIntentPersisted: true, FutureCreateBlocked: true}, nil
		}), nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := o.StopTask(ctx, w.task.TaskID, StopActor{UserID: "admin"}, ""); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-called:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("wake missing")
	}
	close(release)
	o.stopWakeMu.Lock()
	o.stopWakeMu.Unlock()
}

func TestStopBatchBounded(t *testing.T) {
	w, o, _ := reconcileStopWorld(t)
	for i := 0; i < 24; i++ {
		runID, taskID := fmt.Sprintf("run-%d", i), fmt.Sprintf("task-%d", i)
		for _, row := range []any{&model.Task{TaskID: taskID, DedupeKey: taskID, Type: "test", Role: "TesterWorker", Status: model.TaskStatusRunning, CurrentRunID: &runID}, &model.Run{RunID: runID, TaskID: taskID, Status: model.RunStatusRunning}, &model.AgentExecution{RunID: runID, ExecutionID: runID, RunnerEndpoint: "runner-a", Status: model.AgentExecutionStatusDispatching, Provider: "fake", RequestHash: "hash"}} {
			if err := w.db.Create(row).Error; err != nil {
				t.Fatal(err)
			}
		}
		if _, err := repository.NewStopRepository(w.db).Stop(context.Background(), repository.StopTarget{TaskID: taskID}, "admin", "", 3); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	o.stopRunnerResolver = func(string) (StopRunnerClient, error) {
		return stopRunnerFunc(func(_ context.Context, id string, r runner.CancellationRequest) (*runner.CancellationAck, error) {
			calls++
			return &runner.CancellationAck{StopID: r.StopID, TaskID: r.TaskID, ExecutionID: id, CancelIntentPersisted: true, FutureCreateBlocked: true}, nil
		}), nil
	}
	if err := o.ReconcileStops(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	if calls != 20 {
		t.Fatalf("batch unbounded or lost work: %d", calls)
	}
	if err := o.ReconcileStops(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	if calls != 25 {
		t.Fatalf("remaining work lost: %d", calls)
	}
}

func TestStopRestartTakeover(t *testing.T) {
	w, o, stop := reconcileStopWorld(t)
	if err := w.db.Model(&model.StopRequest{}).Where("stop_id = ?", stop.StopID).Updates(map[string]any{"delivery_status": model.StopDeliveryConfirmed, "confirmation_kind": "", "confirmed_at": time.Now().UTC(), "next_attempt_at": nil}).Error; err != nil {
		t.Fatal(err)
	}
	o.stopRunnerResolver = func(string) (StopRunnerClient, error) {
		return stopRunnerFunc(func(_ context.Context, id string, r runner.CancellationRequest) (*runner.CancellationAck, error) {
			return &runner.CancellationAck{StopID: r.StopID, TaskID: r.TaskID, ExecutionID: id, CancelIntentPersisted: true, FutureCreateBlocked: true}, nil
		}), nil
	}
	if err := o.ReconcileStops(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	var stored model.StopRequest
	w.db.Where("stop_id = ?", stop.StopID).First(&stored)
	if stored.ConfirmationKind != model.StopConfirmationCreateBlocked {
		t.Fatalf("legacy confirmation was not rechecked: %+v", stored)
	}
}

func TestStopRoundHasTotalNetworkBudget(t *testing.T) {
	w, o, _ := reconcileStopWorld(t)
	runID, taskID := "other-run", "other-task"
	for _, row := range []any{&model.Task{TaskID: taskID, DedupeKey: taskID, Type: "test", Role: "TesterWorker", Status: model.TaskStatusRunning, CurrentRunID: &runID}, &model.Run{RunID: runID, TaskID: taskID, Status: model.RunStatusRunning}, &model.AgentExecution{RunID: runID, ExecutionID: runID, RunnerEndpoint: "runner-a", Status: model.AgentExecutionStatusRunning, Provider: "fake", RequestHash: "hash"}} {
		if err := w.db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repository.NewStopRepository(w.db).Stop(context.Background(), repository.StopTarget{TaskID: taskID}, "admin", "", 3); err != nil {
		t.Fatal(err)
	}
	calls := 0
	o.stopRunnerResolver = func(string) (StopRunnerClient, error) {
		return stopRunnerFunc(func(ctx context.Context, _ string, _ runner.CancellationRequest) (*runner.CancellationAck, error) {
			calls++
			<-ctx.Done()
			return nil, ctx.Err()
		}), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6500*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := o.ReconcileStops(ctx, 20)
	if err != nil || time.Since(start) > 6*time.Second || calls != 1 {
		t.Fatalf("stop batch blocked scheduler: elapsed=%v calls=%d err=%v", time.Since(start), calls, err)
	}
	var confirmed int64
	if err := w.db.Model(&model.StopRequest{}).Where("delivery_status = ?", model.StopDeliveryConfirmed).Count(&confirmed).Error; err != nil || confirmed != 0 {
		t.Fatalf("false confirmation=%v %v", confirmed, err)
	}
}

func TestCreateBlockedClosesDispatchingProjection(t *testing.T) {
	w, o, _ := reconcileStopWorld(t)
	o.stopRunnerResolver = func(string) (StopRunnerClient, error) {
		return stopRunnerFunc(func(_ context.Context, id string, r runner.CancellationRequest) (*runner.CancellationAck, error) {
			return &runner.CancellationAck{StopID: r.StopID, TaskID: r.TaskID, ExecutionID: id, CancelIntentPersisted: true, FutureCreateBlocked: true}, nil
		}), nil
	}
	if err := o.ReconcileStops(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	got, err := w.executions.GetByRunID(context.Background(), w.run.RunID)
	if err != nil || got.Status != model.AgentExecutionStatusInterrupted || got.ErrorCode != "create_blocked_by_stop" || got.FinishedAt == nil {
		t.Fatalf("phantom execution: %+v %v", got, err)
	}
}

type faultyStopDelivery struct {
	*repository.StopRepository
	id    string
	block bool
}

type blockedStopList struct{ *repository.StopRepository }

func (r *blockedStopList) ListPending(ctx context.Context, _ int) ([]model.StopRequest, error) {
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 1100*time.Millisecond {
		return nil, errors.New("pending query lacks its one-second database budget")
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestStopPendingQueryHasDatabaseBudget(t *testing.T) {
	w, o, _ := reconcileStopWorld(t)
	o.stops = &blockedStopList{repository.NewStopRepository(w.db)}
	if err := o.ReconcileStops(context.Background(), 20); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("query budget: %v", err)
	}
}

func (r *faultyStopDelivery) BeginDelivery(ctx context.Context, id string, lease model.WorkItemLease) (*model.StopRequest, error) {
	if id == r.id {
		if r.block {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return nil, errors.New("temporary row failure")
	}
	return r.StopRepository.BeginDelivery(ctx, id, lease)
}
func TestStopRowDatabaseFailureDoesNotStarveOtherStops(t *testing.T) {
	for _, block := range []bool{false, true} {
		t.Run(fmt.Sprint(block), func(t *testing.T) {
			w, o, first := reconcileStopWorld(t)
			runID, taskID := "healthy-run", "healthy-task"
			for _, row := range []any{&model.Task{TaskID: taskID, DedupeKey: taskID, Type: "test", Role: "TesterWorker", Status: model.TaskStatusRunning, CurrentRunID: &runID}, &model.Run{RunID: runID, TaskID: taskID, Status: model.RunStatusRunning}, &model.AgentExecution{RunID: runID, ExecutionID: runID, RunnerEndpoint: "runner-a", Provider: "fake", Status: model.AgentExecutionStatusDispatching, RequestHash: "hash"}} {
				if err := w.db.Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}
			stops := repository.NewStopRepository(w.db)
			if _, err := stops.Stop(context.Background(), repository.StopTarget{TaskID: taskID}, "admin", "", 3); err != nil {
				t.Fatal(err)
			}
			o.stops = &faultyStopDelivery{StopRepository: stops, id: first.StopID, block: block}
			calls := 0
			o.stopRunnerResolver = func(string) (StopRunnerClient, error) {
				return stopRunnerFunc(func(_ context.Context, id string, r runner.CancellationRequest) (*runner.CancellationAck, error) {
					calls++
					return &runner.CancellationAck{StopID: r.StopID, TaskID: r.TaskID, ExecutionID: id, CancelIntentPersisted: true, FutureCreateBlocked: true}, nil
				}), nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
			defer cancel()
			if err := o.ReconcileStops(ctx, 20); err != nil {
				t.Fatalf("row failure aborted round: %v", err)
			}
			healthy, err := stops.GetForTask(context.Background(), taskID)
			if err != nil || healthy.ConfirmationKind != model.StopConfirmationCreateBlocked || calls != 1 {
				t.Fatalf("healthy target starved: %+v calls=%d err=%v", healthy, calls, err)
			}
		})
	}
}
