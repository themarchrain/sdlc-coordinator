package controlplane

import (
	"context"

	"gorm.io/gorm"
)

type repoCtxKey struct{}

func withRepo(ctx context.Context, repo *Repository) context.Context {
	return context.WithValue(ctx, repoCtxKey{}, repo)
}

func (service *Service) repo(ctx context.Context) *Repository {
	if repo, ok := ctx.Value(repoCtxKey{}).(*Repository); ok && repo != nil {
		return repo
	}
	return service.repository
}

// InTx 在控制面库上开事务；嵌套调用走 gorm savepoint。
func (repository *Repository) InTx(ctx context.Context, fn func(*Repository) error) error {
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Repository{db: tx})
	})
}

func (service *Service) inTx(ctx context.Context, fn func(context.Context) error) error {
	return service.repo(ctx).InTx(ctx, func(tx *Repository) error {
		return fn(withRepo(ctx, tx))
	})
}

func inTxValue[T any](service *Service, ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	var result T
	err := service.inTx(ctx, func(ctx context.Context) error {
		var callErr error
		result, callErr = fn(ctx)
		return callErr
	})
	return result, err
}
