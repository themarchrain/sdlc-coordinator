package scheduler

import (
	"context"
	"time"

	"sdlc-coordinator/internal/orchestrator"

	"go.uber.org/zap"
)

const fallbackInterval = 30 * time.Second

type TickFunc func(context.Context, orchestrator.TickRequest) (*orchestrator.TickResult, error)

type WaitFunc func(context.Context, time.Duration) bool

type Option func(*Runner)

type Runner struct {
	interval time.Duration
	request  orchestrator.TickRequest
	tick     TickFunc
	wait     WaitFunc
	logger   *zap.Logger
}

func New(interval time.Duration, request orchestrator.TickRequest, tick TickFunc, logger *zap.Logger, options ...Option) *Runner {
	if interval <= 0 {
		interval = fallbackInterval
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	runner := &Runner{
		interval: interval,
		request:  request,
		tick:     tick,
		wait:     waitForInterval,
		logger:   logger,
	}
	for _, option := range options {
		option(runner)
	}
	return runner
}

func withWaitFunc(wait WaitFunc) Option {
	return func(runner *Runner) {
		if wait != nil {
			runner.wait = wait
		}
	}
}

func (r *Runner) Run(ctx context.Context) {
	r.logger.Info("internal scheduler started", zap.Duration("interval", r.interval))
	defer r.logger.Info("internal scheduler stopped")
	if r.tick == nil {
		r.logger.Error("internal scheduler has no tick function")
		return
	}

	for r.wait(ctx, r.interval) {
		startedAt := time.Now()
		result, err := r.tick(ctx, r.request)
		if err != nil {
			r.logger.Error("internal scheduler tick failed",
				zap.Error(err),
				zap.Duration("duration", time.Since(startedAt)),
			)
			continue
		}
		if result == nil {
			result = &orchestrator.TickResult{}
		}
		logLevel := zap.InfoLevel
		if tickResultIsIdle(result) {
			logLevel = zap.DebugLevel
		}
		r.logger.Log(logLevel, "internal scheduler tick completed",
			zap.Int("polled_runs", len(result.PolledRuns)),
			zap.Int("reconciled_writebacks", len(result.ReconciledWritebacks)),
			zap.Int("finalized_results", len(result.FinalizedResults)),
			zap.Int("recoveries", len(result.Recoveries)),
			zap.Int("dispatched_tasks", len(result.DispatchedTasks)),
			zap.Int("dispatch_failures", len(result.DispatchFailures)),
			zap.Int("executed_effects", len(result.ExecutedEffects)),
			zap.Duration("duration", time.Since(startedAt)),
		)
	}
}

func tickResultIsIdle(result *orchestrator.TickResult) bool {
	return len(result.PolledRuns) == 0 &&
		len(result.ReconciledWritebacks) == 0 &&
		len(result.FinalizedResults) == 0 &&
		len(result.Recoveries) == 0 &&
		len(result.DispatchedTasks) == 0 &&
		len(result.DispatchFailures) == 0 &&
		len(result.ExecutedEffects) == 0
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
