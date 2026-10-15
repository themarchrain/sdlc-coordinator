package repository

import (
	"context"
	"fmt"
	"time"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

type RunRepository struct {
	db *gorm.DB
}

func NewRunRepository(db *gorm.DB) *RunRepository {
	return &RunRepository{db: db}
}

func (r *RunRepository) Create(ctx context.Context, run *model.Run) error {
	if err := r.db.WithContext(ctx).Create(run).Error; err != nil {
		return fmt.Errorf("create run: %w", err)
	}
	return nil
}

func (r *RunRepository) Update(ctx context.Context, run *model.Run) error {
	if err := r.db.WithContext(ctx).Save(run).Error; err != nil {
		return fmt.Errorf("update run: %w", err)
	}
	return nil
}

// UpdateProviderIfEmpty 仅在 run 尚未钉住 CLI 时写入 provider 列。列级
// 条件更新不整行覆盖：钉住发生在分钟级的工作区准备之后，整行保存会把
// 并发终态化的状态字段回滚成旧值。
func (r *RunRepository) UpdateProviderIfEmpty(ctx context.Context, runID, provider string) error {
	if err := r.db.WithContext(ctx).Model(&model.Run{}).
		Where("run_id = ? AND (provider = '' OR provider IS NULL)", runID).
		Update("provider", provider).Error; err != nil {
		return fmt.Errorf("update run provider: %w", err)
	}
	return nil
}

// UpdateEffortIfEmpty 仅在 run 尚未钉住推理强度时写入 effort 列；动机与
// UpdateProviderIfEmpty 相同：列级条件更新不整行覆盖，避免把并发终态化
// 的状态字段回滚成旧值。
func (r *RunRepository) UpdateEffortIfEmpty(ctx context.Context, runID, effort string) error {
	if err := r.db.WithContext(ctx).Model(&model.Run{}).
		Where("run_id = ? AND (effort = '' OR effort IS NULL)", runID).
		Update("effort", effort).Error; err != nil {
		return fmt.Errorf("update run effort: %w", err)
	}
	return nil
}

// UpdateTaskTimeoutIfEmpty 仅在 run 尚未钉住单任务超时时写入
// task_timeout_seconds 列；动机与 UpdateProviderIfEmpty 相同：列级条件
// 更新不整行覆盖，避免把并发终态化的状态字段回滚成旧值。
func (r *RunRepository) UpdateTaskTimeoutIfEmpty(ctx context.Context, runID string, seconds int64) error {
	if err := r.db.WithContext(ctx).Model(&model.Run{}).
		Where("run_id = ? AND (task_timeout_seconds = 0 OR task_timeout_seconds IS NULL)", runID).
		Update("task_timeout_seconds", seconds).Error; err != nil {
		return fmt.Errorf("update run task timeout: %w", err)
	}
	return nil
}

// UpdateDispatchProjection 只写派发投影列（external_system/external_id/
// input_json）：不整行覆盖，避免把并发推进的状态字段回滚成旧快照。
func (r *RunRepository) UpdateDispatchProjection(ctx context.Context, runID, externalSystem, externalID, inputJSON string) error {
	if err := r.db.WithContext(ctx).Model(&model.Run{}).
		Where("run_id = ?", runID).
		Updates(map[string]any{
			"external_system": externalSystem,
			"external_id":     externalID,
			"input_json":      inputJSON,
		}).Error; err != nil {
		return fmt.Errorf("update run dispatch projection: %w", err)
	}
	return nil
}

// PinRunnerEndpointIfEmpty 仅在 run 尚未钉住 runner 端点时写入端点列（列级
// 条件更新，动机同 UpdateProviderIfEmpty）。端点必须在工作区准备之前钉住：
// 准备、任务包投递、执行、commit/push、清理全链按同一端点寻址，崩溃恢复
// 路径按钉住值重放，不换 runner。
func (r *RunRepository) PinRunnerEndpointIfEmpty(ctx context.Context, runID, endpoint string) error {
	if err := r.db.WithContext(ctx).Model(&model.Run{}).
		Where("run_id = ? AND (runner_endpoint = '' OR runner_endpoint IS NULL)", runID).
		Update("runner_endpoint", endpoint).Error; err != nil {
		return fmt.Errorf("pin run runner endpoint: %w", err)
	}
	return nil
}

// PinRunnerWorkspacePathIfEmpty 仅在 run 尚未记录 runner 工作区路径时写入
// （prepare 返回值）。恢复路径按该路径重建执行请求，两侧路径不共享时这是
// 唯一权威来源。
func (r *RunRepository) PinRunnerWorkspacePathIfEmpty(ctx context.Context, runID, path string) error {
	if err := r.db.WithContext(ctx).Model(&model.Run{}).
		Where("run_id = ? AND (runner_workspace_path = '' OR runner_workspace_path IS NULL)", runID).
		Update("runner_workspace_path", path).Error; err != nil {
		return fmt.Errorf("pin run runner workspace path: %w", err)
	}
	return nil
}

// UpdateFailedIfInFlight 仅在 run 仍处于在途状态时把状态置为 failed 并写
// error_message、completed_at，返回是否实际生效。在途集合取 pending、
// running、finalizing，是既有终态写路径界定终态集合（succeeded、failed、
// blocked）的补集：迟到失败回写持有派发前的旧快照，分钟级工作区准备窗口
// 内 run 可能已被结果回传通道终态化，条件不满足时不落任何列、不整行覆盖，
// 避免把并发终态回滚成旧值。
func (r *RunRepository) UpdateFailedIfInFlight(ctx context.Context, runID, message string, completedAt time.Time) (bool, error) {
	result := r.db.WithContext(ctx).Model(&model.Run{}).
		Where("run_id = ? AND status IN ?", runID, []model.RunStatus{
			model.RunStatusPending, model.RunStatusRunning, model.RunStatusFinalizing,
		}).
		Updates(map[string]any{
			"status":        model.RunStatusFailed,
			"error_message": message,
			"completed_at":  completedAt,
		})
	if result.Error != nil {
		return false, fmt.Errorf("update run failure if in flight: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (r *RunRepository) GetByRunID(ctx context.Context, runID string) (*model.Run, error) {
	var run model.Run
	if err := r.db.WithContext(ctx).Where("run_id = ?", runID).First(&run).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get run by id: %w", err)
	}
	return &run, nil
}

// ListTerminalWithActiveExecution 列出 run 行已终态、但 AgentExecution 投影
// 仍未闭合的 run：MCP 主通道首提即终态化 run 行，执行投影要等 CLI 进程退出
// 后的最终观察才能收尾；tick 用它补一次观察，防止投影滞留 running 令
// artifact 清理永久跳过。
func (r *RunRepository) ListTerminalWithActiveExecution(ctx context.Context, limit int) ([]model.Run, error) {
	if limit <= 0 {
		limit = 100
	}
	var runs []model.Run
	if err := r.db.WithContext(ctx).
		Where("status IN ?", []model.RunStatus{model.RunStatusSucceeded, model.RunStatusFailed, model.RunStatusBlocked}).
		Where("external_system = ?", "agent-runner").
		Where("EXISTS (SELECT 1 FROM agent_executions ae WHERE ae.run_id = runs.run_id AND ae.status IN ?)",
			[]model.AgentExecutionStatus{model.AgentExecutionStatusAccepted, model.AgentExecutionStatusStarting, model.AgentExecutionStatusRunning}).
		Order("updated_at ASC").
		Order("id ASC").
		Limit(limit).
		Find(&runs).Error; err != nil {
		return nil, fmt.Errorf("list terminal runs with active execution: %w", err)
	}
	return runs, nil
}

func (r *RunRepository) ListRunning(ctx context.Context, limit int) ([]model.Run, error) {
	if limit <= 0 {
		limit = 100
	}

	var runs []model.Run
	if err := r.db.WithContext(ctx).
		Where("status = ?", model.RunStatusRunning).
		Order("updated_at ASC").
		Order("id ASC").
		Limit(limit).
		Find(&runs).Error; err != nil {
		return nil, fmt.Errorf("list running runs: %w", err)
	}
	return runs, nil
}

func (r *RunRepository) ListSucceededForArtifactCleanup(ctx context.Context, before time.Time, limit int) ([]model.Run, error) {
	if limit <= 0 {
		limit = 10
	}
	var runs []model.Run
	if err := r.db.WithContext(ctx).
		Where("status = ? AND error_message = ? AND completed_at IS NOT NULL AND completed_at <= ? AND artifacts_purged_at IS NULL AND external_system = ?",
			model.RunStatusSucceeded, "", before, "agent-runner").
		Order("completed_at ASC").
		Order("id ASC").
		Limit(limit).
		Find(&runs).Error; err != nil {
		return nil, fmt.Errorf("list succeeded runs for artifact cleanup: %w", err)
	}
	return runs, nil
}

// MarkArtifactsPurgingIfUnclean 抢占一个 run 的清理归属（跨实例清理栅栏）：
// 仅当 artifacts_purged_at 仍为空时写入哨兵时间，双实例同时进入清理批次时
// 恰好一个赢家执行远端删除；applied=false 表示归属在他方，调用方跳过。
// 清理中途失败由调用方把哨兵清回 NULL，下一轮批次重新竞争。
func (r *RunRepository) MarkArtifactsPurgingIfUnclean(ctx context.Context, runID string, now time.Time) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&model.Run{}).
		Where("run_id = ? AND artifacts_purged_at IS NULL", runID).
		Update("artifacts_purged_at", now)
	if result.Error != nil {
		return false, fmt.Errorf("mark artifacts purging: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

// ClearArtifactsPurging 把清理哨兵清回 NULL（清理失败的回滚通道）。
func (r *RunRepository) ClearArtifactsPurging(ctx context.Context, runID string) error {
	if err := r.db.WithContext(ctx).
		Model(&model.Run{}).
		Where("run_id = ?", runID).
		Update("artifacts_purged_at", nil).Error; err != nil {
		return fmt.Errorf("clear artifacts purging: %w", err)
	}
	return nil
}
