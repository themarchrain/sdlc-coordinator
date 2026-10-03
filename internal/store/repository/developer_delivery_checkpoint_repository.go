package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

// ErrDeliveryCheckpointConflict 表示检查点写入与既有受信状态冲突:基线五元组
// 不一致,或该 run 已记录了另一笔交付提交。冲突必须 fail-closed,不允许覆盖。
var ErrDeliveryCheckpointConflict = errors.New("developer_delivery_checkpoint_conflict")

// DeveloperDeliveryCheckpointRepository 持有 Developer 交付检查点。检查点是
// 提交归属的唯一受信凭据:基线在 prepare 后钉住,提交 SHA 在 push 前以条件
// 更新写入;每 run 至多一笔可交付提交。
type DeveloperDeliveryCheckpointRepository struct {
	db *gorm.DB
}

func NewDeveloperDeliveryCheckpointRepository(db *gorm.DB) *DeveloperDeliveryCheckpointRepository {
	return &DeveloperDeliveryCheckpointRepository{db: db}
}

// RecordBaseline 每 run 幂等钉基线:行不存在则创建;已存在且任务/类型/仓/
// 分支/基线五元组完全一致视为幂等成功(重启复用路径),任一不同即冲突。
func (r *DeveloperDeliveryCheckpointRepository) RecordBaseline(ctx context.Context, record *model.DeveloperDeliveryCheckpoint) error {
	if record == nil {
		return errors.New("developer delivery checkpoint record is required")
	}
	record.BaselineSHA = strings.ToLower(strings.TrimSpace(record.BaselineSHA))
	if err := r.db.WithContext(ctx).Create(record).Error; err == nil {
		return nil
	}
	existing, err := r.Load(ctx, record.RunID)
	if err != nil {
		return fmt.Errorf("load developer delivery checkpoint %s after insert conflict: %w", record.RunID, err)
	}
	if existing == nil {
		return fmt.Errorf("load developer delivery checkpoint %s after insert conflict: record missing", record.RunID)
	}
	if existing.TaskID != record.TaskID || existing.TaskType != record.TaskType ||
		existing.RepositoryURL != record.RepositoryURL || existing.DeliveryBranch != record.DeliveryBranch ||
		!strings.EqualFold(existing.BaselineSHA, record.BaselineSHA) {
		return fmt.Errorf("%w: run %s already pinned a different delivery baseline", ErrDeliveryCheckpointConflict, record.RunID)
	}
	return nil
}

// Load 返回 run 的检查点;不存在返回 nil,nil。
func (r *DeveloperDeliveryCheckpointRepository) Load(ctx context.Context, runID string) (*model.DeveloperDeliveryCheckpoint, error) {
	var record model.DeveloperDeliveryCheckpoint
	if err := r.db.WithContext(ctx).Where("run_id = ?", runID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("load developer delivery checkpoint %s: %w", runID, err)
	}
	return &record, nil
}

// RecordCommit 以条件更新写入交付 SHA:只允许从空翻转一次。同 SHA 重复写入
// 幂等成功(重复结果回传),不同 SHA 冲突(同 run 第二笔"合法"提交不存在)。
func (r *DeveloperDeliveryCheckpointRepository) RecordCommit(ctx context.Context, runID, commitSHA string) error {
	trimmed := strings.ToLower(strings.TrimSpace(commitSHA))
	if trimmed == "" {
		return errors.New("developer delivery commit sha is required")
	}
	result := r.db.WithContext(ctx).Model(&model.DeveloperDeliveryCheckpoint{}).
		Where("run_id = ? AND (commit_sha IS NULL OR commit_sha = '')", runID).
		Updates(map[string]any{"commit_sha": trimmed, "updated_at": time.Now()})
	if result.Error != nil {
		return fmt.Errorf("record developer delivery commit for run %s: %w", runID, result.Error)
	}
	if result.RowsAffected == 1 {
		return nil
	}
	existing, err := r.Load(ctx, runID)
	if err != nil {
		return fmt.Errorf("load developer delivery checkpoint %s after conditional update: %w", runID, err)
	}
	if existing == nil {
		return fmt.Errorf("record developer delivery commit for run %s: %w", runID, ErrNotFound)
	}
	if existing.CommitSHA == trimmed {
		return nil
	}
	return fmt.Errorf("%w: run %s already recorded delivery commit %s", ErrDeliveryCheckpointConflict, runID, existing.CommitSHA)
}
