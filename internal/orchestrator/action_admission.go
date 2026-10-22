package orchestrator

import (
	"context"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
)

type ActionAdmissionRequest = repository.ActionAdmissionRequest

type ActionAdmissionPort interface {
	AdmitAction(context.Context, ActionAdmissionRequest) (*model.ActionAdmission, error)
}

func WithActionAdmissions(admissions ActionAdmissionPort) Option {
	return func(o *Orchestrator) { o.actionAdmissions = admissions }
}
