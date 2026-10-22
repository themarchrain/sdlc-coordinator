package orchestrator

import (
	"context"
	"errors"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/workspace"
	"sdlc-coordinator/internal/workspaceproto"
)

func (c *ResultChannelCoordinator) admitGitAction(ctx context.Context, taskID, runID string, kind model.ActionKind, key, owner string, generation uint64, hash string) error {
	if c.admissions == nil {
		return nil
	}
	allocator, ok := c.admissions.(interface {
		NextAttempt(context.Context, model.ActionKind, string) (int, error)
	})
	if !ok {
		return ErrStopUnavailable
	}
	attempt, err := allocator.NextAttempt(ctx, kind, key)
	if err != nil {
		return err
	}
	if attempt > 1 {
		reader, ok := c.finalizer.(interface {
			ReadDeliveryOperation(context.Context, string, string) (*workspaceproto.OperationRecord, error)
		})
		if !ok {
			return repository.ErrActionOutcomeUnknown
		}
		record, readErr := reader.ReadDeliveryOperation(ctx, runID, key)
		var httpErr *runner.HTTPError
		if readErr != nil && !(errors.As(readErr, &httpErr) && httpErr.StatusCode == 404) {
			return repository.ErrActionOutcomeUnknown
		}
		if readErr == nil {
			expectedKind := workspaceproto.KindDeveloperCommit
			if kind == model.ActionGitPush {
				expectedKind = workspaceproto.KindDeveloperPush
			}
			if record == nil || record.OperationID != key || record.TaskID != taskID || record.RunID != runID || record.Kind != expectedKind {
				return repository.ErrActionOutcomeUnknown
			}
		}
	}
	_, err = c.admissions.AdmitAction(ctx, ActionAdmissionRequest{TaskID: taskID, RunID: runID, Kind: kind, ActionKey: key, Attempt: attempt, LeaseOwner: owner, LeaseGeneration: generation, RequestHash: hash, IdempotentReplay: attempt > 1})
	return err
}

func (c *ResultChannelCoordinator) commitAdmitted(ctx context.Context, request workspace.DeveloperCommitRequest) (*workspace.DeveloperCommitResult, error) {
	normalized := request
	normalized.DeliveryOwner, normalized.DeliveryGeneration = "", 0
	hash, err := workspaceproto.PayloadHash(normalized)
	if err != nil {
		return nil, err
	}
	if err := c.admitGitAction(ctx, request.TaskID, request.RunID, model.ActionGitCommit, "commit-"+request.RunID, request.DeliveryOwner, request.DeliveryGeneration, hash); err != nil {
		return nil, err
	}
	return c.finalizer.CommitDelivery(ctx, request)
}

func (c *ResultChannelCoordinator) pushAdmitted(ctx context.Context, request workspace.DeveloperPushRequest) (*workspace.DeveloperDelivery, error) {
	normalized := request
	normalized.DeliveryOwner, normalized.DeliveryGeneration, normalized.Credential = "", 0, nil
	hash, err := workspaceproto.PayloadHash(normalized)
	if err != nil {
		return nil, err
	}
	if err := c.admitGitAction(ctx, request.TaskID, request.RunID, model.ActionGitPush, "push-"+request.RunID, request.DeliveryOwner, request.DeliveryGeneration, hash); err != nil {
		return nil, err
	}
	return c.finalizer.PushDelivery(ctx, request)
}
