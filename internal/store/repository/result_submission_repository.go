package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

type ResultSubmissionRepository struct {
	db *gorm.DB
}

func NewResultSubmissionRepository(db *gorm.DB) *ResultSubmissionRepository {
	return &ResultSubmissionRepository{db: db}
}

func (r *ResultSubmissionRepository) Accept(ctx context.Context, submission *model.ResultSubmission, leaseUntil time.Time) error {
	if submission == nil {
		return errors.New("result submission is required")
	}
	submission.Status = model.ResultSubmissionStatusProcessing
	submission.LeaseUntil = &leaseUntil
	submission.AttemptCount = 1

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(submission).Error; err != nil {
			return err
		}
		taskResult := tx.Model(&model.Task{}).
			Where("task_id = ? AND current_run_id = ? AND status = ?", submission.TaskID, submission.RunID, model.TaskStatusRunning).
			Updates(map[string]any{"status": model.TaskStatusFinalizing, "lease_until": nil})
		if taskResult.Error != nil {
			return taskResult.Error
		}
		if taskResult.RowsAffected != 1 {
			return ErrResultNotAcceptable
		}
		runResult := tx.Model(&model.Run{}).
			Where("run_id = ? AND task_id = ? AND status = ?", submission.RunID, submission.TaskID, model.RunStatusRunning).
			Update("status", model.RunStatusFinalizing)
		if runResult.Error != nil {
			return runResult.Error
		}
		if runResult.RowsAffected != 1 {
			return ErrResultNotAcceptable
		}
		return nil
	})
	if err == nil || errors.Is(err, ErrResultNotAcceptable) {
		return err
	}
	if _, lookupErr := r.GetByRunID(ctx, submission.RunID); lookupErr == nil {
		return ErrResultAlreadyAccepted
	}
	return fmt.Errorf("accept result submission: %w", err)
}

func (r *ResultSubmissionRepository) GetByRunID(ctx context.Context, runID string) (*model.ResultSubmission, error) {
	var submission model.ResultSubmission
	if err := r.db.WithContext(ctx).Where("run_id = ?", runID).First(&submission).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get result submission by run id: %w", err)
	}
	return &submission, nil
}

func (r *ResultSubmissionRepository) GetBySubmissionID(ctx context.Context, submissionID string) (*model.ResultSubmission, error) {
	var submission model.ResultSubmission
	if err := r.db.WithContext(ctx).Where("submission_id = ?", submissionID).First(&submission).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get result submission by id: %w", err)
	}
	return &submission, nil
}

func (r *ResultSubmissionRepository) Update(ctx context.Context, submission *model.ResultSubmission) error {
	if err := r.db.WithContext(ctx).Save(submission).Error; err != nil {
		return fmt.Errorf("update result submission: %w", err)
	}
	return nil
}

func (r *ResultSubmissionRepository) ClaimRecoverable(ctx context.Context, submissionID string, now, leaseUntil time.Time, maxAttempts int) (*model.ResultSubmission, error) {
	result := r.db.WithContext(ctx).Model(&model.ResultSubmission{}).
		Where("submission_id = ?", submissionID).
		Where(
			"status = ? OR (status = ? AND attempt_count < ?)",
			model.ResultSubmissionStatusProcessing,
			model.ResultSubmissionStatusFailed,
			maxAttempts,
		).
		Where("lease_until IS NULL OR lease_until <= ?", now).
		Updates(map[string]any{
			"status":        model.ResultSubmissionStatusProcessing,
			"lease_until":   leaseUntil,
			"attempt_count": gorm.Expr("attempt_count + 1"),
		})
	if result.Error != nil {
		return nil, fmt.Errorf("claim recoverable result submission: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return nil, ErrResultNotClaimable
	}
	return r.GetBySubmissionID(ctx, submissionID)
}

func (r *ResultSubmissionRepository) ListRecoverable(ctx context.Context, now time.Time, maxAttempts, limit int) ([]model.ResultSubmission, error) {
	if limit <= 0 {
		limit = 100
	}
	var submissions []model.ResultSubmission
	if err := r.db.WithContext(ctx).
		Where(
			"status = ? OR (status = ? AND attempt_count < ?)",
			model.ResultSubmissionStatusProcessing,
			model.ResultSubmissionStatusFailed,
			maxAttempts,
		).
		Where("lease_until IS NULL OR lease_until <= ?", now).
		Order("created_at ASC").Order("id ASC").Limit(limit).
		Find(&submissions).Error; err != nil {
		return nil, fmt.Errorf("list recoverable result submissions: %w", err)
	}
	return submissions, nil
}

func (r *ResultSubmissionRepository) FailFinalization(
	ctx context.Context,
	submissionID string,
	message string,
	completedAt time.Time,
	maxAttempts int,
) (*model.ResultSubmission, bool, error) {
	var exhausted bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var submission model.ResultSubmission
		if err := tx.Where("submission_id = ?", submissionID).First(&submission).Error; err != nil {
			return err
		}
		submission.Status = model.ResultSubmissionStatusFailed
		submission.ErrorMessage = message
		submission.LeaseUntil = nil
		if err := tx.Save(&submission).Error; err != nil {
			return err
		}
		if submission.AttemptCount < maxAttempts {
			return nil
		}

		taskResult := tx.Model(&model.Task{}).
			Where("task_id = ? AND current_run_id = ? AND status = ?", submission.TaskID, submission.RunID, model.TaskStatusFinalizing).
			Updates(map[string]any{
				"status":      model.TaskStatusHumanRequired,
				"lease_until": nil,
				"last_error":  message,
			})
		if taskResult.Error != nil {
			return taskResult.Error
		}
		if taskResult.RowsAffected != 1 {
			return ErrResultNotAcceptable
		}
		runResult := tx.Model(&model.Run{}).
			Where("run_id = ? AND task_id = ? AND status = ?", submission.RunID, submission.TaskID, model.RunStatusFinalizing).
			Updates(map[string]any{
				"status":        model.RunStatusBlocked,
				"error_message": message,
				"completed_at":  completedAt,
			})
		if runResult.Error != nil {
			return runResult.Error
		}
		if runResult.RowsAffected != 1 {
			return ErrResultNotAcceptable
		}
		exhausted = true
		return nil
	})
	if err != nil {
		return nil, false, fmt.Errorf("fail result finalization: %w", err)
	}
	submission, err := r.GetBySubmissionID(ctx, submissionID)
	return submission, exhausted, err
}

// DeferReconciliationWait 将 submission 标记为等待写回对账:不计失败尝试
// (补偿 claim 时 +1,下限 0),仅记录错误信息。等待的终止由写回对账升级
// (escalateUnknownWriteback 将 submission 置为 unknown)负责,保证有界。
func (r *ResultSubmissionRepository) DeferReconciliationWait(ctx context.Context, submissionID, message string) (*model.ResultSubmission, error) {
	var submission model.ResultSubmission
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("submission_id = ?", submissionID).First(&submission).Error; err != nil {
			return err
		}
		submission.Status = model.ResultSubmissionStatusFailed
		submission.ErrorMessage = message
		submission.LeaseUntil = nil
		if submission.AttemptCount > 0 {
			submission.AttemptCount--
		}
		return tx.Save(&submission).Error
	})
	if err != nil {
		return nil, fmt.Errorf("defer result finalization for reconciliation: %w", err)
	}
	updated, err := r.GetBySubmissionID(ctx, submissionID)
	return updated, err
}
