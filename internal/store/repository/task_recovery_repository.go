package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

type TaskRecoveryRepository struct {
	db *gorm.DB
}

func NewTaskRecoveryRepository(db *gorm.DB) *TaskRecoveryRepository {
	return &TaskRecoveryRepository{db: db}
}

func (r *TaskRecoveryRepository) GetByRequest(ctx context.Context, taskID, requestID string) (*model.TaskRecovery, error) {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(requestID) == "" {
		return nil, ErrNotFound
	}
	var recovery model.TaskRecovery
	err := r.db.WithContext(ctx).Where("task_id = ? AND request_id = ?", taskID, requestID).First(&recovery).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get task recovery by request: %w", err)
	}
	return &recovery, nil
}
