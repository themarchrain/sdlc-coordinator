package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sdlc-coordinator/internal/model"
)

func (r *StopRepository) ListPending(ctx context.Context, limit int) ([]model.StopRequest, error) {
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	query := r.db.WithContext(ctx).Where("delivery_status <> ? OR COALESCE(confirmation_kind, '') = ''", model.StopDeliveryConfirmed)
	if r.db.Dialector.Name() == "mysql" {
		query = query.Where("next_attempt_at IS NULL OR next_attempt_at <= NOW(6)")
	} else {
		query = query.Where("next_attempt_at IS NULL OR next_attempt_at <= ?", time.Now().UTC())
	}
	var rows []model.StopRequest
	err := query.Order("next_attempt_at ASC").Order("id ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (r *StopRepository) GetForTask(ctx context.Context, taskID string) (*model.StopRequest, error) {
	var stop model.StopRequest
	if err := r.db.WithContext(ctx).Where("task_id = ?", taskID).Order("id DESC").First(&stop).Error; err != nil {
		return nil, stopLookupError(err)
	}
	return &stop, nil
}

func (r *StopRepository) withDeliveryRight(ctx context.Context, id string, lease model.WorkItemLease, apply func(*gorm.DB, *model.StopRequest, time.Time) error) error {
	var target model.StopRequest
	if err := r.db.WithContext(ctx).Where("stop_id = ?", id).First(&target).Error; err != nil {
		return stopLookupError(err)
	}
	return withLockedTask(ctx, r.db, target.TaskID, func(tx *gorm.DB, _ *model.Task, now time.Time) error {
		if lease.Scope != "stop:"+id {
			return ErrActionLeaseLost
		}
		query := tx.Where("scope = ?", lease.Scope)
		if tx.Dialector.Name() == "mysql" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var current model.WorkItemLease
		if err := query.First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrActionLeaseLost
			}
			return err
		}
		if current.Owner != lease.Owner || current.Generation != lease.Generation || !current.LeaseUntil.After(now) {
			return ErrActionLeaseLost
		}
		if err := tx.Where("stop_id = ?", id).First(&target).Error; err != nil {
			return err
		}
		return apply(tx, &target, now)
	})
}

func (r *StopRepository) BeginDelivery(ctx context.Context, id string, lease model.WorkItemLease) (*model.StopRequest, error) {
	var result *model.StopRequest
	err := r.withDeliveryRight(ctx, id, lease, func(tx *gorm.DB, stop *model.StopRequest, now time.Time) error {
		if stop.ConfirmationKind != "" && stop.DeliveryStatus == model.StopDeliveryConfirmed {
			return ErrActionAlreadyAdmitted
		}
		if stop.NextAttemptAt != nil && stop.NextAttemptAt.After(now) {
			return ErrActionAlreadyAdmitted
		}
		if stop.ExecutionID == "" {
			var task model.Task
			if err := tx.Where("task_id = ?", stop.TaskID).First(&task).Error; err != nil {
				return err
			}
			run, execution, err := currentStopTarget(tx, &task)
			if err != nil {
				return err
			}
			if err := initializeStopDelivery(tx, run, execution, stop, now); err != nil {
				return err
			}
		} else {
			stop.DeliveryStatus, stop.ConfirmedAt = model.StopDeliveryDelivering, nil
		}
		stop.AttemptCount++
		stop.LastAttemptAt = &now
		next := now.Add(10 * time.Second)
		stop.NextAttemptAt = &next
		if stop.DeliveryStatus == model.StopDeliveryConfirmed {
			stop.NextAttemptAt = nil
		}
		if err := tx.Save(stop).Error; err != nil {
			return err
		}
		result = stop
		return nil
	})
	return result, err
}

func (r *StopRepository) FinishDelivery(ctx context.Context, id string, lease model.WorkItemLease, kind model.StopConfirmationKind, execution *model.AgentExecution, lastError string) error {
	return r.withDeliveryRight(ctx, id, lease, func(tx *gorm.DB, stop *model.StopRequest, now time.Time) error {
		if stop.DeliveryStatus == model.StopDeliveryConfirmed && stop.ConfirmationKind != "" {
			return ErrActionAlreadyAdmitted
		}
		if kind != "" && kind != model.StopConfirmationCreateBlocked && kind != model.StopConfirmationExecutionExited {
			return ErrInvalidStopRequest
		}
		updates := map[string]any{"delivery_status": model.StopDeliveryWaitingExit, "last_error": lastError, "updated_at": now, "next_attempt_at": now.Add(2 * time.Second), "confirmation_kind": "", "confirmed_at": nil}
		if kind != "" {
			updates["delivery_status"] = model.StopDeliveryConfirmed
			updates["confirmation_kind"] = kind
			updates["confirmed_at"] = now
			updates["next_attempt_at"] = nil
		}
		if execution != nil {
			if execution.ExecutionID != stop.ExecutionID || execution.RunID != stop.RunID {
				return ErrInvalidStopRequest
			}
			values := map[string]any{"status": execution.Status, "result_hash": execution.ResultHash, "error_code": execution.ErrorCode, "error_message": execution.ErrorMessage, "started_at": execution.StartedAt, "finished_at": execution.FinishedAt, "last_observed_at": now, "updated_at": now}
			if err := tx.Model(&model.AgentExecution{}).Where("execution_id = ? AND run_id = ?", stop.ExecutionID, stop.RunID).Where("status NOT IN ? OR status = ?", terminalExecutionStatuses(), execution.Status).Updates(values).Error; err != nil {
				return err
			}
		}
		return tx.Model(stop).Updates(updates).Error
	})
}

func initializeStopDelivery(tx *gorm.DB, run *model.Run, execution *model.AgentExecution, stop *model.StopRequest, now time.Time) error {
	stop.DeliveryStatus, stop.ConfirmationKind, stop.ConfirmedAt, stop.NextAttemptAt = model.StopDeliveryConfirmed, model.StopConfirmationNoRemoteTarget, &now, nil
	if execution != nil {
		stop.ExecutionID, stop.RunnerEndpoint = execution.ExecutionID, execution.RunnerEndpoint
		if stop.RunnerEndpoint == "" && run != nil {
			stop.RunnerEndpoint = run.RunnerEndpoint
		}
	} else if run != nil {
		var admitted int64
		if err := tx.Model(&model.ActionAdmission{}).Where("task_id = ? AND run_id = ? AND kind = ? AND not_sent_at IS NULL", stop.TaskID, run.RunID, model.ActionExecutionCreate).Count(&admitted).Error; err != nil {
			return err
		}
		if admitted > 0 || run.ExternalSystem == "agent-runner" {
			stop.ExecutionID, stop.RunnerEndpoint = run.RunID, run.RunnerEndpoint
			if run.ExternalID != "" {
				stop.ExecutionID = run.ExternalID
			}
		}
	}
	if stop.ExecutionID != "" {
		stop.DeliveryStatus, stop.ConfirmationKind, stop.ConfirmedAt, stop.NextAttemptAt = model.StopDeliveryPending, "", nil, &now
	}
	return nil
}
