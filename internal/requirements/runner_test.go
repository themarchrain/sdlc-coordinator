package requirements

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunnerRunsSyncUntilContextDone(t *testing.T) {
	calls := 0
	runner := NewRunner(time.Millisecond, func(context.Context) (SyncResult, error) {
		calls++
		return SyncResult{Created: 1}, nil
	}, nil, WithWait(func(ctx context.Context, _ time.Duration) bool {
		return calls < 3
	}))
	runner.Run(context.Background())
	if calls != 3 {
		t.Fatalf("expected 3 sync calls, got %d", calls)
	}
}

func TestRunnerSurvivesSyncErrors(t *testing.T) {
	calls := 0
	runner := NewRunner(time.Millisecond, func(context.Context) (SyncResult, error) {
		calls++
		return SyncResult{}, errors.New("boom")
	}, nil, WithWait(func(ctx context.Context, _ time.Duration) bool {
		return calls < 2
	}))
	runner.Run(context.Background())
	if calls != 2 {
		t.Fatalf("sync errors must not stop the runner, got %d calls", calls)
	}
}
