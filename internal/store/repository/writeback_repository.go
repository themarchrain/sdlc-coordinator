package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

type WritebackRepository struct {
	db *gorm.DB
}

func (r *WritebackRepository) GetSucceededMergeRequest(ctx context.Context, projectID string, mergeRequestIID int) (*model.Writeback, error) {
	var writeback model.Writeback
	err := r.db.WithContext(ctx).
		Where(
			"kind = ? AND project_id = ? AND external_id = ? AND status = ?",
			"create_merge_request",
			strings.TrimSpace(projectID),
			strconv.Itoa(mergeRequestIID),
			model.WritebackStatusSucceeded,
		).
		Order("id DESC").
		First(&writeback).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get succeeded merge request writeback: %w", err)
	}
	return &writeback, nil
}

func (r *WritebackRepository) GetLatestSucceededReviewerComment(ctx context.Context, projectID string, mergeRequestIID int) (*model.Writeback, error) {
	var writeback model.Writeback
	err := r.db.WithContext(ctx).
		Joins("JOIN tasks ON tasks.task_id = writebacks.task_id").
		Where("writebacks.kind = ? AND writebacks.project_id = ? AND writebacks.target_type = ? AND writebacks.target_i_id = ? AND writebacks.status = ?", "create_merge_request_note", strings.TrimSpace(projectID), "merge_request", mergeRequestIID, model.WritebackStatusSucceeded).
		Where("tasks.type = ? AND tasks.role = ?", "review_mr", "ReviewerWorker").
		Where("writebacks.operation_marker LIKE ? AND writebacks.operation_marker LIKE ?", "<!-- %-coordinator-operation:codehost:%", "%:create_merge_request_note -->").
		Order("writebacks.id DESC").
		First(&writeback).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get latest succeeded reviewer comment: %w", err)
	}
	return &writeback, nil
}

func NewWritebackRepository(db *gorm.DB) *WritebackRepository {
	return &WritebackRepository{db: db}
}

func (r *WritebackRepository) Create(ctx context.Context, writeback *model.Writeback) error {
	if err := r.db.WithContext(ctx).Create(writeback).Error; err != nil {
		return fmt.Errorf("create writeback: %w", err)
	}
	return nil
}

func (r *WritebackRepository) Update(ctx context.Context, writeback *model.Writeback) error {
	if err := r.db.WithContext(ctx).Save(writeback).Error; err != nil {
		return fmt.Errorf("update writeback: %w", err)
	}
	return nil
}

func (r *WritebackRepository) GetByDedupeKey(ctx context.Context, dedupeKey string) (*model.Writeback, error) {
	var writeback model.Writeback
	if err := r.db.WithContext(ctx).Where("dedupe_key = ?", dedupeKey).First(&writeback).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get writeback by dedupe key: %w", err)
	}
	return &writeback, nil
}

func (r *WritebackRepository) ListByRunID(ctx context.Context, runID string) ([]model.Writeback, error) {
	var writebacks []model.Writeback
	if err := r.db.WithContext(ctx).
		Where("run_id = ?", runID).
		Order("id ASC").
		Find(&writebacks).Error; err != nil {
		return nil, fmt.Errorf("list writebacks by run id: %w", err)
	}
	return writebacks, nil
}

func (r *WritebackRepository) ListUnknown(ctx context.Context, limit int) ([]model.Writeback, error) {
	if limit <= 0 {
		limit = 100
	}
	var writebacks []model.Writeback
	if err := r.db.WithContext(ctx).
		Where("status = ?", model.WritebackStatusUnknown).
		Order("updated_at ASC").
		Order("id ASC").
		Limit(limit).
		Find(&writebacks).Error; err != nil {
		return nil, fmt.Errorf("list unknown writebacks: %w", err)
	}
	return writebacks, nil
}

func (r *WritebackRepository) UpsertByDedupeKey(ctx context.Context, writeback *model.Writeback) error {
	var existing model.Writeback
	err := r.db.WithContext(ctx).
		Where("dedupe_key = ?", writeback.DedupeKey).
		First(&existing).Error
	if err == nil {
		existing.TaskID = writeback.TaskID
		existing.RunID = writeback.RunID
		existing.Kind = writeback.Kind
		existing.ProjectID = writeback.ProjectID
		existing.TargetType = writeback.TargetType
		existing.TargetIID = writeback.TargetIID
		existing.Status = writeback.Status
		existing.AttemptCount = writeback.AttemptCount
		existing.OperationMarker = writeback.OperationMarker
		existing.ReconcileAttemptCount = writeback.ReconcileAttemptCount
		existing.LastReconciledAt = writeback.LastReconciledAt
		existing.RequestJSON = writeback.RequestJSON
		existing.ResponseJSON = writeback.ResponseJSON
		existing.ExternalID = writeback.ExternalID
		existing.ExternalURL = writeback.ExternalURL
		existing.ErrorMessage = writeback.ErrorMessage
		if err := r.db.WithContext(ctx).Save(&existing).Error; err != nil {
			return fmt.Errorf("update writeback by dedupe key: %w", err)
		}
		writeback.ID = existing.ID
		writeback.WritebackID = existing.WritebackID
		return nil
	}
	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("get writeback by dedupe key: %w", err)
	}

	if err := r.db.WithContext(ctx).Create(writeback).Error; err != nil {
		return fmt.Errorf("create writeback by dedupe key: %w", err)
	}
	return nil
}
