package repository

import (
	"context"
	"fmt"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

type ArtifactRepository struct {
	db *gorm.DB
}

func NewArtifactRepository(db *gorm.DB) *ArtifactRepository {
	return &ArtifactRepository{db: db}
}

func (r *ArtifactRepository) Create(ctx context.Context, artifact *model.Artifact) error {
	if err := r.db.WithContext(ctx).Create(artifact).Error; err != nil {
		return fmt.Errorf("create artifact: %w", err)
	}
	return nil
}

func (r *ArtifactRepository) ListByTaskID(ctx context.Context, taskID string) ([]model.Artifact, error) {
	var artifacts []model.Artifact
	if err := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("id ASC").
		Find(&artifacts).Error; err != nil {
		return nil, fmt.Errorf("list artifacts by task id: %w", err)
	}
	return artifacts, nil
}

func (r *ArtifactRepository) ListByRunID(ctx context.Context, runID string) ([]model.Artifact, error) {
	var artifacts []model.Artifact
	if err := r.db.WithContext(ctx).
		Where("run_id = ?", runID).
		Order("id ASC").
		Find(&artifacts).Error; err != nil {
		return nil, fmt.Errorf("list artifacts by run id: %w", err)
	}
	return artifacts, nil
}

func (r *ArtifactRepository) UpsertByIdentity(ctx context.Context, artifact *model.Artifact) error {
	var existing model.Artifact
	err := r.db.WithContext(ctx).
		Where("task_id = ? AND run_id = ? AND kind = ? AND name = ?", artifact.TaskID, artifact.RunID, artifact.Kind, artifact.Name).
		First(&existing).Error
	if err == nil {
		existing.URL = artifact.URL
		existing.MetadataJSON = artifact.MetadataJSON
		if err := r.db.WithContext(ctx).Save(&existing).Error; err != nil {
			return fmt.Errorf("update artifact by identity: %w", err)
		}
		artifact.ID = existing.ID
		artifact.ArtifactID = existing.ArtifactID
		return nil
	}
	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("get artifact by identity: %w", err)
	}

	if err := r.db.WithContext(ctx).Create(artifact).Error; err != nil {
		return fmt.Errorf("create artifact by identity: %w", err)
	}
	return nil
}
