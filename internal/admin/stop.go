package admin

import (
	"context"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
)

func (s *QueryService) StopStatus(ctx context.Context, target repository.StopTarget) (*model.StopStatus, error) {
	return repository.NewStopRepository(s.db).ReadStatus(ctx, target)
}

func (s *QueryService) CanStop(ctx context.Context, target repository.StopTarget) (bool, error) {
	return repository.NewStopRepository(s.db).CanStop(ctx, target)
}
