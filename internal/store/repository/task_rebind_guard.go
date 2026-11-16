package repository

import (
	"context"
	"time"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

// RebindExpectation is the caller's snapshot of a task that is about to be
// rebound. The guarded write applies only while the row still matches, so a
// stale admin page cannot overwrite a newer recovery generation or current run.
type RebindExpectation struct {
	Status             model.TaskStatus
	RecoveryGeneration uint64
	CurrentRunID       string
}

func RebindExpectationFromTask(task *model.Task) RebindExpectation {
	if task == nil {
		return RebindExpectation{}
	}
	expectation := RebindExpectation{Status: task.Status, RecoveryGeneration: task.RecoveryGeneration}
	if task.CurrentRunID != nil {
		expectation.CurrentRunID = *task.CurrentRunID
	}
	return expectation
}

// RebindGuarded binds a repository and restores a parked task to pending while
// holding the task lock, writing only the binding and dispatch columns.
func (r *TaskRepository) RebindGuarded(ctx context.Context, taskID, repositoryID string, expect RebindExpectation) (*model.Task, error) {
	var rebound model.Task
	err := withLockedTask(ctx, r.db, taskID, func(tx *gorm.DB, task *model.Task, _ time.Time) error {
		rebound = *task
		if task.Status == model.TaskStatusInterrupted {
			return ErrTaskInterrupted
		}
		if expect.RecoveryGeneration != task.RecoveryGeneration || expect.CurrentRunID != currentRunIDOf(task) {
			return ErrRecoveryRevisionChanged
		}
		next := task.Status
		switch task.Status {
		case model.TaskStatusBlocked, model.TaskStatusHumanRequired:
			next = model.TaskStatusPending
		case model.TaskStatusPending, model.TaskStatusRetryPending, model.TaskStatusWaitingUpstream:
		default:
			return ErrTaskNotClaimable
		}
		if err := tx.Model(task).Updates(map[string]any{
			"status":         next,
			"repository_id":  repositoryID,
			"last_error":     "",
			"current_run_id": nil,
			"lease_until":    nil,
		}).Error; err != nil {
			return err
		}
		rebound.Status = next
		rebound.RepositoryID = &repositoryID
		rebound.LastError = ""
		rebound.CurrentRunID = nil
		rebound.LeaseUntil = nil
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &rebound, nil
}

func currentRunIDOf(task *model.Task) string {
	if task == nil || task.CurrentRunID == nil {
		return ""
	}
	return *task.CurrentRunID
}
