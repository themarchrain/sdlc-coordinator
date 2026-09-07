package repository

import (
	"context"
	"fmt"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

type InboundEventRepository struct {
	db *gorm.DB
}

func NewInboundEventRepository(db *gorm.DB) *InboundEventRepository {
	return &InboundEventRepository{db: db}
}

func (r *InboundEventRepository) Create(ctx context.Context, event *model.InboundEvent) error {
	if err := r.db.WithContext(ctx).Create(event).Error; err != nil {
		return fmt.Errorf("create inbound event: %w", err)
	}
	return nil
}

func (r *InboundEventRepository) Update(ctx context.Context, event *model.InboundEvent) error {
	if err := r.db.WithContext(ctx).Save(event).Error; err != nil {
		return fmt.Errorf("update inbound event: %w", err)
	}
	return nil
}

func (r *InboundEventRepository) GetByDedupeKey(ctx context.Context, dedupeKey string) (*model.InboundEvent, error) {
	var event model.InboundEvent
	if err := r.db.WithContext(ctx).Where("dedupe_key = ?", dedupeKey).First(&event).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get inbound event by dedupe key: %w", err)
	}
	return &event, nil
}

func (r *InboundEventRepository) ListByStatus(ctx context.Context, status model.InboundEventStatus, limit int) ([]model.InboundEvent, error) {
	if limit <= 0 {
		limit = 100
	}

	var events []model.InboundEvent
	if err := r.db.WithContext(ctx).
		Where("status = ?", status).
		Order("received_at ASC").
		Order("id ASC").
		Limit(limit).
		Find(&events).Error; err != nil {
		return nil, fmt.Errorf("list inbound events by status: %w", err)
	}
	return events, nil
}
