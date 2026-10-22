package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	effectsexec "sdlc-coordinator/internal/effects"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
)

type ActionAdmissionRequest = repository.ActionAdmissionRequest

type ActionAdmissionPort interface {
	AdmitAction(context.Context, ActionAdmissionRequest) (*model.ActionAdmission, error)
}

func WithActionAdmissions(admissions ActionAdmissionPort) Option {
	return func(o *Orchestrator) { o.actionAdmissions = admissions }
}

func (o *Orchestrator) admitExternalAction(ctx context.Context, request ActionAdmissionRequest, lease model.WorkItemLease) (*model.ActionAdmission, error) {
	if o.actionAdmissions == nil {
		if o.stops != nil {
			return nil, ErrStopUnavailable
		}
		return nil, nil
	}
	request.LeaseOwner, request.LeaseGeneration = lease.Owner, lease.Generation
	return o.actionAdmissions.AdmitAction(ctx, request)
}

func (o *Orchestrator) confirmActionNotSent(ctx context.Context, admission *model.ActionAdmission) error {
	if admission == nil {
		return nil
	}
	proof, ok := o.actionAdmissions.(interface {
		ConfirmNotSent(context.Context, *model.ActionAdmission) error
	})
	if !ok {
		return ErrStopUnavailable
	}
	return proof.ConfirmNotSent(ctx, admission)
}

func (o *Orchestrator) executeAdmittedEffect(ctx context.Context, effect *model.Effect) (string, error) {
	if o.actionAdmissions == nil {
		if o.stops != nil {
			return "", ErrStopUnavailable
		}
		return o.effectExecutor.ExecuteEffect(ctx, effect)
	}
	lease, won, err := o.acquireWorkItemLease(ctx, "effect:"+effect.EffectID, "effect", 5*time.Minute)
	if err != nil {
		return "", err
	}
	if !won {
		return "", repository.ErrActionLeaseLost
	}
	defer o.releaseWorkItemLease(lease)
	admission, err := o.admitExternalAction(ctx, ActionAdmissionRequest{TaskID: effect.TaskID, RunID: effect.RunID, Kind: model.ActionIMMessage, ActionKey: effect.EffectID, Attempt: effect.AttemptCount}, lease)
	if err != nil {
		return "", err
	}
	response, err := o.effectExecutor.ExecuteEffect(ctx, effect)
	if errors.Is(err, ErrEffectExecutorDisabled) {
		if proofErr := o.confirmActionNotSent(ctx, admission); proofErr != nil {
			return "", proofErr
		}
	} else if err != nil {
		return "", fmt.Errorf("%w: %v", effectsexec.ErrEffectDeliveryUnverified, err)
	}
	return response, err
}

func actionOutcomeUncertain(err error) bool {
	return errors.Is(err, repository.ErrActionAlreadyAdmitted) || errors.Is(err, repository.ErrActionOutcomeUnknown)
}

func (o *Orchestrator) updateReconciledWriteback(ctx context.Context, writeback *model.Writeback, expected model.WritebackStatus) (bool, error) {
	if o.actionAdmissions != nil && writeback.Status == model.WritebackStatusPending {
		guarded, ok := o.writebacks.(interface {
			UpdateReconciledStoppedAware(context.Context, *model.Writeback, model.WritebackStatus) (bool, error)
		})
		if !ok {
			return false, ErrStopUnavailable
		}
		return guarded.UpdateReconciledStoppedAware(ctx, writeback, expected)
	}
	return o.writebacks.UpdateGuardedIfStatus(ctx, writeback, expected)
}
