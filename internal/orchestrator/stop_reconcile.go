package orchestrator

import (
	"context"
	"errors"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store/repository"
)

type StopRunnerClient interface {
	CancelDurable(context.Context, string, runner.CancellationRequest) (*runner.CancellationAck, error)
}

func WithStopRunnerResolver(resolver func(string) (StopRunnerClient, error)) Option {
	return func(o *Orchestrator) { o.stopRunnerResolver = resolver }
}

type stopDeliveryRepository interface {
	ListPending(context.Context, int) ([]model.StopRequest, error)
	BeginDelivery(context.Context, string, model.WorkItemLease) (*model.StopRequest, error)
	FinishDelivery(context.Context, string, model.WorkItemLease, model.StopConfirmationKind, *model.AgentExecution, string) error
}

func (o *Orchestrator) ReconcileStops(ctx context.Context, limit int) error {
	if o.stops == nil {
		return nil
	}
	repo, ok := o.stops.(stopDeliveryRepository)
	if !ok || o.workItemLeases == nil {
		return ErrStopUnavailable
	}
	rows, err := repo.ListPending(ctx, limit)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := o.reconcileStop(ctx, repo, row); err != nil && !errors.Is(err, repository.ErrActionLeaseLost) && !errors.Is(err, repository.ErrActionAlreadyAdmitted) {
			return err
		}
	}
	return nil
}

func (o *Orchestrator) reconcileStop(ctx context.Context, repo stopDeliveryRepository, row model.StopRequest) error {
	lease, won, err := o.acquireWorkItemLease(ctx, "stop:"+row.StopID, "stop-delivery", 15*time.Second)
	if err != nil || !won {
		return err
	}
	defer o.releaseWorkItemLease(lease)
	stop, err := repo.BeginDelivery(ctx, row.StopID, lease)
	if err != nil {
		return err
	}
	if stop.DeliveryStatus == model.StopDeliveryConfirmed {
		return nil
	}
	var ack *runner.CancellationAck
	if o.stopRunnerResolver == nil || stop.RunnerEndpoint == "" {
		err = ErrStopUnavailable
	} else {
		var client StopRunnerClient
		client, err = o.stopRunnerResolver(stop.RunnerEndpoint)
		if err == nil && client == nil {
			err = ErrStopUnavailable
		}
		if err == nil {
			requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			ack, err = client.CancelDurable(requestCtx, stop.ExecutionID, runner.CancellationRequest{StopID: stop.StopID, TaskID: stop.TaskID})
			cancel()
		}
	}
	kind, projection := model.StopConfirmationKind(""), (*model.AgentExecution)(nil)
	lastError := "runner_cancel_unconfirmed"
	if err == nil {
		kind, projection, err = o.validateStopAck(ctx, stop, ack)
		if err == nil {
			lastError = ""
		}
	}
	return repo.FinishDelivery(ctx, stop.StopID, lease, kind, projection, lastError)
}

func (o *Orchestrator) validateStopAck(ctx context.Context, stop *model.StopRequest, ack *runner.CancellationAck) (model.StopConfirmationKind, *model.AgentExecution, error) {
	invalid := func() (model.StopConfirmationKind, *model.AgentExecution, error) {
		return "", nil, errors.New("runner cancellation evidence does not match stop")
	}
	if ack == nil || ack.StopID != stop.StopID || ack.TaskID != stop.TaskID || ack.ExecutionID != stop.ExecutionID || !ack.CancelIntentPersisted || !ack.FutureCreateBlocked {
		return invalid()
	}
	if !ack.ExecutionPresent {
		if ack.Execution != nil || ack.ExitConfirmed {
			return invalid()
		}
		return model.StopConfirmationCreateBlocked, nil, nil
	}
	execution := ack.Execution
	if execution == nil || execution.ExecutionID != stop.ExecutionID || execution.TaskID != stop.TaskID || execution.RequestHash == "" {
		return invalid()
	}
	if o.agentExecutions == nil {
		return invalid()
	}
	stored, err := o.agentExecutions.GetByRunID(ctx, stop.RunID)
	if err != nil || stored.ExecutionID != stop.ExecutionID || stored.RequestHash != execution.RequestHash || (stored.RunnerEndpoint != "" && stored.RunnerEndpoint != stop.RunnerEndpoint) {
		return invalid()
	}
	terminal := execution.Status == runner.StatusSucceeded || execution.Status == runner.StatusFailed || execution.Status == runner.StatusCancelled || execution.Status == runner.StatusInterrupted
	if ack.ExitConfirmed && (!terminal || execution.FinishedAt == nil) {
		return invalid()
	}
	if !terminal && execution.Status != runner.StatusAccepted && execution.Status != runner.StatusStarting && execution.Status != runner.StatusRunning {
		return invalid()
	}
	projection := &model.AgentExecution{RunID: stop.RunID, ExecutionID: stop.ExecutionID, Status: model.AgentExecutionStatus(execution.Status), ResultHash: execution.ResultHash, ErrorCode: execution.ErrorCode, ErrorMessage: execution.ErrorMessage, StartedAt: execution.StartedAt, FinishedAt: execution.FinishedAt}
	if ack.ExitConfirmed {
		return model.StopConfirmationExecutionExited, projection, nil
	}
	return "", projection, nil
}

func (o *Orchestrator) WakeStopReconciliation() {
	if o.stopRunnerResolver == nil || !o.stopWakeMu.TryLock() {
		return
	}
	go func() {
		defer o.stopWakeMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		for attempt := 0; attempt < 3; attempt++ {
			if err := o.ReconcileStops(ctx, 20); err != nil {
				return
			}
			if attempt == 2 {
				return
			}
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
}
