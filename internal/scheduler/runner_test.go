package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"sdlc-coordinator/internal/orchestrator"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type controlledWaiter struct {
	ready   chan time.Duration
	trigger chan struct{}
}

func newControlledWaiter() *controlledWaiter {
	return &controlledWaiter{
		ready:   make(chan time.Duration, 10),
		trigger: make(chan struct{}),
	}
}

func (w *controlledWaiter) wait(ctx context.Context, interval time.Duration) bool {
	w.ready <- interval
	select {
	case <-ctx.Done():
		return false
	case <-w.trigger:
		return true
	}
}

func receiveWithin[T any](t *testing.T, channel <-chan T) T {
	t.Helper()
	select {
	case value := <-channel:
		return value
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for scheduler event")
		var zero T
		return zero
	}
}

func assertNoValue[T any](t *testing.T, channel <-chan T) {
	t.Helper()
	select {
	case value := <-channel:
		t.Fatalf("unexpected scheduler event: %#v", value)
	default:
	}
}

func TestRunnerWaitsBeforeFirstTickAndPassesConfiguredRequest(t *testing.T) {
	waiter := newControlledWaiter()
	calls := make(chan orchestrator.TickRequest, 1)
	request := orchestrator.TickRequest{DispatchLimit: 1, EffectLimit: 5}
	runner := New(30*time.Second, request, func(_ context.Context, got orchestrator.TickRequest) (*orchestrator.TickResult, error) {
		calls <- got
		return &orchestrator.TickResult{}, nil
	}, zap.NewNop(), withWaitFunc(waiter.wait))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runner.Run(ctx)

	if interval := receiveWithin(t, waiter.ready); interval != 30*time.Second {
		t.Fatalf("expected 30s interval, got %s", interval)
	}
	assertNoValue(t, calls)
	waiter.trigger <- struct{}{}
	if got := receiveWithin(t, calls); got != request {
		t.Fatalf("unexpected tick request: %#v", got)
	}
}

func TestRunnerDoesNotOverlapSlowTick(t *testing.T) {
	waiter := newControlledWaiter()
	started := make(chan int32, 2)
	releaseFirst := make(chan struct{})
	var callCount atomic.Int32
	runner := New(time.Minute, orchestrator.TickRequest{}, func(_ context.Context, _ orchestrator.TickRequest) (*orchestrator.TickResult, error) {
		call := callCount.Add(1)
		started <- call
		if call == 1 {
			<-releaseFirst
		}
		return &orchestrator.TickResult{}, nil
	}, zap.NewNop(), withWaitFunc(waiter.wait))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runner.Run(ctx)

	receiveWithin(t, waiter.ready)
	waiter.trigger <- struct{}{}
	if call := receiveWithin(t, started); call != 1 {
		t.Fatalf("expected first call, got %d", call)
	}
	assertNoValue(t, waiter.ready)
	assertNoValue(t, started)

	close(releaseFirst)
	receiveWithin(t, waiter.ready)
	if got := callCount.Load(); got != 1 {
		t.Fatalf("expected one call before a fresh interval, got %d", got)
	}
	waiter.trigger <- struct{}{}
	if call := receiveWithin(t, started); call != 2 {
		t.Fatalf("expected second call after fresh interval, got %d", call)
	}
}

func TestRunnerContinuesAfterTickError(t *testing.T) {
	waiter := newControlledWaiter()
	started := make(chan int32, 2)
	var callCount atomic.Int32
	runner := New(time.Minute, orchestrator.TickRequest{}, func(_ context.Context, _ orchestrator.TickRequest) (*orchestrator.TickResult, error) {
		call := callCount.Add(1)
		started <- call
		if call == 1 {
			return nil, errors.New("temporary tick failure")
		}
		return &orchestrator.TickResult{}, nil
	}, zap.NewNop(), withWaitFunc(waiter.wait))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runner.Run(ctx)

	receiveWithin(t, waiter.ready)
	waiter.trigger <- struct{}{}
	receiveWithin(t, started)
	receiveWithin(t, waiter.ready)
	waiter.trigger <- struct{}{}
	if call := receiveWithin(t, started); call != 2 {
		t.Fatalf("expected scheduler to continue after error, got call %d", call)
	}
}

func TestRunnerLogsIdleTickAtDebugLevel(t *testing.T) {
	logs := runSingleTickAndCollectLogs(t, &orchestrator.TickResult{})
	entries := logs.FilterMessage("internal scheduler tick completed").All()
	if len(entries) != 1 {
		t.Fatalf("expected one completed tick log, got %d", len(entries))
	}
	if entries[0].Level != zapcore.DebugLevel {
		t.Fatalf("expected idle tick at debug level, got %s", entries[0].Level)
	}
}

func TestRunnerLogsActiveTickAtInfoLevel(t *testing.T) {
	tests := map[string]*orchestrator.TickResult{
		"polled run":           {PolledRuns: make([]orchestrator.PollRunTickResult, 1)},
		"reconciled writeback": {ReconciledWritebacks: make([]orchestrator.WritebackReconciliationResult, 1)},
		"finalized result":     {FinalizedResults: make([]orchestrator.ResultFinalizationResult, 1)},
		"recovery":             {Recoveries: make([]orchestrator.RecoveryResult, 1)},
		"dispatched task":      {DispatchedTasks: make([]orchestrator.DispatchTickResult, 1)},
		"dispatch failure":     {DispatchFailures: make([]orchestrator.DispatchTickResult, 1)},
		"executed effect":      {ExecutedEffects: make([]orchestrator.EffectExecutionResult, 1)},
	}
	for name, result := range tests {
		t.Run(name, func(t *testing.T) {
			logs := runSingleTickAndCollectLogs(t, result)
			entries := logs.FilterMessage("internal scheduler tick completed").All()
			if len(entries) != 1 {
				t.Fatalf("expected one completed tick log, got %d", len(entries))
			}
			if entries[0].Level != zapcore.InfoLevel {
				t.Fatalf("expected active tick at info level, got %s", entries[0].Level)
			}
		})
	}
}

func runSingleTickAndCollectLogs(t *testing.T, result *orchestrator.TickResult) *observer.ObservedLogs {
	t.Helper()
	waiter := newControlledWaiter()
	core, logs := observer.New(zapcore.DebugLevel)
	runner := New(time.Minute, orchestrator.TickRequest{}, func(_ context.Context, _ orchestrator.TickRequest) (*orchestrator.TickResult, error) {
		return result, nil
	}, zap.New(core), withWaitFunc(waiter.wait))

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		runner.Run(ctx)
		close(stopped)
	}()

	receiveWithin(t, waiter.ready)
	waiter.trigger <- struct{}{}
	receiveWithin(t, waiter.ready)
	cancel()
	receiveWithin(t, stopped)
	return logs
}

func TestRunnerStopsWhenContextIsCancelled(t *testing.T) {
	waiter := newControlledWaiter()
	stopped := make(chan struct{})
	runner := New(time.Minute, orchestrator.TickRequest{}, func(_ context.Context, _ orchestrator.TickRequest) (*orchestrator.TickResult, error) {
		t.Fatal("tick must not run after cancellation")
		return nil, nil
	}, zap.NewNop(), withWaitFunc(waiter.wait))

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		runner.Run(ctx)
		close(stopped)
	}()

	receiveWithin(t, waiter.ready)
	cancel()
	receiveWithin(t, stopped)
}
