package repository

import (
	"context"
	"fmt"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

type DependencyRepository struct {
	db *gorm.DB
}

func NewDependencyRepository(db *gorm.DB) *DependencyRepository {
	return &DependencyRepository{db: db}
}

func (r *DependencyRepository) Create(ctx context.Context, dependency *model.Dependency) error {
	if err := r.db.WithContext(ctx).Create(dependency).Error; err != nil {
		return fmt.Errorf("create dependency: %w", err)
	}
	return nil
}

func (r *DependencyRepository) Update(ctx context.Context, dependency *model.Dependency) error {
	if err := r.db.WithContext(ctx).Save(dependency).Error; err != nil {
		return fmt.Errorf("update dependency: %w", err)
	}
	return nil
}

func (r *DependencyRepository) ListByParentTaskID(ctx context.Context, parentTaskID string) ([]model.Dependency, error) {
	var dependencies []model.Dependency
	if err := r.db.WithContext(ctx).
		Where("parent_task_id = ?", parentTaskID).
		Order("id ASC").
		Find(&dependencies).Error; err != nil {
		return nil, fmt.Errorf("list dependencies by parent task id: %w", err)
	}
	return dependencies, nil
}

func (r *DependencyRepository) ListByChildTaskID(ctx context.Context, childTaskID string) ([]model.Dependency, error) {
	var dependencies []model.Dependency
	if err := r.db.WithContext(ctx).
		Where("child_task_id = ?", childTaskID).
		Order("id ASC").
		Find(&dependencies).Error; err != nil {
		return nil, fmt.Errorf("list dependencies by child task id: %w", err)
	}
	return dependencies, nil
}

func (r *DependencyRepository) UpsertByIdentity(ctx context.Context, dependency *model.Dependency) error {
	var existing model.Dependency
	err := r.db.WithContext(ctx).
		Where("parent_task_id = ? AND child_task_id = ? AND relation = ?", dependency.ParentTaskID, dependency.ChildTaskID, dependency.Relation).
		First(&existing).Error
	if err == nil {
		existing.Status = nextDependencyStatus(existing.Status, dependency.Status)
		existing.MetadataJSON = dependency.MetadataJSON
		if err := r.db.WithContext(ctx).Save(&existing).Error; err != nil {
			return fmt.Errorf("update dependency by identity: %w", err)
		}
		dependency.ID = existing.ID
		dependency.DependencyID = existing.DependencyID
		return nil
	}
	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("get dependency by identity: %w", err)
	}

	if err := r.db.WithContext(ctx).Create(dependency).Error; err != nil {
		return fmt.Errorf("create dependency by identity: %w", err)
	}
	return nil
}

func nextDependencyStatus(existing, incoming model.DependencyStatus) model.DependencyStatus {
	if isTerminalDependencyStatus(existing) {
		return existing
	}
	return incoming
}

func isTerminalDependencyStatus(status model.DependencyStatus) bool {
	switch status {
	case model.DependencyStatusBlocked,
		model.DependencyStatusResolved:
		return true
	default:
		return false
	}
}
