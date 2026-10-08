package repository

import (
	"context"
	"fmt"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"

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

// UpsertByIdentity 按 (task, run, kind, name) 身份写入或更新 artifact。
// 先读后插在两实例并发首写同一身份时会双双插入,败者撞唯一索引:这里在
// 插入失败时回读既有行改为更新,使并发首写收敛为一行(胜负与内容无损,
// 两侧 metadata 一致)。
func (r *ArtifactRepository) UpsertByIdentity(ctx context.Context, artifact *model.Artifact) error {
	if err := r.upsertByIdentityOnce(ctx, artifact); err != nil {
		if !store.IsDuplicateKey(err) {
			return err
		}
		// 并发首写败者:按身份回读后改为更新既有行。
		var existing model.Artifact
		err := r.db.WithContext(ctx).
			Where("task_id = ? AND run_id = ? AND kind = ? AND name = ?", artifact.TaskID, artifact.RunID, artifact.Kind, artifact.Name).
			First(&existing).Error
		if err != nil {
			return fmt.Errorf("reload artifact by identity after duplicate key: %w", err)
		}
		existing.URL = artifact.URL
		existing.MetadataJSON = artifact.MetadataJSON
		if err := r.db.WithContext(ctx).Save(&existing).Error; err != nil {
			return fmt.Errorf("update artifact by identity after duplicate key: %w", err)
		}
		artifact.ID = existing.ID
		artifact.ArtifactID = existing.ArtifactID
		return nil
	}
	return nil
}

func (r *ArtifactRepository) upsertByIdentityOnce(ctx context.Context, artifact *model.Artifact) error {
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

// ListByTaskAndName 按 (task, kind, name) 读取 artifact(id 升序),供验证
// 处方复放开发记录的测试命令等只读消费。
func (r *ArtifactRepository) ListByTaskAndName(ctx context.Context, taskID, kind, name string) ([]model.Artifact, error) {
	var artifacts []model.Artifact
	if err := r.db.WithContext(ctx).
		Where("task_id = ? AND kind = ? AND name = ?", taskID, kind, name).
		Order("id ASC").
		Find(&artifacts).Error; err != nil {
		return nil, fmt.Errorf("list artifacts by task and name: %w", err)
	}
	return artifacts, nil
}
