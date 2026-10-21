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

func (r *TaskRepository) StartRunAtomic(ctx context.Context, taskID string, run *model.Run, leaseUntil, startedAt time.Time) (*model.Task, error) {
	if run == nil || run.RunID == "" {
		return nil, ErrTaskNotClaimable
	}
	var current *model.Task
	created := *run
	err := withLockedTask(ctx, r.db, taskID, func(tx *gorm.DB, task *model.Task, now time.Time) error {
		if task.Status != model.TaskStatusPending && task.Status != model.TaskStatusRetryPending {
			return ErrTaskNotClaimable
		}
		created.TaskID = taskID
		created.Status = model.RunStatusRunning
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
	*run = created
	return current, nil
}
