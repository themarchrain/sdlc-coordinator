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
