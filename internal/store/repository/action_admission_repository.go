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
	TaskID          string
	RunID           string
	Kind            model.ActionKind
	ActionKey       string
	Attempt         int
	LeaseOwner      string
	LeaseGeneration uint64
}

type ActionAdmissionRepository struct{ db *gorm.DB }

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
			if previous.Attempt == request.Attempt {
				return ErrActionAlreadyAdmitted
			}
			return ErrActionOutcomeUnknown
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		admission = &model.ActionAdmission{AdmissionID: model.NewID("admission"), TaskID: task.TaskID, RunID: request.RunID, Kind: request.Kind, ActionKey: request.ActionKey, Attempt: request.Attempt, LeaseOwner: lease.Owner, LeaseGeneration: lease.Generation, AdmittedAt: now}
		return tx.Create(admission).Error
	})
	if err != nil {
		return nil, err
	}
	return admission, nil
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
