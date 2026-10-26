package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/store/repository"
)

var (
	ErrResultConflict   = errors.New("immutable result conflict")
	ErrResultProcessing = errors.New("result is processing")
)

const maxFinalizationAttempts = 3

type SubmitResultRequest struct {
	Source     string
	ToolName   string
	RawPayload []byte
	Canonical  results.CanonicalResult
	// NormalizationTrail records the thin-normalizer repair trail (JSON array
	// of repairs); empty means the payload passed through byte identical.
	NormalizationTrail string
}

type SubmitResultResponse struct {
	Submission *model.ResultSubmission
	Task       *model.Task
	Run        *model.Run
}

type ResultFinalizationResult struct {
	SubmissionID string                       `json:"submission_id"`
	TaskID       string                       `json:"task_id"`
	RunID        string                       `json:"run_id"`
	Status       model.ResultSubmissionStatus `json:"status"`
	Finalized    bool                         `json:"finalized"`
	Error        string                       `json:"error,omitempty"`
}

func (o *Orchestrator) SubmitResult(ctx context.Context, req SubmitResultRequest) (*SubmitResultResponse, error) {
	if o.resultSubmissions == nil {
		return nil, errors.New("result submission repository is required")
	}
	canonicalJSON, hash, err := results.Canonicalize(req.Canonical)
	if err != nil {
		return nil, err
	}
	task, err := o.tasks.GetByTaskID(ctx, req.Canonical.TaskID)
	if err != nil {
		return nil, err
	}
	run, err := o.runs.GetByRunID(ctx, req.Canonical.RunID)
	if err != nil {
		return nil, err
	}
	if run.TaskID != task.TaskID || task.CurrentRunID == nil || *task.CurrentRunID != run.RunID {
		return nil, repository.ErrResultNotAcceptable
	}
	if err := o.validateCanonicalResultForAcceptance(task, run, req.Canonical); err != nil {
		return nil, err
	}

	existing, err := o.resultSubmissions.GetByRunID(ctx, run.RunID)
	if err == nil {
		return o.replaySubmission(ctx, existing, hash)
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	rawPayload := req.RawPayload
	if len(rawPayload) == 0 {
		rawPayload = canonicalJSON
	}
	submission := &model.ResultSubmission{
		TaskID: task.TaskID, RunID: run.RunID, Source: req.Source, ToolName: req.ToolName, Role: run.WorkerRole,
		PayloadHash: hash, RawPayloadJSON: string(rawPayload), CanonicalJSON: string(canonicalJSON),
		NormalizationJSON: req.NormalizationTrail,
	}
	if submission.Source == "" {
		submission.Source = "unknown"
	}
	if err := o.resultSubmissions.Accept(ctx, submission, o.policy.LeaseUntil(o.now())); err != nil {
		if errors.Is(err, repository.ErrResultAlreadyAccepted) {
			existing, lookupErr := o.resultSubmissions.GetByRunID(ctx, run.RunID)
			if lookupErr != nil {
				return nil, lookupErr
			}
			return o.replaySubmission(ctx, existing, hash)
		}
		return nil, err
	}
	return o.finalizeAcceptedSubmission(ctx, submission, req.Canonical)
}

func (o *Orchestrator) validateCanonicalResultForAcceptance(task *model.Task, run *model.Run, canonical results.CanonicalResult) error {
	if err := validateReworkCanonicalResult(task, canonical); err != nil {
		return err
	}
	if err := validateCodeHostWritebacksForTask(task, canonical); err != nil {
		return err
	}
	if _, err := BuildArtifactsFromResult(canonical); err != nil {
		return err
	}
	if _, err := BuildEffectsFromResult(canonical); err != nil {
		return err
	}
	if _, err := BuildCodeHostWritebacksFromResult(canonical); err != nil {
		return err
	}
	for index, next := range canonical.NextTasks {
		if _, _, err := BuildNextTaskExpansion(task, run, next, index); err != nil {
			return err
		}
	}
	return nil
}

func (o *Orchestrator) replaySubmission(ctx context.Context, submission *model.ResultSubmission, hash string) (*SubmitResultResponse, error) {
	if submission.PayloadHash != hash {
		return nil, ErrResultConflict
	}
	if submission.Status == model.ResultSubmissionStatusSucceeded {
		return o.submissionResponse(ctx, submission)
	}
	if submission.Status == model.ResultSubmissionStatusInterrupted {
		return nil, repository.ErrTaskInterrupted
	}
	if !resultSubmissionLeaseExpired(submission, o.now()) {
		return nil, ErrResultProcessing
	}
	claimed, err := o.resultSubmissions.ClaimRecoverable(
		ctx,
		submission.SubmissionID,
		o.now(),
		o.policy.LeaseUntil(o.now()),
		maxFinalizationAttempts,
	)
	if err != nil {
		if errors.Is(err, repository.ErrResultNotClaimable) {
			current, lookupErr := o.resultSubmissions.GetBySubmissionID(ctx, submission.SubmissionID)
			if lookupErr != nil {
				return nil, lookupErr
			}
			if current.Status == model.ResultSubmissionStatusSucceeded {
				return o.submissionResponse(ctx, current)
			}
			return nil, ErrResultProcessing
		}
		return nil, err
	}
	canonical, err := results.DecodeCanonicalStrict([]byte(claimed.CanonicalJSON))
	if err != nil {
		return nil, fmt.Errorf("decode accepted canonical result: %w", err)
	}
	return o.finalizeAcceptedSubmission(ctx, claimed, canonical)
}

func (o *Orchestrator) submissionResponse(ctx context.Context, submission *model.ResultSubmission) (*SubmitResultResponse, error) {
	task, err := o.tasks.GetByTaskID(ctx, submission.TaskID)
	if err != nil {
		return nil, err
	}
	run, err := o.runs.GetByRunID(ctx, submission.RunID)
	if err != nil {
		return nil, err
	}
	return &SubmitResultResponse{Submission: submission, Task: task, Run: run}, nil
}

func (o *Orchestrator) finalizeAcceptedSubmission(ctx context.Context, submission *model.ResultSubmission, canonical results.CanonicalResult) (*SubmitResultResponse, error) {
	task, run, err := o.ApplyCanonicalResult(ctx, canonical)
	if err != nil {
		if errors.Is(err, ErrWritebackRequiresReconciliation) {
			// 写回结果未知、等待对账裁决:不计失败尝试,避免对账完成前
			// submission 重试预算被烧穿;等待由对账升级门终止,有界。
			deferred, deferErr := o.resultSubmissions.DeferReconciliationWait(ctx, submission.SubmissionID, err.Error())
			if deferErr != nil {
				return nil, fmt.Errorf("finalize result: %v; defer submission: %w", err, deferErr)
			}
			*submission = *deferred
			return nil, err
		}
		failed, _, updateErr := o.resultSubmissions.FailFinalization(
			ctx,
			submission.SubmissionID,
			err.Error(),
			o.now(),
			maxFinalizationAttempts,
		)
		if updateErr != nil {
			return nil, fmt.Errorf("finalize result: %v; update submission: %w", err, updateErr)
		}
		*submission = *failed
		return nil, err
	}
	if _, atomic := o.tasks.(resultFinalizer); atomic {
		current, err := o.resultSubmissions.GetBySubmissionID(ctx, submission.SubmissionID)
		if err != nil {
			return nil, err
		}
		*submission = *current
		if current.Status == model.ResultSubmissionStatusSucceeded {
			return o.submissionResponse(ctx, submission)
		}
	}
	completedAt := o.now()
	submission.Status = model.ResultSubmissionStatusSucceeded
	submission.ErrorMessage = ""
	submission.LeaseUntil = nil
	submission.CompletedAt = &completedAt
	if err := o.resultSubmissions.Update(ctx, submission); err != nil {
		return nil, err
	}
	return &SubmitResultResponse{Submission: submission, Task: task, Run: run}, nil
}

func (o *Orchestrator) FinalizeRecoverableSubmissions(ctx context.Context, limit int) ([]ResultFinalizationResult, error) {
	if o.resultSubmissions == nil {
		return nil, errors.New("result submission repository is required")
	}
	now := o.now()
	recoverable, err := o.resultSubmissions.ListRecoverable(ctx, now, maxFinalizationAttempts, limit)
	if err != nil {
		return nil, err
	}
	finalized := make([]ResultFinalizationResult, 0, len(recoverable))
	for i := range recoverable {
		claimed, err := o.resultSubmissions.ClaimRecoverable(
			ctx,
			recoverable[i].SubmissionID,
			now,
			o.policy.LeaseUntil(now),
			maxFinalizationAttempts,
		)
		if errors.Is(err, repository.ErrResultNotClaimable) {
			continue
		}
		if err != nil {
			return nil, err
		}

		result := ResultFinalizationResult{
			SubmissionID: claimed.SubmissionID,
			TaskID:       claimed.TaskID,
			RunID:        claimed.RunID,
			Status:       claimed.Status,
		}
		canonical, decodeErr := results.DecodeCanonicalStrict([]byte(claimed.CanonicalJSON))
		if decodeErr != nil {
			result.Error = fmt.Sprintf("decode accepted canonical result: %v", decodeErr)
			if err := o.markSubmissionFailed(ctx, claimed, result.Error); err != nil {
				return nil, err
			}
			result.Status = claimed.Status
			finalized = append(finalized, result)
			continue
		}
		response, finalizeErr := o.finalizeAcceptedSubmission(ctx, claimed, canonical)
		if finalizeErr != nil {
			result.Status = claimed.Status
			result.Error = finalizeErr.Error()
			finalized = append(finalized, result)
			continue
		}
		result.Status = response.Submission.Status
		result.Finalized = true
		finalized = append(finalized, result)
	}
	return finalized, nil
}

func (o *Orchestrator) markSubmissionFailed(ctx context.Context, submission *model.ResultSubmission, message string) error {
	failed, _, err := o.resultSubmissions.FailFinalization(
		ctx,
		submission.SubmissionID,
		message,
		o.now(),
		maxFinalizationAttempts,
	)
	if err != nil {
		return err
	}
	*submission = *failed
	return nil
}

func resultSubmissionLeaseExpired(submission *model.ResultSubmission, now time.Time) bool {
	return submission != nil && (submission.LeaseUntil == nil || !submission.LeaseUntil.After(now))
}
