package repository

import (
	"context"
	"fmt"

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
	if err := r.db.WithContext(ctx).Save(execution).Error; err != nil {
		return fmt.Errorf("update agent execution: %w", err)
	}
	return nil
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
