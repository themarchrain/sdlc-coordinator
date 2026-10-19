package configuration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"sdlc-coordinator/internal/store"

	"gorm.io/gorm"
)

var (
	ErrSnapshotNotFound = errors.New("runtime configuration snapshot not found")
	// ErrRevisionConflict 表示保存所基于的 revision 已过期：保存窗口内其他
	// 实例已写入新快照（或首次保存被并发抢先），本提交被拒绝，调用方刷新
	// 后重试；绝不基于旧快照发新 revision 覆盖他人字段。
	ErrRevisionConflict = errors.New("runtime configuration revision conflict")
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (repository *Repository) Migrate() error {
	if err := repository.db.AutoMigrate(&SnapshotRecord{}); err != nil {
		return fmt.Errorf("migrate runtime configuration snapshots: %w", err)
	}
	return nil
}

func (repository *Repository) Latest(ctx context.Context) (*Snapshot, error) {
	var record SnapshotRecord
	if err := repository.db.WithContext(ctx).Order("revision DESC").First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSnapshotNotFound
		}
		return nil, fmt.Errorf("load latest runtime configuration snapshot: %w", err)
	}
	return decodeSnapshotRecord(record)
}

// Save 追加一个新快照（读取库内最新 revision 后受保护写入）。多实例下由
// Latest+SaveGuarded 两步构成：期间若有并发写入，返回 ErrRevisionConflict。
func (repository *Repository) Save(ctx context.Context, status SnapshotStatus, source SnapshotSource, cfg RuntimeConfig) (*Snapshot, error) {
	expected := uint64(0)
	latest, err := repository.Latest(ctx)
	switch {
	case err == nil:
		expected = latest.Revision
	case errors.Is(err, ErrSnapshotNotFound):
	default:
		return nil, err
	}
	return repository.SaveGuarded(ctx, status, source, cfg, expected)
}

// SaveGuarded 以期望 revision 为条件写入新快照：仅当库内当前最新 revision
// 仍等于 expectedRevision 时才落 revision+1。调用方先读最新快照、以其为
// 合并基线，再以该 revision 作期望写入——多实例下并发保存由此收敛为一个
// 赢家，败者收到 ErrRevisionConflict 而不是基于旧快照覆盖新字段。revision
// 列上的唯一索引是最终防线：读检查与插入之间的极端竞争由重复键映射为
// 同一冲突哨兵。
func (repository *Repository) SaveGuarded(ctx context.Context, status SnapshotStatus, source SnapshotSource, cfg RuntimeConfig, expectedRevision uint64) (*Snapshot, error) {
	payload, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("encode runtime configuration snapshot: %w", err)
	}
	var saved SnapshotRecord
	err = repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var latest SnapshotRecord
		findErr := tx.Order("revision DESC").First(&latest).Error
		switch {
		case errors.Is(findErr, gorm.ErrRecordNotFound):
			if expectedRevision != 0 {
				return ErrRevisionConflict
			}
		case findErr != nil:
			return fmt.Errorf("load latest runtime configuration revision: %w", findErr)
		default:
			if latest.Revision != expectedRevision {
				return ErrRevisionConflict
			}
		}
		saved = SnapshotRecord{
			Revision:    expectedRevision + 1,
			Status:      string(status),
			Source:      string(source),
			PayloadJSON: string(payload),
		}
		if status == StatusReady {
			now := time.Now().UTC()
			saved.AppliedAt = &now
		}
		if err := tx.Create(&saved).Error; err != nil {
			if store.IsDuplicateKey(err) {
				return ErrRevisionConflict
			}
			return fmt.Errorf("create runtime configuration snapshot: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return decodeSnapshotRecord(saved)
}

func (repository *Repository) MarkApplied(ctx context.Context, revision uint64, appliedAt time.Time) error {
	result := repository.db.WithContext(ctx).Model(&SnapshotRecord{}).
		Where("revision = ?", revision).
		Updates(map[string]any{
			"status":     string(StatusReady),
			"applied_at": appliedAt.UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("mark runtime configuration applied: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrSnapshotNotFound
	}
	return nil
}

func decodeSnapshotRecord(record SnapshotRecord) (*Snapshot, error) {
	var cfg RuntimeConfig
	if err := json.Unmarshal([]byte(record.PayloadJSON), &cfg); err != nil {
		return nil, fmt.Errorf("decode runtime configuration snapshot revision %d: %w", record.Revision, err)
	}
	return &Snapshot{
		Revision:  record.Revision,
		Status:    SnapshotStatus(record.Status),
		Source:    SnapshotSource(record.Source),
		Config:    cfg,
		AppliedAt: record.AppliedAt,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}, nil
}
