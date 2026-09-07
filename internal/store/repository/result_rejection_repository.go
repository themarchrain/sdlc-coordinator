package repository

import (
	"context"
	"errors"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

type ResultRejectionRepository struct {
	db *gorm.DB
}

func NewResultRejectionRepository(db *gorm.DB) *ResultRejectionRepository {
	return &ResultRejectionRepository{db: db}
}

func (r *ResultRejectionRepository) Record(ctx context.Context, rejection *model.ResultSubmissionRejection) error {
	if r == nil || r.db == nil {
		return errors.New("result rejection repository is required")
	}
	return r.db.WithContext(ctx).Create(rejection).Error
}

func (r *ResultRejectionRepository) CountByRun(ctx context.Context, runID string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("result rejection repository is required")
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.ResultSubmissionRejection{}).Where("run_id = ?", runID).Count(&count).Error
	return count, err
}
