package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
)

var (
	ErrRecoveryUnavailable         = errors.New("task recovery unavailable")
	ErrRecoveryInvalid             = errors.New("invalid task recovery request")
	ErrRecoveryRequiresAuditedPath = errors.New("blocked task recovery requires audited path")
)

type TaskRecoveryRepository interface {
	GetByRequest(ctx context.Context, taskID, requestID string) (*model.TaskRecovery, error)
	Accept(ctx context.Context, taskID, actorID string, command repository.RecoveryCommand, proof repository.RecoveryDBProof) (*model.TaskRecovery, bool, error)
}

type RecoverTaskRequest struct {
	RequestID              string
	BlockRevision          string
	Reason                 string
	AcknowledgeUnknownRisk bool
}

type RecoverTaskResponse struct {
	Recovery *model.TaskRecovery
	Task     *model.Task
	Replayed bool
}

func AssertRunRecoveryCurrent(task *model.Task, run *model.Run) error {
	if task == nil || run == nil {
		return repository.ErrResultNotAcceptable
	}
	if task.RecoveryGeneration != run.RecoveryGeneration {
		return repository.ErrRecoveryGenerationMismatch
	}
	if task.CurrentRunID == nil || *task.CurrentRunID != run.RunID {
		return repository.ErrResultNotAcceptable
	}
	return nil
}

func (o *Orchestrator) RecoverBlockedTask(ctx context.Context, taskID, actorID string, request RecoverTaskRequest) (*RecoverTaskResponse, error) {
	if o.recoveries == nil {
		return nil, ErrRecoveryUnavailable
	}
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(actorID) == "" || strings.TrimSpace(request.RequestID) == "" || strings.TrimSpace(request.BlockRevision) == "" {
		return nil, ErrRecoveryInvalid
	}
	if existing, err := o.recoveries.GetByRequest(ctx, taskID, request.RequestID); err == nil {
		if existing.RequestedBy == actorID && existing.BlockRevision == request.BlockRevision && existing.Reason == request.Reason && existing.RiskAcknowledged == request.AcknowledgeUnknownRisk {
			task, taskErr := o.tasks.GetByTaskID(ctx, taskID)
			if taskErr != nil {
				return nil, taskErr
			}
			return &RecoverTaskResponse{Recovery: existing, Task: task, Replayed: true}, nil
		}
		return nil, repository.ErrRecoveryRequestConflict
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}

	assessment, err := AssessTaskRecovery(ctx, o.recoveryReader, taskID)
	if err != nil {
		return nil, err
	}
	if !assessment.Eligible {
		return nil, fmt.Errorf("%w: %s", repository.ErrRecoveryNotEligible, assessment.ReasonCode)
	}
	if assessment.RequiresRiskConfirmation {
		if !request.AcknowledgeUnknownRisk || strings.TrimSpace(request.Reason) == "" {
			return nil, repository.ErrRecoveryRiskRequired
		}
	}
	if assessment.BlockRevision != request.BlockRevision {
		return nil, repository.ErrRecoveryRevisionChanged
	}
	task, err := o.tasks.GetByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	cycleMax := o.policy.normalized().MaxAttempts
	command := repository.RecoveryCommand{
		RequestID:           request.RequestID,
		RequestHash:         recoveryCommandHash(taskID, actorID, request, assessment, cycleMax),
		BlockRevision:       assessment.BlockRevision,
		Reason:              request.Reason,
		RiskAcknowledged:    request.AcknowledgeUnknownRisk,
		ExpectedGeneration:  task.RecoveryGeneration,
		CycleMaxAttempts:    cycleMax,
		FailureSnapshotJSON: recoveryFailureSnapshot(assessment),
		Classification:      string(assessment.Classification),
	}
	proof := repository.RecoveryDBProof{
		TaskID:             task.TaskID,
		TaskVersion:        repository.RecoveryTaskVersion(task),
		CurrentRunID:       taskCurrentRunID(task),
		RecoveryGeneration: task.RecoveryGeneration,
		AttemptCount:       task.AttemptCount,
		SafetyDigest:       assessment.SafetyDigest,
	}
	recovery, replayed, err := o.recoveries.Accept(ctx, taskID, actorID, command, proof)
	if err != nil {
		return nil, err
	}
	current, err := o.tasks.GetByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return &RecoverTaskResponse{Recovery: recovery, Task: current, Replayed: replayed}, nil
}

func recoveryCommandHash(taskID, actorID string, request RecoverTaskRequest, assessment RecoveryAssessment, cycleMax int) string {
	payload := struct {
		TaskID         string `json:"task_id"`
		ActorID        string `json:"actor_id"`
		RequestID      string `json:"request_id"`
		BlockRevision  string `json:"block_revision"`
		Reason         string `json:"reason"`
		RiskAck        bool   `json:"risk_ack"`
		Generation     uint64 `json:"generation"`
		CycleMax       int    `json:"cycle_max"`
		Classification string `json:"classification"`
		FailureStage   string `json:"failure_stage"`
		FailureCode    string `json:"failure_code"`
		FailureSummary string `json:"failure_summary"`
	}{
		TaskID: taskID, ActorID: actorID, RequestID: request.RequestID, BlockRevision: request.BlockRevision,
		Reason: request.Reason, RiskAck: request.AcknowledgeUnknownRisk,
		Generation: assessmentGeneration(assessment), CycleMax: cycleMax, Classification: string(assessment.Classification),
		FailureStage: assessment.FailureStage, FailureCode: assessment.FailureCode, FailureSummary: assessment.FailureSummary,
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func recoveryFailureSnapshot(assessment RecoveryAssessment) string {
	payload := struct {
		Classification string `json:"classification"`
		Stage          string `json:"stage,omitempty"`
		Code           string `json:"code,omitempty"`
		Summary        string `json:"summary,omitempty"`
	}{Classification: string(assessment.Classification), Stage: assessment.FailureStage, Code: assessment.FailureCode, Summary: assessment.FailureSummary}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func assessmentGeneration(assessment RecoveryAssessment) uint64 {
	return assessment.RecoveryGeneration
}

func taskCurrentRunID(task *model.Task) string {
	if task == nil || task.CurrentRunID == nil {
		return ""
	}
	return strings.TrimSpace(*task.CurrentRunID)
}
