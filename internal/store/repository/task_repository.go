package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	if err := withLockedTask(ctx, r.db, task.TaskID, func(tx *gorm.DB, current *model.Task, _ time.Time) error {
		if current.Status == model.TaskStatusInterrupted {
			return ErrTaskInterrupted
		}
		return tx.Save(task).Error
	}); err != nil {
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
	var ids []uint
	err := r.db.WithContext(ctx).Model(&model.Task{}).
		Where("role = ?", "ReviewerWorker").
		Where("status IN ?", []model.TaskStatus{
			model.TaskStatusPending, model.TaskStatusClaimed, model.TaskStatusRunning,
			model.TaskStatusFinalizing, model.TaskStatusRetryPending,
		}).
		Where("task_id <> ?", excludeTaskID).
		Where("(id < (?) OR status IN ?)",
			r.db.Model(&model.Task{}).Select("id").Where("task_id = ?", excludeTaskID),
			[]model.TaskStatus{model.TaskStatusClaimed, model.TaskStatusRunning, model.TaskStatusFinalizing}).
		Where("substr(dedupe_key, 1, ?) = ?", len(prefix), prefix).
		Limit(1).Pluck("id", &ids).Error
	if err != nil {
		return false, fmt.Errorf("find active reviewer by dedupe prefix: %w", err)
	}
	return len(ids) > 0, nil
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
	return r.ListDispatchableAfter(ctx, nil, limit, 0)
}

func (r *TaskRepository) MaxTaskID(ctx context.Context) (uint, error) {
	var id uint
	if err := r.db.WithContext(ctx).Model(&model.Task{}).Select("COALESCE(MAX(id), 0)").Scan(&id).Error; err != nil {
		return 0, fmt.Errorf("find maximum task id: %w", err)
	}
	return id, nil
}

func (r *TaskRepository) ListDispatchableAfter(ctx context.Context, after *model.Task, limit int, throughID uint) ([]model.Task, error) {
	if limit <= 0 {
		limit = 100
	}

	query := r.db.WithContext(ctx).
		Where("status IN ?", []model.TaskStatus{model.TaskStatusPending, model.TaskStatusRetryPending})
	if throughID > 0 {
		query = query.Where("id <= ?", throughID)
	}
	if after != nil {
		query = query.Where("updated_at > ? OR (updated_at = ? AND id > ?)", after.UpdatedAt, after.UpdatedAt, after.ID)
	}
	var tasks []model.Task
	if err := query.
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

func (r *TaskRepository) AdvanceGuarded(ctx context.Context, task *model.Task, expected model.TaskStatus) (bool, error) {
	if task == nil {
		return false, fmt.Errorf("task is required")
	}
	result := r.db.WithContext(ctx).
		Model(&model.Task{}).
		Where("task_id = ? AND status = ?", task.TaskID, expected).
		Updates(map[string]any{
			"status":        task.Status,
			"attempt_count": task.AttemptCount,
			"lease_until":   task.LeaseUntil,
		})
	if result.Error != nil {
		return false, fmt.Errorf("advance task guarded: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

// AdvanceGuardedForCurrentRun 以仍处于 expected 状态且 current_run_id 仍是
// runID 为条件推进任务：run 已被接管/重派（current_run_id 前移）时 0 行，
// 迟到失败不得推进当前任务、不得清掉新执行权。是失败回写路径的读-写窗口
// 栅栏——只读 task 校验通过后，接管仍可能发生在这条 UPDATE 之前。
func (r *TaskRepository) AdvanceGuardedForCurrentRun(ctx context.Context, task *model.Task, expected model.TaskStatus, runID string) (bool, error) {
	if task == nil {
		return false, fmt.Errorf("task is required")
	}
	result := r.db.WithContext(ctx).
		Model(&model.Task{}).
		Where("task_id = ? AND status = ? AND current_run_id = ?", task.TaskID, expected, runID).
		Updates(map[string]any{
			"status":        task.Status,
			"attempt_count": task.AttemptCount,
			"lease_until":   task.LeaseUntil,
		})
	if result.Error != nil {
		return false, fmt.Errorf("advance task guarded for current run: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

// RecoverExpiredGuarded 原子回收一条租约过期的任务（跨实例过期恢复的执行权
// 栅栏）：仅当任务仍在 claimed/running 且租约已过窗口时生效，attempt_count
// 在数据库侧原子 +1（MySQL 用 NOW(6) 判到期，SQLite 用参数时间）。目标状态
// 由调用方按预算给出；updated=false 表示另一实例已抢先回收，调用方不得用
// 旧快照续写任何状态或通知。
func (r *TaskRepository) RecoverExpiredGuarded(ctx context.Context, taskID string, next model.TaskStatus, lastError string, now time.Time) (bool, error) {
	updates := map[string]any{
		"status":        next,
		"last_error":    lastError,
		"lease_until":   nil,
		"attempt_count": gorm.Expr("attempt_count + 1"),
	}
	query := r.db.WithContext(ctx).
		Model(&model.Task{}).
		Where("task_id = ? AND status IN ?", taskID, []model.TaskStatus{model.TaskStatusClaimed, model.TaskStatusRunning})
	if r.db.Dialector.Name() == "mysql" {
		query = query.Where("lease_until IS NOT NULL AND lease_until <= NOW(6)")
	} else {
		query = query.Where("lease_until IS NOT NULL AND lease_until <= ?", now)
	}
	result := query.Updates(updates)
	if result.Error != nil {
		return false, fmt.Errorf("recover expired task: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

// UpdateLastErrorIfStatus 仅在任务仍处于期望状态时写入 last_error：任务已被
// 并发推进（状态已变）时放弃写入，不把错误信息挂到更新的状态上。
func (r *TaskRepository) UpdateLastErrorIfStatus(ctx context.Context, taskID string, status model.TaskStatus, lastError string) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&model.Task{}).
		Where("task_id = ? AND status = ?", taskID, status).
		Update("last_error", lastError)
	if result.Error != nil {
		return false, fmt.Errorf("update task last error guarded: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

// RestoreWaitingParentGuarded 把仍处于 waiting_upstream 的父任务原子恢复为
// pending（依赖链解除的执行权栅栏）：双实例同时 resolve 同一父任务的不同
// 子依赖时恰好一个赢家推进，败者 0 行退出且不覆盖胜者写入。
func (r *TaskRepository) RestoreWaitingParentGuarded(ctx context.Context, taskID string) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&model.Task{}).
		Where("task_id = ? AND status = ?", taskID, model.TaskStatusWaitingUpstream).
		Updates(map[string]any{
			"status":      model.TaskStatusPending,
			"lease_until": nil,
		})
	if result.Error != nil {
		return false, fmt.Errorf("restore waiting parent: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
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

	target, err := r.GetByDedupeKey(ctx, dedupeKey)
	if err != nil {
		return nil, fmt.Errorf("finalize merge request rework: %w", err)
	}
	var finalized model.Task
	err = withLockedTask(ctx, r.db, target.TaskID, func(tx *gorm.DB, task *model.Task, _ time.Time) error {
		finalized = *task
		if finalized.Status == model.TaskStatusInterrupted {
			return ErrTaskInterrupted
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
	var target model.Run
	if err := r.db.WithContext(ctx).Where("run_id = ?", runID).First(&target).Error; err != nil {
		return stopLookupError(err)
	}
	switch target.Status {
	case model.RunStatusSucceeded, model.RunStatusFailed, model.RunStatusBlocked:
		return nil
	}
	return withLockedTask(ctx, r.db, target.TaskID, func(tx *gorm.DB, task *model.Task, _ time.Time) error {
		query := tx.Where("run_id = ? AND task_id = ?", runID, task.TaskID)
		if tx.Dialector.Name() == "mysql" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var run model.Run
		if err := query.First(&run).Error; err != nil {
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
		return tx.Save(task).Error
	})
}

// ListByDeliveryBranch 按交付分支读任务(id 降序,最新在前),供验证处方
// 反查产生该分支的开发任务;分支不唯一(历史多轮),调用方按角色过滤。
func (r *TaskRepository) ListByDeliveryBranch(ctx context.Context, deliveryBranch string) ([]model.Task, error) {
	branch := strings.TrimSpace(deliveryBranch)
	if branch == "" {
		return []model.Task{}, nil
	}
	var tasks []model.Task
	if err := r.db.WithContext(ctx).
		Where("delivery_branch = ?", branch).
		Order("id DESC").
		Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("list tasks by delivery branch: %w", err)
	}
	return tasks, nil
}

// ListSucceededVerificationsWithoutAudit 读出已完成且从未被门禁评估过的
// 验证任务(反连接审计表):门禁每任务至多裁决一次,审计行即防重边界。
func (r *TaskRepository) ListSucceededVerificationsWithoutAudit(ctx context.Context, limit int) ([]model.Task, error) {
	return r.ListSucceededVerificationsWithoutAuditAfter(ctx, 0, limit)
}

// ListSucceededVerificationsWithoutAuditAfter 从 afterID 之后扫描,到尾后
// 回绕补齐。这使未审计的瞬时失败任务保留重试机会,又不会长期
// 占满固定取数窗口。
func (r *TaskRepository) ListSucceededVerificationsWithoutAuditAfter(ctx context.Context, afterID uint, limit int) ([]model.Task, error) {
	// limit 的归一点在门禁侧(默认批量);此处不代设默认值。
	var tasks []model.Task
	base := func() *gorm.DB {
		return r.db.WithContext(ctx).
			Where("type = ? AND role = ? AND status = ?", "verify_merge_request", "TesterWorker", model.TaskStatusSucceeded).
			Where("NOT EXISTS (SELECT 1 FROM merge_gate_audits a WHERE a.task_id = tasks.task_id)")
	}
	if err := base().
		Where("id > ?", afterID).
		Order("id ASC").
		Limit(limit).
		Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("list unevaluated verification tasks: %w", err)
	}
	if afterID == 0 || len(tasks) >= limit {
		return tasks, nil
	}
	var wrapped []model.Task
	if err := base().
		Where("id <= ?", afterID).
		Order("id ASC").
		Limit(limit - len(tasks)).
		Find(&wrapped).Error; err != nil {
		return nil, fmt.Errorf("list wrapped unevaluated verification tasks: %w", err)
	}
	tasks = append(tasks, wrapped...)
	return tasks, nil
}
