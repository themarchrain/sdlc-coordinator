package orchestrator

import (
	"time"

	"sdlc-coordinator/internal/failure"
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

func (p Policy) AttemptsRemaining(task *model.Task) int {
	if task == nil {
		return 0
	}
	if task.RecoveryGeneration == 0 || task.RetryCycleMaxAttempts == 0 {
		remaining := p.normalized().MaxAttempts - task.AttemptCount
		if remaining < 0 {
			return 0
		}
		return remaining
	}
	if task.RetryCycleMaxAttempts < 0 || task.RetryCycleBaseCount < 0 || task.AttemptCount < task.RetryCycleBaseCount {
		return 0
	}
	remaining := task.RetryCycleMaxAttempts - (task.AttemptCount - task.RetryCycleBaseCount)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (p Policy) NextFailureStatus(task *model.Task) model.TaskStatus {
	if p.AttemptsRemaining(task) > 0 {
		return model.TaskStatusRetryPending
	}
	return model.TaskStatusBlocked
}

func (p Policy) NextExecutionFailureStatus(task *model.Task, info *failure.Info) model.TaskStatus {
	if task != nil && task.RecoveryGeneration > 0 {
		switch failure.Classify(info) {
		case failure.ClassificationPrerequisite, failure.ClassificationFatal:
			return model.TaskStatusBlocked
		}
	}
	return p.NextFailureStatus(task)
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
		if p.AttemptsRemaining(task) > 0 {
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
		model.TaskStatusInterrupted,
		model.TaskStatusSucceeded:
		task.LeaseUntil = nil
	}
}
