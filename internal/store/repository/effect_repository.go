package repository

import (
	"context"
	"fmt"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

type EffectRepository struct {
	db *gorm.DB
}

func NewEffectRepository(db *gorm.DB) *EffectRepository {
	return &EffectRepository{db: db}
}

func (r *EffectRepository) Create(ctx context.Context, effect *model.Effect) error {
	if err := r.db.WithContext(ctx).Create(effect).Error; err != nil {
		return fmt.Errorf("create effect: %w", err)
	}
	return nil
}

func (r *EffectRepository) Update(ctx context.Context, effect *model.Effect) error {
	if err := r.db.WithContext(ctx).Save(effect).Error; err != nil {
		return fmt.Errorf("update effect: %w", err)
	}
	return nil
}

func (r *EffectRepository) GetByDedupeKey(ctx context.Context, dedupeKey string) (*model.Effect, error) {
	var effect model.Effect
	if err := r.db.WithContext(ctx).Where("dedupe_key = ?", dedupeKey).First(&effect).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get effect by dedupe key: %w", err)
	}
	return &effect, nil
}

func (r *EffectRepository) ListByRunID(ctx context.Context, runID string) ([]model.Effect, error) {
	var effects []model.Effect
	if err := r.db.WithContext(ctx).
		Where("run_id = ?", runID).
		Order("id ASC").
		Find(&effects).Error; err != nil {
		return nil, fmt.Errorf("list effects by run id: %w", err)
	}
	return effects, nil
}

func (r *EffectRepository) ListPending(ctx context.Context, limit int) ([]model.Effect, error) {
	if limit <= 0 {
		limit = 100
	}

	var effects []model.Effect
	if err := r.db.WithContext(ctx).
		Where("status = ?", model.EffectStatusPending).
		Order("created_at ASC").
		Order("id ASC").
		Limit(limit).
		Find(&effects).Error; err != nil {
		return nil, fmt.Errorf("list pending effects: %w", err)
	}
	return effects, nil
}

// ListStaleRunning 返回停留 running 超过 staleAfter 的效果——进程在执行返回后、
// 终态落库前崩溃时会遗留这种行;ListPending 只取 pending,它们永远不会被回访。
func (r *EffectRepository) ListStaleRunning(ctx context.Context, staleBefore interface{}, limit int) ([]model.Effect, error) {
	if limit <= 0 {
		limit = 100
	}
	var effects []model.Effect
	if err := r.db.WithContext(ctx).
		Where("status = ? AND updated_at < ?", model.EffectStatusRunning, staleBefore).
		Order("updated_at ASC").
		Limit(limit).
		Find(&effects).Error; err != nil {
		return nil, fmt.Errorf("list stale running effects: %w", err)
	}
	return effects, nil
}

func (r *EffectRepository) UpsertByDedupeKey(ctx context.Context, effect *model.Effect) error {
	var existing model.Effect
	err := r.db.WithContext(ctx).
		Where("dedupe_key = ?", effect.DedupeKey).
		First(&existing).Error
	if err == nil {
		existing.TaskID = effect.TaskID
		existing.RunID = effect.RunID
		existing.Kind = effect.Kind
		existing.Target = effect.Target
		existing.Critical = effect.Critical
		existing.Status = effect.Status
		existing.AttemptCount = effect.AttemptCount
		existing.RequestJSON = effect.RequestJSON
		existing.ResponseJSON = effect.ResponseJSON
		existing.ErrorMessage = effect.ErrorMessage
		if err := r.db.WithContext(ctx).Save(&existing).Error; err != nil {
			return fmt.Errorf("update effect by dedupe key: %w", err)
		}
		effect.ID = existing.ID
		effect.EffectID = existing.EffectID
		return nil
	}
	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("get effect by dedupe key: %w", err)
	}

	if err := r.db.WithContext(ctx).Create(effect).Error; err != nil {
		return fmt.Errorf("create effect by dedupe key: %w", err)
	}
	return nil
}
