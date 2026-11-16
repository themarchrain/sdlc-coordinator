package repository

import (
	"context"
	"errors"
	"time"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func withLockedTask(ctx context.Context, db *gorm.DB, taskID string, apply func(*gorm.DB, *model.Task, time.Time) error) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tx.Dialector.Name() == "sqlite" {
			if err := tx.Exec("UPDATE tasks SET id = id WHERE task_id = ?", taskID).Error; err != nil {
				return err
			}
		}
		query := tx.Where("task_id = ?", taskID)
		if tx.Dialector.Name() == "mysql" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var task model.Task
		if err := query.First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		now := time.Now().UTC()
		if tx.Dialector.Name() == "mysql" {
			if err := tx.Raw("SELECT NOW(6)").Scan(&now).Error; err != nil {
				return err
			}
		}
		return apply(tx, &task, now)
	})
}

// RecoveryBudgetExhaustedReason is the operator-facing reason stored on a task
// the recovery cycle budget stopped.
const RecoveryBudgetExhaustedReason = "本轮恢复的尝试额度已用尽，任务已停靠：开启新的恢复轮次后才能再次运行"

func (r *TaskRepository) StartRunAtomic(ctx context.Context, taskID string, run *model.Run, leaseUntil, startedAt time.Time) (*model.Task, error) {
	if run == nil || run.RunID == "" {
		return nil, ErrTaskNotClaimable
	}
	var current *model.Task
	created := *run
	budgetExhausted := false
	err := withLockedTask(ctx, r.db, taskID, func(tx *gorm.DB, task *model.Task, now time.Time) error {
		if task.Status != model.TaskStatusPending && task.Status != model.TaskStatusRetryPending {
			return ErrTaskNotClaimable
		}
		// 恢复轮开始新 Run 也要消耗本轮额度：额度为 0 时不再启动，避免
		// 已耗次数超过本轮上限。代次 0 沿用旧策略。
		if task.RecoveryGeneration > 0 && task.RetryCycleMaxAttempts > 0 {
			if task.AttemptCount-task.RetryCycleBaseCount >= task.RetryCycleMaxAttempts {
				// 额度耗尽不是竞态：留在 pending 会被下一次 Tick 反复拾取并
				// 再次拒绝，任务既不运行也到不了 blocked。这里直接停靠。
				if err := tx.Model(task).Updates(map[string]any{
					"status":     model.TaskStatusBlocked,
					"last_error": RecoveryBudgetExhaustedReason,
					"updated_at": now,
				}).Error; err != nil {
					return err
				}
				// 停靠写必须提交：跟着错误回滚掉等于什么都没做。
				budgetExhausted = true
				return nil
			}
		}
		created.TaskID = taskID
		created.Status = model.RunStatusRunning
		created.RecoveryGeneration = task.RecoveryGeneration
		created.StartedAt = &startedAt
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		if err := tx.Model(task).Updates(map[string]any{
			"status": model.TaskStatusRunning, "current_run_id": created.RunID,
			"attempt_count": task.AttemptCount + 1, "lease_until": leaseUntil, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		current = task
		return nil
	})
	if err != nil {
		return nil, err
	}
	if budgetExhausted {
		return nil, ErrRecoveryBudgetExhausted
	}
	*run = created
	return current, nil
}

func withLockedCurrentRun(ctx context.Context, db *gorm.DB, runID string, apply func(*gorm.DB, *model.Run) error) error {
	var target model.Run
	if err := db.WithContext(ctx).Select("task_id").Where("run_id = ?", runID).First(&target).Error; err != nil {
		return stopLookupError(err)
	}
	return withLockedTask(ctx, db, target.TaskID, func(tx *gorm.DB, task *model.Task, _ time.Time) error {
		if task.Status == model.TaskStatusInterrupted || task.Status == model.TaskStatusSucceeded || task.CurrentRunID == nil || *task.CurrentRunID != runID {
			return ErrTaskNotClaimable
		}
		query := tx.Where("run_id = ? AND task_id = ?", runID, task.TaskID)
		if tx.Dialector.Name() == "mysql" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var run model.Run
		if err := query.First(&run).Error; err != nil {
			return stopLookupError(err)
		}
		if run.Status == model.RunStatusInterrupted {
			return ErrTaskNotClaimable
		}
		return apply(tx, &run)
	})
}
