package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"sdlc-coordinator/internal/model"
)

func (r *StopRepository) CanStop(ctx context.Context, target StopTarget) (bool, error) {
	taskID, runID, err := r.resolveTarget(ctx, target)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	task, err := NewTaskRepository(r.db).GetByTaskID(ctx, taskID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if runID != "" && (task.CurrentRunID == nil || *task.CurrentRunID != runID) {
		return false, nil
	}
	if task.Status == model.TaskStatusSucceeded && target.ExecutionID != "" {
		var execution model.AgentExecution
		if err := r.db.WithContext(ctx).Where("execution_id = ?", target.ExecutionID).First(&execution).Error; err != nil {
			return false, err
		}
		run, err := NewRunRepository(r.db).GetByRunID(ctx, runID)
		if err != nil {
			return false, err
		}
		return run.Status == model.RunStatusSucceeded && executionMayBeRunning(execution.Status), nil
	}
	return taskCanBeStopped(task.Status), nil
}

func (r *StopRepository) ReadStatus(ctx context.Context, target StopTarget) (*model.StopStatus, error) {
	query := r.db.WithContext(ctx)
	switch {
	case target.TaskID != "":
		query = query.Where("task_id = ?", target.TaskID)
	case target.RunID != "":
		query = query.Where("run_id = ?", target.RunID)
	case target.ExecutionID != "":
		query = query.Where("execution_id = ?", target.ExecutionID)
	default:
		return nil, ErrInvalidStopRequest
	}
	var stop model.StopRequest
	if err := query.Order("id DESC").First(&stop).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	result := &model.StopStatus{StopID: stop.StopID, Scope: stop.Scope, TaskID: stop.TaskID, RunID: stop.RunID, ExecutionID: stop.ExecutionID, DeliveryStatus: stop.DeliveryStatus, ConfirmationKind: stop.ConfirmationKind, ExitConfirmed: stop.ConfirmationKind == model.StopConfirmationExecutionExited, RequestedBy: stop.RequestedBy, RequestedAt: stop.RequestedAt, ReasonCode: stop.ReasonCode, Reason: stop.Reason, ConfirmedAt: stop.ConfirmedAt}
	var task model.Task
	if err := r.db.WithContext(ctx).Where("task_id = ?", stop.TaskID).First(&task).Error; err != nil {
		return nil, err
	}
	result.TaskStatus = task.Status
	if stop.RunID != "" {
		var run model.Run
		if err := r.db.WithContext(ctx).Where("run_id = ?", stop.RunID).First(&run).Error; err != nil {
			return nil, err
		}
		result.RunStatus = run.Status
	}
	if stop.ExecutionID != "" {
		var execution model.AgentExecution
		err := r.db.WithContext(ctx).Where("execution_id = ?", stop.ExecutionID).First(&execution).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		result.ExecutionStatus = execution.Status
	}
	if err := r.db.WithContext(ctx).Model(&model.Writeback{}).Where("task_id = ? AND status = ?", stop.TaskID, model.WritebackStatusUnknown).Count(&result.UnknownWritebacks).Error; err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Model(&model.Effect{}).Where("task_id = ? AND status = ?", stop.TaskID, model.EffectStatusDeliveryUnverified).Count(&result.UnknownMessages).Error; err != nil {
		return nil, err
	}
	return result, nil
}
