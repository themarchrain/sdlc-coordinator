package repository

import (
	"context"
	"errors"
	"time"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidActionAdmission = errors.New("invalid action admission")
	ErrActionAlreadyAdmitted  = errors.New("action already admitted")
	ErrActionLeaseLost        = errors.New("action execution right expired")
	ErrActionOutcomeUnknown   = errors.New("previous action outcome unknown")
)

type ActionAdmissionRequest struct {
	TaskID           string
	RunID            string
	Kind             model.ActionKind
	ActionKey        string
	Attempt          int
	LeaseOwner       string
	LeaseGeneration  uint64
	IdempotentReplay bool
	RequestHash      string
}

type ActionAdmissionRepository struct{ db *gorm.DB }

func (r *ActionAdmissionRepository) ConfirmCompleted(ctx context.Context, admission *model.ActionAdmission) error {
	if admission == nil || (admission.Kind != model.ActionGitCommit && admission.Kind != model.ActionGitPush) {
		return ErrInvalidActionAdmission
	}
	return withLockedTask(ctx, r.db, admission.TaskID, func(tx *gorm.DB, _ *model.Task, now time.Time) error {
		result := tx.Model(&model.ActionAdmission{}).Where("admission_id = ? AND task_id = ? AND run_id = ? AND kind = ? AND lease_owner = ? AND lease_generation = ? AND not_sent_at IS NULL", admission.AdmissionID, admission.TaskID, admission.RunID, admission.Kind, admission.LeaseOwner, admission.LeaseGeneration).Update("completed_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrActionLeaseLost
		}
		return nil
	})
}

func NewActionAdmissionRepository(db *gorm.DB) *ActionAdmissionRepository {
	return &ActionAdmissionRepository{db: db}
}

func (r *ActionAdmissionRepository) AdmitAction(ctx context.Context, request ActionAdmissionRequest) (*model.ActionAdmission, error) {
	scope, err := ActionAdmissionScope(request)
	if err != nil {
		return nil, err
	}
	if request.TaskID == "" || request.ActionKey == "" || request.Attempt <= 0 || request.LeaseOwner == "" || request.LeaseGeneration == 0 {
		return nil, ErrInvalidActionAdmission
	}
	var admission *model.ActionAdmission
	err = withLockedTask(ctx, r.db, request.TaskID, func(tx *gorm.DB, task *model.Task, now time.Time) error {
		if task.Status == model.TaskStatusInterrupted {
			return ErrTaskInterrupted
		}
		var stopped int64
		if err := tx.Model(&model.StopRequest{}).Where("dedupe_key = ?", "task:"+task.TaskID).Count(&stopped).Error; err != nil {
			return err
		}
		if stopped != 0 {
			return ErrTaskInterrupted
		}
		if request.RunID != "" {
			if task.CurrentRunID == nil || *task.CurrentRunID != request.RunID {
				return ErrStopNotCurrentRun
			}
			query := tx.Where("run_id = ? AND task_id = ?", request.RunID, task.TaskID)
			if tx.Dialector.Name() == "mysql" {
				query = query.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			var run model.Run
			if err := query.First(&run).Error; err != nil {
				return stopLookupError(err)
			}
			if run.RecoveryGeneration != task.RecoveryGeneration {
				return ErrRecoveryGenerationMismatch
			}
			if run.Status == model.RunStatusInterrupted {
				return ErrTaskInterrupted
			}
			if request.Kind == model.ActionExecutionCreate || request.Kind == model.ActionGitCommit || request.Kind == model.ActionGitPush {
				if (task.Status != model.TaskStatusRunning && task.Status != model.TaskStatusFinalizing) || (run.Status != model.RunStatusRunning && run.Status != model.RunStatusFinalizing) {
					return ErrResultNotAcceptable
				}
			}
		} else if request.Kind != model.ActionIMMessage {
			return ErrInvalidActionAdmission
		}
		query := tx.Where("scope = ?", scope)
		if tx.Dialector.Name() == "mysql" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var lease model.WorkItemLease
		if err := query.First(&lease).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrActionLeaseLost
			}
			return err
		}
		if lease.Owner != request.LeaseOwner || lease.Generation != request.LeaseGeneration || !lease.LeaseUntil.After(now) {
			return ErrActionLeaseLost
		}
		var previous model.ActionAdmission
		err := tx.Where("kind = ? AND action_key = ?", request.Kind, request.ActionKey).Order("attempt DESC").First(&previous).Error
		if err == nil {
			if previous.TaskID != request.TaskID || previous.RunID != request.RunID {
				return ErrInvalidActionAdmission
			}
			if previous.Attempt >= request.Attempt {
				return ErrActionAlreadyAdmitted
			}
			var unresolved int64
			if err := tx.Model(&model.ActionAdmission{}).Where("kind = ? AND action_key = ? AND not_sent_at IS NULL", request.Kind, request.ActionKey).Count(&unresolved).Error; err != nil {
				return err
			}
			canReplay := request.IdempotentReplay && request.RequestHash != "" && request.RequestHash == previous.RequestHash && (request.Kind == model.ActionExecutionCreate || request.Kind == model.ActionGitCommit || request.Kind == model.ActionGitPush)
			if unresolved != 0 && !canReplay {
				return ErrActionOutcomeUnknown
			}
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		admission = &model.ActionAdmission{AdmissionID: model.NewID("admission"), TaskID: task.TaskID, RunID: request.RunID, Kind: request.Kind, ActionKey: request.ActionKey, Attempt: request.Attempt, LeaseOwner: lease.Owner, LeaseGeneration: lease.Generation, AdmittedAt: now, RequestHash: request.RequestHash}
		return tx.Create(admission).Error
	})
	if err != nil {
		return nil, err
	}
	return admission, nil
}

func (r *ActionAdmissionRepository) NextAttempt(ctx context.Context, kind model.ActionKind, key string) (int, error) {
	var last int
	if err := r.db.WithContext(ctx).Model(&model.ActionAdmission{}).Select("COALESCE(MAX(attempt), 0)").Where("kind = ? AND action_key = ?", kind, key).Scan(&last).Error; err != nil {
		return 0, err
	}
	return last + 1, nil
}

func (r *ActionAdmissionRepository) ConfirmNotSent(ctx context.Context, admission *model.ActionAdmission) error {
	return withLockedTask(ctx, r.db, admission.TaskID, func(tx *gorm.DB, _ *model.Task, now time.Time) error {
		result := tx.Model(&model.ActionAdmission{}).
			Where("admission_id = ? AND task_id = ? AND lease_owner = ? AND lease_generation = ? AND not_sent_at IS NULL", admission.AdmissionID, admission.TaskID, admission.LeaseOwner, admission.LeaseGeneration).
			Update("not_sent_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			var existing model.ActionAdmission
			if err := tx.Where("admission_id = ? AND task_id = ? AND lease_owner = ? AND lease_generation = ?", admission.AdmissionID, admission.TaskID, admission.LeaseOwner, admission.LeaseGeneration).First(&existing).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrActionLeaseLost
				}
				return err
			}
			if existing.NotSentAt == nil {
				return ErrActionLeaseLost
			}
		}
		return nil
	})
}

func ActionAdmissionScope(request ActionAdmissionRequest) (string, error) {
	switch request.Kind {
	case model.ActionExecutionCreate:
		return "execution-create:" + request.ActionKey, nil
	case model.ActionGitCommit, model.ActionGitPush:
		return "developer-delivery:" + request.RunID, nil
	case model.ActionWriteback:
		return "codehost-writeback:" + request.ActionKey, nil
	case model.ActionIMMessage:
		return "effect:" + request.ActionKey, nil
	case model.ActionAutoMerge:
		if request.ActionKey != request.TaskID {
			return "", ErrInvalidActionAdmission
		}
		return "merge-gate:" + request.TaskID, nil
	default:
		return "", ErrInvalidActionAdmission
	}
}
