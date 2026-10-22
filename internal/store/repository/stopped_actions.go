package repository

import (
	"context"
	"time"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

func (r *EffectRepository) RetryStoppedAware(ctx context.Context, key string) (*model.Effect, error) {
	effect, err := r.GetByDedupeKey(ctx, key)
	if err != nil {
		return nil, err
	}
	err = withLockedTask(ctx, r.db, effect.TaskID, func(tx *gorm.DB, task *model.Task, _ time.Time) error {
		if task.Status == model.TaskStatusInterrupted {
			return ErrTaskInterrupted
		}
		var count int64
		if err := tx.Model(&model.ActionAdmission{}).Where("kind = ? AND action_key = ? AND not_sent_at IS NULL", model.ActionIMMessage, effect.EffectID).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrActionOutcomeUnknown
		}
		result := tx.Model(&model.Effect{}).Where("effect_id = ? AND status = ?", effect.EffectID, model.EffectStatusFailed).
			Updates(map[string]any{"status": model.EffectStatusPending, "error_message": "", "response_json": ""})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrResultNotClaimable
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.GetByDedupeKey(ctx, key)
}

func (r *WritebackRepository) UpdateReconciledStoppedAware(ctx context.Context, writeback *model.Writeback, expected model.WritebackStatus) (bool, error) {
	var applied bool
	err := withLockedTask(ctx, r.db, writeback.TaskID, func(tx *gorm.DB, task *model.Task, _ time.Time) error {
		if expected == model.WritebackStatusUnknown && writeback.Status == model.WritebackStatusPending {
			writeback.Status, writeback.ErrorMessage = model.WritebackStatusUnknown, "remote absence observed; automatic resend disabled"
		} else if task.Status == model.TaskStatusInterrupted && writeback.Status == model.WritebackStatusPending {
			writeback.Status, writeback.ErrorMessage = model.WritebackStatusSkipped, "task_interrupted"
		}

		var err error
		applied, err = NewWritebackRepository(tx).UpdateGuardedIfStatus(ctx, writeback, expected)
		return err
	})
	return applied, err
}

func (r *EffectRepository) RecoverStaleDelivery(ctx context.Context, key string, attempt int, window time.Duration, message string) (model.EffectStatus, bool, error) {
	effect, err := r.GetByDedupeKey(ctx, key)
	if err != nil {
		return "", false, err
	}
	status, applied := model.EffectStatusDeliveryUnverified, false
	err = withLockedTask(ctx, r.db, effect.TaskID, func(tx *gorm.DB, task *model.Task, now time.Time) error {
		var total, unknown int64
		admissions := tx.Model(&model.ActionAdmission{}).Where("kind = ? AND action_key = ?", model.ActionIMMessage, effect.EffectID)
		if err := admissions.Count(&total).Error; err != nil {
			return err
		}
		if err := admissions.Where("not_sent_at IS NULL").Count(&unknown).Error; err != nil {
			return err
		}
		if total != 0 && unknown == 0 {
			status = model.EffectStatusPending
			if task.Status == model.TaskStatusInterrupted {
				status, message = model.EffectStatusSkipped, "task_interrupted"
			}
		}
		query := tx.Model(&model.Effect{}).Where("effect_id = ? AND status = ? AND attempt_count = ?", effect.EffectID, model.EffectStatusRunning, attempt)
		if tx.Dialector.Name() == "mysql" {
			query = query.Where("updated_at <= DATE_SUB(NOW(6), INTERVAL ? MICROSECOND)", window.Microseconds())
		} else {
			if effect.UpdatedAt.After(now.Add(-window)) {
				return nil
			}
			query = query.Where("updated_at = ?", effect.UpdatedAt)
		}
		result := query.Updates(map[string]any{"status": status, "error_message": message, "updated_at": now})
		applied = result.RowsAffected == 1
		return result.Error
	})
	return status, applied, err
}
