package repository

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type StopTarget struct{ TaskID, RunID, ExecutionID string }

type StopDecision struct {
	Stop      *model.StopRequest
	Task      *model.Task
	Run       *model.Run
	Execution *model.AgentExecution
}

type StopRepository struct{ db *gorm.DB }

func NewStopRepository(db *gorm.DB) *StopRepository { return &StopRepository{db: db} }

func (r *StopRepository) Stop(ctx context.Context, target StopTarget, requestedBy, reason string, maxSubmissionAttempts int) (*StopDecision, error) {
	if strings.TrimSpace(requestedBy) == "" || !utf8.ValidString(reason) || utf8.RuneCountInString(reason) > 500 || maxSubmissionAttempts <= 0 {
		return nil, ErrInvalidStopRequest
	}
	taskID, targetRunID, err := r.resolveTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	var decision *StopDecision
	err = withLockedTask(ctx, r.db, taskID, func(tx *gorm.DB, task *model.Task, now time.Time) error {
		if targetRunID != "" && (task.CurrentRunID == nil || *task.CurrentRunID != targetRunID) {
			return ErrStopNotCurrentRun
		}
		run, execution, err := currentStopTarget(tx, task)
		if err != nil {
			return err
		}
		if target.ExecutionID != "" && (execution == nil || execution.ExecutionID != target.ExecutionID) {
			return ErrStopNotCurrentRun
		}
		scope, key := model.StopScopeTask, "task:"+task.TaskID
		if task.Status == model.TaskStatusSucceeded {
			if target.ExecutionID == "" || run == nil || run.Status != model.RunStatusSucceeded {
				return ErrStopAlreadyTerminal
			}
			scope, key = model.StopScopeExecutionCleanup, "execution_cleanup:"+target.ExecutionID
		}
		var existing model.StopRequest
		lookup := tx.Where("dedupe_key = ?", key).First(&existing).Error
		if lookup == nil {
			decision = &StopDecision{Stop: &existing, Task: task, Run: run, Execution: execution}
			return nil
		}
		if !errors.Is(lookup, gorm.ErrRecordNotFound) {
			return lookup
		}
		if scope == model.StopScopeTask && !taskCanBeStopped(task.Status) {
			return ErrStopAlreadyTerminal
		}
		if scope == model.StopScopeExecutionCleanup && !executionMayBeRunning(execution.Status) {
			return ErrStopAlreadyTerminal
		}
		stop := &model.StopRequest{
			StopID: model.NewID("stop"), DedupeKey: key, Scope: scope, TaskID: task.TaskID,
			RequestedBy: requestedBy, RequestedAt: now, ReasonCode: "manual_stop", Reason: reason,
			DeliveryStatus: model.StopDeliveryConfirmed, ConfirmedAt: &now, CreatedAt: now, UpdatedAt: now,
		}
		if task.CurrentRunID != nil {
			stop.RunID = *task.CurrentRunID
		}
		if err := initializeStopDelivery(tx, run, execution, stop, now); err != nil {
			return err
		}
		if err := tx.Create(stop).Error; err != nil {
			return err
		}
		if scope == model.StopScopeTask {
			if err := tx.Model(task).Updates(map[string]any{"status": model.TaskStatusInterrupted, "lease_until": nil, "updated_at": now}).Error; err != nil {
				return err
			}
			if run != nil {
				if run.Status == model.RunStatusPending || run.Status == model.RunStatusRunning || run.Status == model.RunStatusFinalizing {
					if err := tx.Model(run).Updates(map[string]any{"status": model.RunStatusInterrupted, "completed_at": now, "updated_at": now}).Error; err != nil {
						return err
					}
				}
				if err := tx.Model(&model.ResultSubmission{}).Where("task_id = ? AND run_id = ?", task.TaskID, run.RunID).
					Where("status = ? OR (status = ? AND attempt_count < ?)", model.ResultSubmissionStatusProcessing, model.ResultSubmissionStatusFailed, maxSubmissionAttempts).
					Updates(map[string]any{"status": model.ResultSubmissionStatusInterrupted, "lease_until": nil, "completed_at": now, "updated_at": now}).Error; err != nil {
					return err
				}
			}
		}
		decision = &StopDecision{Stop: stop, Task: task, Run: run, Execution: execution}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return decision, nil
}

func (r *StopRepository) resolveTarget(ctx context.Context, target StopTarget) (string, string, error) {
	count := 0
	for _, id := range []string{target.TaskID, target.RunID, target.ExecutionID} {
		if id != "" {
			count++
		}
	}
	if count != 1 {
		return "", "", ErrInvalidStopRequest
	}
	if target.TaskID != "" {
		return target.TaskID, "", nil
	}
	runID := target.RunID
	if target.ExecutionID != "" {
		var execution model.AgentExecution
		if err := r.db.WithContext(ctx).Where("execution_id = ?", target.ExecutionID).First(&execution).Error; err != nil {
			return "", "", stopLookupError(err)
		}
		runID = execution.RunID
	}
	var run model.Run
	if err := r.db.WithContext(ctx).Where("run_id = ?", runID).First(&run).Error; err != nil {
		return "", "", stopLookupError(err)
	}
	return run.TaskID, runID, nil
}

func currentStopTarget(tx *gorm.DB, task *model.Task) (*model.Run, *model.AgentExecution, error) {
	if task.CurrentRunID == nil || *task.CurrentRunID == "" {
		return nil, nil, nil
	}
	query := tx
	if tx.Dialector.Name() == "mysql" {
		query = tx.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var run model.Run
	if err := query.Where("run_id = ?", *task.CurrentRunID).First(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	if run.TaskID != task.TaskID {
		return nil, nil, ErrStopNotCurrentRun
	}
	var execution model.AgentExecution
	executionQuery := tx.Where("run_id = ?", run.RunID)
	if tx.Dialector.Name() == "mysql" {
		executionQuery = executionQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := executionQuery.First(&execution).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &run, nil, nil
		}
		return nil, nil, err
	}
	return &run, &execution, nil
}

func stopLookupError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

func taskCanBeStopped(status model.TaskStatus) bool {
	switch status {
	case model.TaskStatusPending, model.TaskStatusClaimed, model.TaskStatusRunning, model.TaskStatusFinalizing,
		model.TaskStatusRetryPending, model.TaskStatusWaitingUpstream, model.TaskStatusWaitingMerge, model.TaskStatusHumanRequired, model.TaskStatusBlocked:
		return true
	default:
		return false
	}
}

func executionMayBeRunning(status model.AgentExecutionStatus) bool {
	switch status {
	case model.AgentExecutionStatusDispatching, model.AgentExecutionStatusAccepted, model.AgentExecutionStatusStarting, model.AgentExecutionStatusRunning:
		return true
	default:
		return false
	}
}
