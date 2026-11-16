package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

var (
	ErrInvalidExecutionConfirmation = errors.New("invalid execution exit confirmation")
	ErrExecutionFingerprintChanged  = errors.New("execution evidence changed since the confirmation was read")
	ErrExecutionNotInterrupted      = errors.New("execution is not in the interrupted state")
)

type ExecutionExitConfirmationRepository struct {
	db *gorm.DB
}

func NewExecutionExitConfirmationRepository(db *gorm.DB) *ExecutionExitConfirmationRepository {
	return &ExecutionExitConfirmationRepository{db: db}
}

func (r *ExecutionExitConfirmationRepository) GetByRequest(ctx context.Context, taskID, requestID string) (*model.ExecutionExitConfirmation, error) {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(requestID) == "" {
		return nil, ErrNotFound
	}
	var confirmation model.ExecutionExitConfirmation
	err := r.db.WithContext(ctx).Where("task_id = ? AND request_id = ?", taskID, requestID).First(&confirmation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get execution exit confirmation: %w", err)
	}
	return &confirmation, nil
}

// Create stores one immutable confirmation. The execution row is re-read under
// the task lock so a confirmation read against older evidence cannot vouch for
// a row that changed in the meantime.
func (r *ExecutionExitConfirmationRepository) Create(ctx context.Context, confirmation *model.ExecutionExitConfirmation) (*model.ExecutionExitConfirmation, bool, error) {
	if confirmation == nil || strings.TrimSpace(confirmation.TaskID) == "" || strings.TrimSpace(confirmation.RequestID) == "" ||
		strings.TrimSpace(confirmation.RunID) == "" || strings.TrimSpace(confirmation.ExecutionID) == "" ||
		strings.TrimSpace(confirmation.ExecutionFingerprint) == "" || strings.TrimSpace(confirmation.RequestedBy) == "" ||
		strings.TrimSpace(confirmation.Reason) == "" || strings.TrimSpace(confirmation.RemoteState) == "" {
		return nil, false, ErrInvalidExecutionConfirmation
	}
	if existing, err := r.GetByRequest(ctx, confirmation.TaskID, confirmation.RequestID); err == nil {
		if exitConfirmationReplayMatches(existing, confirmation) {
			return existing, true, nil
		}
		return nil, false, ErrRecoveryRequestConflict
	} else if !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}

	var stored *model.ExecutionExitConfirmation
	replayed := false
	err := withLockedTask(ctx, r.db, confirmation.TaskID, func(tx *gorm.DB, task *model.Task, _ time.Time) error {
		if task.RecoveryGeneration != confirmation.SourceGeneration {
			return ErrRecoveryGenerationMismatch
		}
		var execution model.AgentExecution
		if err := tx.Where("run_id = ?", confirmation.RunID).First(&execution).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if execution.ExecutionID != confirmation.ExecutionID || model.ExecutionExitFingerprint(&execution) != confirmation.ExecutionFingerprint {
			return ErrExecutionFingerprintChanged
		}
		if execution.Status != model.AgentExecutionStatusInterrupted {
			return ErrExecutionNotInterrupted
		}
		var existing model.ExecutionExitConfirmation
		lookup := tx.Where("task_id = ? AND request_id = ?", confirmation.TaskID, confirmation.RequestID).First(&existing).Error
		if lookup == nil {
			if !exitConfirmationReplayMatches(&existing, confirmation) {
				return ErrRecoveryRequestConflict
			}
			stored, replayed = &existing, true
			return nil
		}
		if !errors.Is(lookup, gorm.ErrRecordNotFound) {
			return lookup
		}
		lookup = tx.Where("execution_id = ? AND execution_fingerprint = ?", confirmation.ExecutionID, confirmation.ExecutionFingerprint).First(&existing).Error
		if lookup == nil {
			stored, replayed = &existing, true
			return nil
		}
		if !errors.Is(lookup, gorm.ErrRecordNotFound) {
			return lookup
		}
		if err := tx.Create(confirmation).Error; err != nil {
			return fmt.Errorf("create execution exit confirmation: %w", err)
		}
		stored = confirmation
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return stored, replayed, nil
}

func exitConfirmationReplayMatches(existing, incoming *model.ExecutionExitConfirmation) bool {
	if existing == nil || incoming == nil {
		return false
	}
	return existing.RequestedBy == incoming.RequestedBy &&
		existing.RunID == incoming.RunID &&
		existing.ExecutionID == incoming.ExecutionID &&
		existing.ExecutionFingerprint == incoming.ExecutionFingerprint &&
		existing.SourceGeneration == incoming.SourceGeneration &&
		existing.Reason == incoming.Reason
}
