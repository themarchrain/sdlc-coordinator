package orchestrator

import (
	"time"

	"sdlc-coordinator/internal/model"
)

type Policy struct {
	LeaseDuration time.Duration
	MaxAttempts   int
}

func DefaultPolicy() Policy {
	return Policy{
		LeaseDuration: 35 * time.Minute,
		MaxAttempts:   3,
	}
}

func (p Policy) normalized() Policy {
	if p.LeaseDuration <= 0 {
		p.LeaseDuration = 35 * time.Minute
	}
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 3
	}
	return p
}

func (p Policy) LeaseUntil(now time.Time) time.Time {
	return now.Add(p.normalized().LeaseDuration)
}

func (p Policy) NextFailureStatus(task *model.Task) model.TaskStatus {
	if task == nil {
		return model.TaskStatusBlocked
	}

	if task.AttemptCount < p.normalized().MaxAttempts {
		return model.TaskStatusRetryPending
	}

	return model.TaskStatusBlocked
}

func (p Policy) RecoveryStatus(task *model.Task, now time.Time) (model.TaskStatus, bool) {
	if task == nil || task.LeaseUntil == nil {
		return "", false
	}

	if task.LeaseUntil.After(now) {
		return "", false
	}

	switch task.Status {
	case model.TaskStatusClaimed, model.TaskStatusRunning:
		if task.AttemptCount < p.normalized().MaxAttempts {
			return model.TaskStatusRetryPending, true
		}
		return model.TaskStatusHumanRequired, true
	default:
		return "", false
	}
}

func (p Policy) Apply(task *model.Task, from, to model.TaskStatus, now time.Time) {
	if task == nil {
		return
	}

	normalized := p.normalized()

	switch to {
	case model.TaskStatusClaimed:
		lease := now.Add(normalized.LeaseDuration)
		task.LeaseUntil = &lease
	case model.TaskStatusRunning:
		lease := now.Add(normalized.LeaseDuration)
		task.LeaseUntil = &lease
		if from != model.TaskStatusRunning {
			task.AttemptCount++
		}
	case model.TaskStatusPending,
		model.TaskStatusWaitingUpstream,
		model.TaskStatusWaitingMerge,
		model.TaskStatusRetryPending,
		model.TaskStatusBlocked,
		model.TaskStatusHumanRequired,
		model.TaskStatusSucceeded:
		task.LeaseUntil = nil
	}
}
