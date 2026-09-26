package repository

import (
	"context"
	"errors"
	"fmt"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

// MergeGateAuditRepository 记录自动合并门禁的裁决留痕:每验证任务至多
// 一行(task_id 唯一约束),审计即防重。
type MergeGateAuditRepository struct {
	db *gorm.DB
}

func NewMergeGateAuditRepository(db *gorm.DB) *MergeGateAuditRepository {
	return &MergeGateAuditRepository{db: db}
}

func (r *MergeGateAuditRepository) Create(ctx context.Context, audit *model.MergeGateAudit) error {
	if err := r.db.WithContext(ctx).Create(audit).Error; err != nil {
		return fmt.Errorf("create merge gate audit: %w", err)
	}
	return nil
}

func (r *MergeGateAuditRepository) GetByTaskID(ctx context.Context, taskID string) (*model.MergeGateAudit, error) {
	var audit model.MergeGateAudit
	if err := r.db.WithContext(ctx).Where("task_id = ?", taskID).First(&audit).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get merge gate audit by task id: %w", err)
	}
	return &audit, nil
}
