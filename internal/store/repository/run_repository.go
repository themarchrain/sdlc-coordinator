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
