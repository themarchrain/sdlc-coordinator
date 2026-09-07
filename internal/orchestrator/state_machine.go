package orchestrator

import (
	"errors"
	"fmt"

	"sdlc-coordinator/internal/model"
)

var (
	ErrNilTask           = errors.New("task is nil")
	ErrInvalidTransition = errors.New("invalid task transition")
)

type StateMachine struct {
	allowed map[model.TaskStatus]map[model.TaskStatus]struct{}
}

func NewStateMachine() *StateMachine {
	return &StateMachine{
		allowed: map[model.TaskStatus]map[model.TaskStatus]struct{}{
			model.TaskStatusPending: {
				model.TaskStatusClaimed: {},
			},
			model.TaskStatusClaimed: {
				model.TaskStatusRunning:       {},
				model.TaskStatusRetryPending:  {},
				model.TaskStatusHumanRequired: {},
			},
			model.TaskStatusRunning: {
				model.TaskStatusFinalizing:      {},
				model.TaskStatusSucceeded:       {},
				model.TaskStatusWaitingUpstream: {},
				model.TaskStatusWaitingMerge:    {},
				model.TaskStatusRetryPending:    {},
				model.TaskStatusBlocked:         {},
				model.TaskStatusHumanRequired:   {},
			},
			model.TaskStatusFinalizing: {
				model.TaskStatusSucceeded:       {},
				model.TaskStatusWaitingUpstream: {},
				model.TaskStatusWaitingMerge:    {},
				model.TaskStatusRetryPending:    {},
				model.TaskStatusBlocked:         {},
				model.TaskStatusHumanRequired:   {},
			},
			model.TaskStatusWaitingUpstream: {
				model.TaskStatusPending: {},
			},
			model.TaskStatusWaitingMerge: {},
			model.TaskStatusRetryPending: {
				model.TaskStatusClaimed: {},
			},
			model.TaskStatusBlocked: {},
			model.TaskStatusHumanRequired: {
				model.TaskStatusPending: {},
			},
			model.TaskStatusSucceeded: {},
		},
	}
}

func (s *StateMachine) CanTransition(from, to model.TaskStatus) bool {
	if from == to {
		return true
	}

	nextStates, ok := s.allowed[from]
	if !ok {
		return false
	}

	_, ok = nextStates[to]
	return ok
}

func (s *StateMachine) Transition(task *model.Task, next model.TaskStatus) error {
	if task == nil {
		return ErrNilTask
	}

	if !s.CanTransition(task.Status, next) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, task.Status, next)
	}

	task.Status = next
	return nil
}
