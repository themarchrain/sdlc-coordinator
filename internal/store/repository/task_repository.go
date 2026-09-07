package repository

import (
	"context"
	"fmt"
	"time"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

type TaskRepository struct {
	db *gorm.DB
}

func NewTaskRepository(db *gorm.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

func (r *TaskRepository) Create(ctx context.Context, task *model.Task) error {
	if err := r.db.WithContext(ctx).Create(task).Error; err != nil {
		return fmt.Errorf("create task: %w", err)
	}
	return nil
}

func (r *TaskRepository) Update(ctx context.Context, task *model.Task) error {
	if err := r.db.WithContext(ctx).Save(task).Error; err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	return nil
}

func (r *TaskRepository) GetByTaskID(ctx context.Context, taskID string) (*model.Task, error) {
	var task model.Task
	if err := r.db.WithContext(ctx).Where("task_id = ?", taskID).First(&task).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get task by id: %w", err)
	}
	return &task, nil
}

func (r *TaskRepository) GetByDedupeKey(ctx context.Context, dedupeKey string) (*model.Task, error) {
	var task model.Task
	if err := r.db.WithContext(ctx).Where("dedupe_key = ?", dedupeKey).First(&task).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get task by dedupe key: %w", err)
	}
	return &task, nil
}

// HasActiveReviewerTaskForDedupePrefix reports whether another active
// ReviewerWorker task shares the dedupe key prefix (same merge request, any
// head). Used to serialize reviewer dispatch per MR so multi-round review
// conclusions settle in order.
func (r *TaskRepository) HasActiveReviewerTaskForDedupePrefix(ctx context.Context, prefix string, excludeTaskID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Task{}).
		Where("role = ?", "ReviewerWorker").
		Where("status IN ?", []model.TaskStatus{
			model.TaskStatusPending, model.TaskStatusClaimed, model.TaskStatusRunning,
			model.TaskStatusFinalizing, model.TaskStatusRetryPending,
		}).
		Where("task_id <> ?", excludeTaskID).
		Where("substr(dedupe_key, 1, ?) = ?", len(prefix), prefix).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("find active reviewer by dedupe prefix: %w", err)
	}
	return count > 0, nil
}

func (r *TaskRepository) ListExpiredLeases(ctx context.Context, now time.Time, limit int) ([]model.Task, error) {
	if limit <= 0 {
		limit = 100
	}

	var tasks []model.Task
	if err := r.db.WithContext(ctx).
		Where("status IN ?", []model.TaskStatus{model.TaskStatusClaimed, model.TaskStatusRunning}).
		Where("lease_until IS NOT NULL AND lease_until <= ?", now).
		Order("lease_until ASC").
		Order("id ASC").
		Limit(limit).
		Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("list expired leases: %w", err)
	}
	return tasks, nil
}

func (r *TaskRepository) ListDispatchable(ctx context.Context, limit int) ([]model.Task, error) {
	if limit <= 0 {
		limit = 100
	}

	var tasks []model.Task
	if err := r.db.WithContext(ctx).
		Where("status IN ?", []model.TaskStatus{model.TaskStatusPending, model.TaskStatusRetryPending}).
		Order("updated_at ASC").
		Order("id ASC").
		Limit(limit).
		Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("list dispatchable tasks: %w", err)
	}
	return tasks, nil
}

func (r *TaskRepository) Claim(ctx context.Context, taskID, runID string, leaseUntil time.Time) error {
	updates := map[string]any{
		"status":         model.TaskStatusClaimed,
		"current_run_id": runID,
		"lease_until":    leaseUntil,
	}

	result := r.db.WithContext(ctx).
		Model(&model.Task{}).
		Where("task_id = ? AND status IN ?", taskID, []model.TaskStatus{model.TaskStatusPending, model.TaskStatusRetryPending}).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("claim task: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrTaskNotClaimable
	}
	return nil
}

func (r *TaskRepository) RenewActiveLease(ctx context.Context, taskID, runID string, leaseUntil time.Time) error {
	result := r.db.WithContext(ctx).
		Model(&model.Task{}).
		Where("task_id = ? AND current_run_id = ? AND status = ?", taskID, runID, model.TaskStatusRunning).
		Update("lease_until", leaseUntil)
	if result.Error != nil {
		return fmt.Errorf("renew active task lease: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrTaskLeaseNotRenewable
	}
	return nil
}

func (r *TaskRepository) ParkUnbound(ctx context.Context, taskID string) error {
	result := r.db.WithContext(ctx).
		Model(&model.Task{}).
		Where("task_id = ? AND status IN ?", taskID, []model.TaskStatus{model.TaskStatusPending, model.TaskStatusRetryPending}).
		Updates(map[string]any{
			"status":     model.TaskStatusBlocked,
			"last_error": "repository not bound: task has no repository_id and cannot be dispatched",
		})
	if result.Error != nil {
		return fmt.Errorf("park unbound task: %w", result.Error)
	}
	return nil
}

func (r *TaskRepository) ResumeHumanRequired(ctx context.Context, dedupeKey, contextJSON string) error {
	updates := map[string]any{
		"status":         model.TaskStatusPending,
		"current_run_id": nil,
		"lease_until":    nil,
		"context_json":   contextJSON,
		"last_error":     "",
	}

	result := r.db.WithContext(ctx).
		Model(&model.Task{}).
		Where("dedupe_key = ? AND role = ? AND status = ?", dedupeKey, "DeveloperWorker", model.TaskStatusHumanRequired).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("resume human-required task: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrTaskNotClaimable
	}
	return nil
}

func (r *TaskRepository) ResumeRework(ctx context.Context, dedupeKey, contextJSON string) error {
	updates := map[string]any{
		"status":         model.TaskStatusPending,
		"current_run_id": nil,
		"lease_until":    nil,
		"context_json":   contextJSON,
		"last_error":     "",
	}

	result := r.db.WithContext(ctx).
		Model(&model.Task{}).
		Where("dedupe_key = ? AND type = ? AND role = ?", dedupeKey, "rework_merge_request", "DeveloperWorker").
		Where("status IN ?", []model.TaskStatus{model.TaskStatusWaitingMerge, model.TaskStatusHumanRequired, model.TaskStatusBlocked}).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("resume merge request rework task: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrTaskNotClaimable
	}
	return nil
}

func (r *TaskRepository) MarkHumanRequiredIfDispatchable(ctx context.Context, taskID, reason string) error {
	result := r.db.WithContext(ctx).
		Model(&model.Task{}).
		Where("task_id = ? AND status IN ?", taskID, []model.TaskStatus{model.TaskStatusPending, model.TaskStatusRetryPending}).
		Updates(map[string]any{
			"status":         model.TaskStatusHumanRequired,
			"current_run_id": nil,
			"lease_until":    nil,
			"last_error":     reason,
		})
	if result.Error != nil {
		return fmt.Errorf("mark dispatchable task human required: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrTaskNotClaimable
	}
	return nil
}

func (r *TaskRepository) FinalizeRework(ctx context.Context, dedupeKey string, next model.TaskStatus, lastError string, completedAt time.Time) (*model.Task, error) {
	if next != model.TaskStatusSucceeded && next != model.TaskStatusBlocked {
		return nil, fmt.Errorf("finalize merge request rework: unsupported status %s", next)
	}

	var finalized model.Task
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("dedupe_key = ?", dedupeKey).First(&finalized).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrNotFound
			}
			return err
		}
		if finalized.Type != model.TaskTypeReworkMergeRequest || finalized.Role != "DeveloperWorker" {
			return ErrTaskNotClaimable
		}
		if finalized.Status == model.TaskStatusSucceeded {
			return nil
		}
		if finalized.CurrentRunID != nil {
			result := tx.Model(&model.Run{}).
				Where("run_id = ? AND task_id = ? AND status IN ?", *finalized.CurrentRunID, finalized.TaskID, []model.RunStatus{model.RunStatusPending, model.RunStatusRunning, model.RunStatusFinalizing}).
				Updates(map[string]any{
					"status":        model.RunStatusBlocked,
					"error_message": lastError,
					"completed_at":  completedAt,
				})
			if result.Error != nil {
				return result.Error
			}
		}
		finalized.Status = next
		finalized.LeaseUntil = nil
		finalized.LastError = lastError
		return tx.Save(&finalized).Error
	})
	if err != nil {
		return nil, fmt.Errorf("finalize merge request rework: %w", err)
	}
	return &finalized, nil
}

// BlockRunAndRequireHuman 在单个事务里把 run 终态化为 blocked，并把其 task 迁移到
// human_required。run 已终态化时幂等返回；run 已不是 task 的当前 run（stale）时
// 只终态化 run，不动 task。task 状态白名单与 orchestrator 状态机迁入
// human_required 的允许来源保持一致。
func (r *TaskRepository) BlockRunAndRequireHuman(ctx context.Context, runID, message string, completedAt time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run model.Run
		if err := tx.Where("run_id = ?", runID).First(&run).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrNotFound
			}
			return err
		}
		switch run.Status {
		case model.RunStatusSucceeded, model.RunStatusFailed, model.RunStatusBlocked:
			return nil
		}
		result := tx.Model(&model.Run{}).
			Where("run_id = ? AND status IN ?", runID, []model.RunStatus{model.RunStatusPending, model.RunStatusRunning, model.RunStatusFinalizing}).
			Updates(map[string]any{
				"status":        model.RunStatusBlocked,
				"error_message": message,
				"completed_at":  completedAt,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrTaskNotClaimable
		}
		var task model.Task
		if err := tx.Where("task_id = ?", run.TaskID).First(&task).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrNotFound
			}
			return err
		}
		if task.CurrentRunID == nil || *task.CurrentRunID != run.RunID {
			return nil
		}
		switch task.Status {
		case model.TaskStatusClaimed, model.TaskStatusRunning, model.TaskStatusFinalizing:
		default:
			return fmt.Errorf("cannot require human for task in status %s", task.Status)
		}
		task.Status = model.TaskStatusHumanRequired
		task.LeaseUntil = nil
		task.LastError = message
		return tx.Save(&task).Error
	})
}
