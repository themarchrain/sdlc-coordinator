package controlplane

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// RepoRouteIndex 是 webhook 租户路由索引：控制面持只读索引，
// 注册/改仓写穿（先索引后仓行，失败补偿删索引），低频对账以租户库为权威。
// 索引只路由不鉴权：secret 留租户库仓行。
type RepoRouteIndex struct {
	ID                uint   `gorm:"primaryKey"`
	Provider          string `gorm:"not null;uniqueIndex:idx_repo_route_identity"`
	BaseURL           string `gorm:"not null;uniqueIndex:idx_repo_route_identity;type:text"`
	PathWithNamespace string `gorm:"not null;uniqueIndex:idx_repo_route_identity;type:text"`
	TenantID          uint   `gorm:"not null;index"`
	RepoFingerprint   string `gorm:"not null;default:''"`
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

func (repository *Repository) UpsertRepoRouteIndex(ctx context.Context, row RepoRouteIndex) error {
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing RepoRouteIndex
		err := tx.Where("provider = ? AND base_url = ? AND path_with_namespace = ?",
			row.Provider, row.BaseURL, row.PathWithNamespace).First(&existing).Error
		if err == nil {
			row.ID = existing.ID
			row.CreatedAt = existing.CreatedAt
			return tx.Model(&RepoRouteIndex{}).Where("id = ?", existing.ID).Updates(map[string]any{
				"tenant_id": row.TenantID, "repo_fingerprint": row.RepoFingerprint,
			}).Error
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&row).Error
		}
		return err
	})
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
