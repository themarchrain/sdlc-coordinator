package configuration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

var ErrSnapshotNotFound = errors.New("runtime configuration snapshot not found")

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

func (repository *Repository) Save(ctx context.Context, status SnapshotStatus, source SnapshotSource, cfg RuntimeConfig) (*Snapshot, error) {
	payload, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("encode runtime configuration snapshot: %w", err)
	}
	var saved SnapshotRecord
	err = repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var latestRevision uint64
		if err := tx.Model(&SnapshotRecord{}).Select("COALESCE(MAX(revision), 0)").Scan(&latestRevision).Error; err != nil {
			return fmt.Errorf("load latest runtime configuration revision: %w", err)
		}
		saved = SnapshotRecord{
			Revision:    latestRevision + 1,
			Status:      string(status),
			Source:      string(source),
			PayloadJSON: string(payload),
		}
		if status == StatusReady {
			now := time.Now().UTC()
			saved.AppliedAt = &now
		}
		if err := tx.Create(&saved).Error; err != nil {
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
