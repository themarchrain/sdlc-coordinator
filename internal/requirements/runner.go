package requirements

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
)

const SyncInterval = time.Minute

type SyncFunc func(context.Context) (SyncResult, error)

type RunnerOption func(*Runner)

type Runner struct {
	interval time.Duration
	sync     SyncFunc
	wait     func(context.Context, time.Duration) bool
	logger   *zap.Logger
}

func NewRunner(interval time.Duration, sync SyncFunc, logger *zap.Logger, options ...RunnerOption) *Runner {
	if interval <= 0 {
		interval = SyncInterval
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	runner := &Runner{interval: interval, sync: sync, wait: waitForInterval, logger: logger}
	for _, option := range options {
		option(runner)
	}
	return runner
}

func WithWait(wait func(context.Context, time.Duration) bool) RunnerOption {
	return func(runner *Runner) {
		if wait != nil {
			runner.wait = wait
		}
	}
}

func (runner *Runner) Run(ctx context.Context) {
	runner.logger.Info("requirements syncer started", zap.Duration("interval", runner.interval))
	defer runner.logger.Info("requirements syncer stopped")
	if runner.sync == nil {
		runner.logger.Error("requirements syncer has no sync function")
		return
	}
	for runner.wait(ctx, runner.interval) {
		startedAt := time.Now()
		result, err := runner.sync(ctx)
		if err != nil {
			if errors.Is(err, ErrSyncInProgress) {
				runner.logger.Debug("requirements sync already running")
				continue
			}
			runner.logger.Warn("requirements sync failed", zap.Error(err), zap.Duration("duration", time.Since(startedAt)))
			continue
		}
		level := zap.DebugLevel
		if result.Created > 0 || result.Recovered > 0 || result.Failed > 0 || result.Mirrored > 0 || result.MirrorErrors > 0 {
			level = zap.InfoLevel
		}
		runner.logger.Log(level, "requirements sync completed",
			zap.Int("created", result.Created),
			zap.Int("recovered", result.Recovered),
			zap.Int("failed", result.Failed),
			zap.Int("skipped", result.Skipped),
			zap.Int("mirrored", result.Mirrored),
			zap.Int("mirror_errors", result.MirrorErrors),
			zap.Duration("duration", time.Since(startedAt)),
		)
	}
}

func waitForInterval(ctx context.Context, interval time.Duration) bool {
	timer := time.NewTimer(interval)
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
