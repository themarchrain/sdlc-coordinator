package orchestrator

import (
	"context"
	"errors"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
)

var ErrStopUnavailable = errors.New("task stop unavailable")

type StopActor struct{ UserID string }

type StopResult = model.StopStatus

type StopRepositoryPort interface {
	Stop(context.Context, repository.StopTarget, string, string, int) (*repository.StopDecision, error)
}

func WithStopRepository(stops StopRepositoryPort) Option {
	return func(o *Orchestrator) { o.stops = stops }
}

func (o *Orchestrator) StopTask(ctx context.Context, taskID string, actor StopActor, reason string) (*StopResult, error) {
	return o.stop(ctx, repository.StopTarget{TaskID: taskID}, actor, reason)
}

func (o *Orchestrator) StopRun(ctx context.Context, runID string, actor StopActor, reason string) (*StopResult, error) {
	return o.stop(ctx, repository.StopTarget{RunID: runID}, actor, reason)
}

func (o *Orchestrator) StopExecution(ctx context.Context, executionID string, actor StopActor, reason string) (*StopResult, error) {
	return o.stop(ctx, repository.StopTarget{ExecutionID: executionID}, actor, reason)
}

func (o *Orchestrator) stop(ctx context.Context, target repository.StopTarget, actor StopActor, reason string) (*StopResult, error) {
	if o.stops == nil {
		return nil, ErrStopUnavailable
	}
	decision, err := o.stops.Stop(ctx, target, actor.UserID, reason, maxFinalizationAttempts)
	if err != nil {
		return nil, err
	}
	stop := decision.Stop
	result := &StopResult{
		StopID: stop.StopID, Scope: stop.Scope, TaskID: stop.TaskID, RunID: stop.RunID, ExecutionID: stop.ExecutionID,
		TaskStatus: decision.Task.Status, DeliveryStatus: stop.DeliveryStatus, ExitConfirmed: stop.ConfirmationKind == model.StopConfirmationExecutionExited, ConfirmationKind: stop.ConfirmationKind,
		RequestedBy: stop.RequestedBy, RequestedAt: stop.RequestedAt, ReasonCode: stop.ReasonCode, Reason: stop.Reason, ConfirmedAt: stop.ConfirmedAt,
	}
	if decision.Run != nil {
		result.RunStatus = decision.Run.Status
	}
	if decision.Execution != nil {
		result.ExecutionStatus = decision.Execution.Status
	}
	o.WakeStopReconciliation()
	return result, nil
}
