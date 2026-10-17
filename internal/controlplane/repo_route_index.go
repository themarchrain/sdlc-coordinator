package controlplane

import (
	"context"
	"errors"
	"fmt"
	"time"

	"sdlc-coordinator/internal/store"

	"gorm.io/gorm"
)

// RepoRouteIndex 是 webhook 租户路由索引：控制面持只读索引，
// 注册/改仓写穿（先索引后仓行，失败补偿删索引），低频对账以租户库为权威。
// 索引只路由不鉴权：secret 留租户库仓行。
type RepoRouteIndex struct {
	ID                uint   `gorm:"primaryKey"`
	Provider          string `gorm:"not null;uniqueIndex:idx_repo_route_identity;size:64"`
	BaseURL           string `gorm:"not null;uniqueIndex:idx_repo_route_identity;size:255"`
	PathWithNamespace string `gorm:"not null;uniqueIndex:idx_repo_route_identity;size:255"`
	TenantID          uint   `gorm:"not null;index"`
	RepoFingerprint   string `gorm:"not null;default:''"`
	Version           uint64 `gorm:"not null;default:1"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (RepoRouteIndex) TableName() string { return "repo_route_index" }

// RepoRouteIdentity 是索引的路由身份键（与注册仓行的平台身份锚点一致）。
type RepoRouteIdentity struct {
	Provider          string
	BaseURL           string
	PathWithNamespace string
}

var ErrRepoRouteNotFound = errors.New("repo route not found")

// ErrRepoRouteIdentityClaimed 表示路由身份已被其他租户占用。索引行是
// webhook 的租户路由锚点，静默换主等于把占用方的流量改道或打死，因此
// 跨租户改写一律拒绝；确属漂移的修正由对账按快照版本条件更新。
var ErrRepoRouteIdentityClaimed = errors.New("repo route identity claimed by another tenant")

func (repository *Repository) UpsertRepoRouteIndex(ctx context.Context, row RepoRouteIndex) error {
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing RepoRouteIndex
		err := tx.Where("provider = ? AND base_url = ? AND path_with_namespace = ?",
			row.Provider, row.BaseURL, row.PathWithNamespace).First(&existing).Error
		if err == nil {
			if existing.TenantID != row.TenantID {
				return ErrRepoRouteIdentityClaimed
			}
			row.ID = existing.ID
			row.CreatedAt = existing.CreatedAt
			result := tx.Model(&RepoRouteIndex{}).Where("id = ? AND tenant_id = ?", existing.ID, row.TenantID).Updates(map[string]any{
				"repo_fingerprint": row.RepoFingerprint,
				"version":          gorm.Expr("version + 1"),
			})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrRepoRouteIdentityClaimed
			}
			return nil
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&row).Error
		}
		return err
	})
}

func (repository *Repository) UpsertRepoRouteIndexGuarded(ctx context.Context, row RepoRouteIndex, expected *RepoRouteIndex) (bool, error) {
	if expected == nil {
		result := repository.db.WithContext(ctx).Create(&row)
		if store.IsDuplicateKey(result.Error) {
			return false, nil
		}
		return result.RowsAffected == 1, result.Error
	}
	result := repository.db.WithContext(ctx).Model(&RepoRouteIndex{}).
		Where("id = ? AND version = ?", expected.ID, expected.Version).
		Updates(map[string]any{
			"tenant_id": row.TenantID, "repo_fingerprint": row.RepoFingerprint,
			"version": gorm.Expr("version + 1"),
		})
	return result.RowsAffected == 1, result.Error
}

func (repository *Repository) DeleteRepoRouteIndexGuarded(ctx context.Context, expected RepoRouteIndex) (bool, error) {
	result := repository.db.WithContext(ctx).
		Where("id = ? AND version = ?", expected.ID, expected.Version).
		Delete(&RepoRouteIndex{})
	return result.RowsAffected == 1, result.Error
}

func (repository *Repository) GetRepoRouteIndex(ctx context.Context, identity RepoRouteIdentity) (*RepoRouteIndex, error) {
	var route RepoRouteIndex
	err := repository.db.WithContext(ctx).
		Where("provider = ? AND base_url = ? AND path_with_namespace = ?",
			identity.Provider, identity.BaseURL, identity.PathWithNamespace).
		First(&route).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRepoRouteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get repo route index: %w", err)
	}
	return &route, nil
}

// GetRepoRouteIndexByPath 按 webhook 最小身份（provider+path）解析；跨
// base_url 同 path 多行时视为歧义（fail-closed，由调用方拒绝）。
func (repository *Repository) GetRepoRouteIndexByPath(ctx context.Context, provider, path string) ([]RepoRouteIndex, error) {
	var routes []RepoRouteIndex
	if err := repository.db.WithContext(ctx).
		Where("provider = ? AND path_with_namespace = ?", provider, path).
		Find(&routes).Error; err != nil {
		return nil, fmt.Errorf("resolve repo route by path: %w", err)
	}
	return routes, nil
}

func (repository *Repository) ListRepoRouteIndex(ctx context.Context, tenantID uint) ([]RepoRouteIndex, error) {
	var routes []RepoRouteIndex
	query := repository.db.WithContext(ctx).Order("path_with_namespace ASC")
	if tenantID != 0 {
		query = query.Where("tenant_id = ?", tenantID)
	}
	if err := query.Find(&routes).Error; err != nil {
		return nil, fmt.Errorf("list repo route index: %w", err)
	}
	return routes, nil
}

func (repository *Repository) DeleteRepoRouteIndex(ctx context.Context, identity RepoRouteIdentity) error {
	return repository.db.WithContext(ctx).
		Where("provider = ? AND base_url = ? AND path_with_namespace = ?",
			identity.Provider, identity.BaseURL, identity.PathWithNamespace).
		Delete(&RepoRouteIndex{}).Error
}
