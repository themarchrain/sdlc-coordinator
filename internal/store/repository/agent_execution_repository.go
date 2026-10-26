package repository

import (
	"context"
	"fmt"
	"time"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AgentExecutionRepository struct {
	db *gorm.DB
}

func NewAgentExecutionRepository(db *gorm.DB) *AgentExecutionRepository {
	return &AgentExecutionRepository{db: db}
}

func (r *AgentExecutionRepository) Create(ctx context.Context, execution *model.AgentExecution) error {
	if err := r.db.WithContext(ctx).Create(execution).Error; err != nil {
		return fmt.Errorf("create agent execution: %w", err)
	}
	return nil
}

func (r *AgentExecutionRepository) Update(ctx context.Context, execution *model.AgentExecution) error {
	result := r.db.WithContext(ctx).Model(&model.AgentExecution{}).
		Where("run_id = ? AND execution_id = ? AND request_hash = ?", execution.RunID, execution.ExecutionID, execution.RequestHash).
		Where("status NOT IN ? OR status = ?", terminalExecutionStatuses(), execution.Status).
		Select("*").Omit("id", "run_id", "execution_id", "request_hash", "create_spec_json", "created_at").Updates(execution)
	if result.Error != nil {
		return fmt.Errorf("update agent execution: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		stored, err := r.GetByRunID(ctx, execution.RunID)
		if err != nil {
			return err
		}
		*execution = *stored
	}
	return nil
}

func terminalExecutionStatuses() []model.AgentExecutionStatus {
	return []model.AgentExecutionStatus{model.AgentExecutionStatusSucceeded, model.AgentExecutionStatusFailed, model.AgentExecutionStatusCancelled, model.AgentExecutionStatusInterrupted}
}

func (r *AgentExecutionRepository) CreateDispatchIntent(ctx context.Context, execution *model.AgentExecution) error {
	return withLockedCurrentRun(ctx, r.db, execution.RunID, func(tx *gorm.DB, run *model.Run) error {
		if run.Status != model.RunStatusRunning && run.Status != model.RunStatusFinalizing {
			return ErrTaskNotClaimable
		}
		if run.RunnerEndpoint != "" && run.RunnerEndpoint != execution.RunnerEndpoint {
			return ErrTaskNotClaimable
		}
		if err := tx.Model(run).Updates(map[string]any{"external_system": "agent-runner", "external_id": execution.ExecutionID}).Error; err != nil {
			return err
		}
		return tx.Create(execution).Error
	})
}

func (r *AgentExecutionRepository) TouchLastObservedAt(ctx context.Context, runID string, at time.Time) error {
	if err := r.db.WithContext(ctx).Model(&model.AgentExecution{}).
		Where("run_id = ? AND status IN ?", runID, []model.AgentExecutionStatus{
			model.AgentExecutionStatusDispatching, model.AgentExecutionStatusAccepted,
			model.AgentExecutionStatusStarting, model.AgentExecutionStatusRunning,
		}).Update("last_observed_at", at).Error; err != nil {
		return fmt.Errorf("touch agent execution observation for run %s: %w", runID, err)
	}
	return nil
}

func (r *AgentExecutionRepository) SettleUnacceptedIfDispatching(ctx context.Context, runID string, now time.Time) (bool, error) {
	result := r.db.WithContext(ctx).Model(&model.AgentExecution{}).
		Where("run_id = ? AND status = ?", runID, model.AgentExecutionStatusDispatching).
		Updates(map[string]any{
			"status":           model.AgentExecutionStatusInterrupted,
			"error_code":       "dispatch_not_accepted_after_run_terminal",
			"error_message":    "run finalized before runner accepted the execution",
			"finished_at":      now,
			"last_observed_at": now,
		})
	if result.Error != nil {
		return false, fmt.Errorf("settle unaccepted execution for run %s: %w", runID, result.Error)
	}
	return result.RowsAffected == 1, nil
}

// UpdateRunnerEndpointIfUnchanged 以旧端点值为条件推进端点列（列级条件写，
// 不整行覆盖）。fallback 轮换在远端 Create 之前调用：新目标必须先落库，
// 接管方才不会按旧端点重发同一 run 造成双执行。0 行且库内已是他方更新时
// 由调用方按不确定语义处理（保持待对账，不换端点）。
func (r *AgentExecutionRepository) UpdateRunnerEndpointIfUnchanged(ctx context.Context, runID, fromEndpoint, toEndpoint string) error {
	return withLockedCurrentRun(ctx, r.db, runID, func(tx *gorm.DB, _ *model.Run) error {
		result := tx.Model(&model.AgentExecution{}).
			Where("run_id = ? AND runner_endpoint = ?", runID, fromEndpoint).
			Update("runner_endpoint", toEndpoint)
		if result.Error != nil {
			return fmt.Errorf("update runner endpoint for run %s: %w", runID, result.Error)
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("runner endpoint for run %s was changed concurrently (expected %q)", runID, fromEndpoint)
		}
		return nil
	})
}

func (r *AgentExecutionRepository) GetByRunID(ctx context.Context, runID string) (*model.AgentExecution, error) {
	var execution model.AgentExecution
	if err := r.db.WithContext(ctx).Where("run_id = ?", runID).First(&execution).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get agent execution by run id: %w", err)
	}
	return &execution, nil
}

func (r *AgentExecutionRepository) ListActive(ctx context.Context, limit int) ([]model.AgentExecution, error) {
	if limit <= 0 {
		limit = 100
	}
	var executions []model.AgentExecution
	if err := r.db.WithContext(ctx).
		Where("status IN ?", []model.AgentExecutionStatus{
			model.AgentExecutionStatusAccepted,
			model.AgentExecutionStatusStarting,
			model.AgentExecutionStatusRunning,
		}).
		Order("last_observed_at ASC").
		Order("id ASC").
		Limit(limit).
		Find(&executions).Error; err != nil {
		return nil, fmt.Errorf("list active agent executions: %w", err)
	}
	return executions, nil
}

func (r *AgentExecutionRepository) UpsertEvents(ctx context.Context, events []model.AgentExecutionEvent) error {
	if len(events) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "execution_id"}, {Name: "seq"}},
		DoNothing: true,
	}).Create(&events).Error; err != nil {
		return fmt.Errorf("upsert agent execution events: %w", err)
	}
	return nil
}
