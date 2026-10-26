package repository

import (
	"context"
	"time"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ResultTaskExpansion struct {
	Task       *model.Task
	Dependency *model.Dependency
}

func (r *TaskRepository) FinalizeResultAtomic(ctx context.Context, task *model.Task, run *model.Run, expansions []ResultTaskExpansion, finalizeSubmission bool) error {
	return withLockedTask(ctx, r.db, task.TaskID, func(tx *gorm.DB, current *model.Task, now time.Time) error {
		if current.Status == model.TaskStatusInterrupted {
			return ErrTaskInterrupted
		}
		if (current.Status != model.TaskStatusRunning && current.Status != model.TaskStatusFinalizing) || (current.CurrentRunID != nil && *current.CurrentRunID != run.RunID) {
			return ErrResultNotAcceptable
		}
		query := tx.Where("run_id = ? AND task_id = ?", run.RunID, task.TaskID)
		if tx.Dialector.Name() == "mysql" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var currentRun model.Run
		if err := query.First(&currentRun).Error; err != nil {
			return stopLookupError(err)
		}
		if currentRun.Status != model.RunStatusRunning && currentRun.Status != model.RunStatusFinalizing {
			return ErrResultNotAcceptable
		}
		if err := tx.Model(current).Updates(map[string]any{"status": task.Status, "current_run_id": task.CurrentRunID, "lease_until": task.LeaseUntil, "last_error": task.LastError, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&currentRun).Updates(map[string]any{"status": run.Status, "output_json": run.OutputJSON, "error_message": run.ErrorMessage, "completed_at": run.CompletedAt, "updated_at": now}).Error; err != nil {
			return err
		}
		for _, expansion := range expansions {
			child := expansion.Task
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "dedupe_key"}}, DoNothing: true}).Create(child).Error; err != nil {
				return err
			}
			var stored model.Task
			if err := tx.Where("dedupe_key = ?", child.DedupeKey).First(&stored).Error; err != nil {
				return err
			}
			if expansion.Dependency != nil {
				dependency := *expansion.Dependency
				dependency.ChildTaskID = stored.TaskID
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&dependency).Error; err != nil {
					return err
				}
			}
		}
		if finalizeSubmission {
			if err := tx.Model(&model.ResultSubmission{}).Where("task_id = ? AND run_id = ? AND status = ?", task.TaskID, run.RunID, model.ResultSubmissionStatusProcessing).
				Updates(map[string]any{"status": model.ResultSubmissionStatusSucceeded, "error_message": "", "lease_until": nil, "completed_at": now, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
