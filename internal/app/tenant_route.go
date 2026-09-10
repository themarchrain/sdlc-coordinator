package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/tenantdb"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// repoIndexReconcileInterval 是路由索引对账周期（默认保守 10 分钟；
// 调整只需改此常量）。
const repoIndexReconcileInterval = 10 * time.Minute

// controlPlaneRouteIndex 把控制面仓储适配为 registeredrepo.RouteIndexSync
// （租户作用域 handler 的注册/改仓写穿）。
type controlPlaneRouteIndex struct {
	repo *controlplane.Repository
}

func (c *controlPlaneRouteIndex) Upsert(ctx context.Context, tenantID uint, entry registeredrepo.RouteIndexEntry) error {
	return c.repo.UpsertRepoRouteIndex(ctx, controlplane.RepoRouteIndex{
		Provider: entry.Provider, BaseURL: entry.BaseURL, PathWithNamespace: entry.PathWithNamespace,
		TenantID: tenantID, RepoFingerprint: entry.RepoFingerprint,
	})
}

func (c *controlPlaneRouteIndex) Delete(ctx context.Context, entry registeredrepo.RouteIndexEntry) error {
	return c.repo.DeleteRepoRouteIndex(ctx, controlplane.RepoRouteIdentity{
		Provider: entry.Provider, BaseURL: entry.BaseURL, PathWithNamespace: entry.PathWithNamespace,
	})
}

func (c *controlPlaneRouteIndex) Lookup(ctx context.Context, entry registeredrepo.RouteIndexEntry) (uint, bool, error) {
	row, err := c.repo.GetRepoRouteIndex(ctx, controlplane.RepoRouteIdentity{
		Provider: entry.Provider, BaseURL: entry.BaseURL, PathWithNamespace: entry.PathWithNamespace,
	})
	if errors.Is(err, controlplane.ErrRepoRouteNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return row.TenantID, true, nil
}

// resolveTenantByPath 按 (provider, path) 经控制面路由索引解析租户 id：
// 唯一行 → 该租户；跨 base_url 多行 → error（fail-closed）；未命中 → 0
// （由调用方决定平台库兜底语义）。webhook 验签与事件处理器路由共用。
func resolveTenantByPath(ctx context.Context, index *controlplane.Repository, provider, path string) (uint, error) {
	routes, err := index.GetRepoRouteIndexByPath(ctx, provider, path)
	if err != nil {
		return 0, err
	}
	if len(routes) > 1 {
		// 跨 base_url 同 path 多行：路由歧义，fail-closed。
		return 0, fmt.Errorf("ambiguous repo route for %s/%s", provider, path)
	}
	if len(routes) == 1 {
		return routes[0].TenantID, nil
	}
	return 0, nil
}

// tenantRouteSecretLookup 是 webhook 验签的租户路由解析：
// 最小身份 (provider, path) → 控制面索引 → 打开租户库取仓行 secret。
// 错误语义：err 非 nil = 基础设施抖动（索引查询失败/租户库句柄不可用/
// 仓行查询故障），调用方回 5xx 让代码托管重试；空串 = 未注册
// （fail-closed 401）。索引未命中（tenantID=0）回退平台库仓行（存量
// 部署过渡期），其余情形不再回落平台库——索引已指认租户却验平台副本，
// 等于把事件放进错误的库。
type tenantRouteSecretLookup struct {
	index    *controlplane.Repository
	resolver tenantdb.Resolver
	platform *registeredrepo.Store
}

func (l *tenantRouteSecretLookup) WebhookSecretByPath(ctx context.Context, provider, path string) (string, error) {
	tenantID, err := resolveTenantByPath(ctx, l.index, provider, path)
	if err != nil {
		return "", err
	}
	if tenantID != 0 {
		store, handleErr := l.resolver.Handle(ctx, tenantID)
		if handleErr != nil {
			return "", fmt.Errorf("tenant store unavailable for tenant %d: %w", tenantID, handleErr)
		}
		repo, getErr := registeredrepo.NewStore(store.DB).GetByPath(ctx, provider, path)
		if getErr == nil {
			return strings.TrimSpace(repo.WebhookSecret), nil
		}
		if errors.Is(getErr, gorm.ErrRecordNotFound) {
			// 租户库无此仓行（索引漂移）：按未注册拒绝，等对账清理索引。
			return "", nil
		}
		return "", fmt.Errorf("tenant repo lookup failed for tenant %d: %w", tenantID, getErr)
	}
	repo, err := l.platform.GetByPath(ctx, provider, path)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("platform repo lookup failed: %w", err)
	}
	return strings.TrimSpace(repo.WebhookSecret), nil
}

// reconcileRepoRouteIndex 以租户库为权威对账路由索引：租户库中
// 的仓决定索引应有的行；指派错租户的行修正（先删后建，Upsert 拒绝跨
// 租户换主），同租户的指纹漂移原地更新；索引多出的行删除。停用组织
// 的行冻结保留（身份认领持续占用，防停用窗口被抢注；其数据面已由
// 引擎层状态门切断）。同身份被多个租户同时声称时视为冲突：保留现状
// 不翻转、不删除，告警交由人工裁决。
func reconcileRepoRouteIndex(ctx context.Context, repo *controlplane.Repository, resolver tenantdb.Resolver, logger *zap.Logger) error {
	tenants, err := repo.ListTenantSummaries(ctx)
	if err != nil {
		return fmt.Errorf("list tenants: %w", err)
	}
	want := make(map[string]controlplane.RepoRouteIndex)
	claimant := make(map[string]uint)
	conflicted := make(map[string]bool)
	frozen := make(map[uint]bool)
	for _, tenant := range tenants {
		if tenant.Status != controlplane.TenantStatusActive {
			// 停用组织的仓不参与重建，其既有行冻结保留（见删除循环）。
			frozen[tenant.ID] = true
			continue
		}
		store, err := resolver.Handle(ctx, tenant.ID)
		if err != nil {
			continue
		}
		rows, err := registeredrepo.NewStore(store.DB).List(ctx)
		if err != nil {
			continue
		}
		for i := range rows {
			row := &rows[i]
			key := strings.Join([]string{row.Provider, row.BaseURL, row.PathWithNamespace}, "\x00")
			if prev, ok := claimant[key]; ok && prev != tenant.ID {
				conflicted[key] = true
				continue
			}
			claimant[key] = tenant.ID
			want[key] = controlplane.RepoRouteIndex{
				Provider: row.Provider, BaseURL: row.BaseURL, PathWithNamespace: row.PathWithNamespace,
				TenantID: tenant.ID, RepoFingerprint: row.RepositoryID,
			}
		}
	}
	for key := range conflicted {
		delete(want, key)
		if logger != nil {
			logger.Warn("repo route identity claimed by multiple tenants; keeping current assignment",
				zap.String("route", strings.ReplaceAll(key, "\x00", " ")))
		}
	}
	existing, err := repo.ListRepoRouteIndex(ctx, 0)
	if err != nil {
		return fmt.Errorf("list route index: %w", err)
	}
	existingKeys := make(map[string]controlplane.RepoRouteIndex, len(existing))
	for _, row := range existing {
		key := strings.Join([]string{row.Provider, row.BaseURL, row.PathWithNamespace}, "\x00")
		existingKeys[key] = row
	}
	for key, row := range want {
		if current, ok := existingKeys[key]; !ok || current.TenantID != row.TenantID || current.RepoFingerprint != row.RepoFingerprint {
			if ok && current.TenantID != row.TenantID {
				if frozen[current.TenantID] {
					// 既有行属于冻结组织：停用不等于放弃身份，按冲突保留
					// 现状，不翻给活跃认领方。
					if logger != nil {
						logger.Warn("repo route identity of a disabled tenant claimed by another tenant; keeping current assignment",
							zap.String("route", strings.ReplaceAll(key, "\x00", " ")))
					}
					continue
				}
				// 指派错租户的漂移修正：先删旧行再建（Upsert 拒绝跨租户
				// 换主）。同租户的指纹漂移不需要删，Upsert 原地更新指纹。
				if err := repo.DeleteRepoRouteIndex(ctx, controlplane.RepoRouteIdentity{
					Provider: row.Provider, BaseURL: row.BaseURL, PathWithNamespace: row.PathWithNamespace,
				}); err != nil {
					return fmt.Errorf("reconcile route index delete: %w", err)
				}
			}
			if err := repo.UpsertRepoRouteIndex(ctx, row); err != nil {
				if errors.Is(err, controlplane.ErrRepoRouteIdentityClaimed) {
					if logger != nil {
						logger.Warn("repo route identity claim lost during reconcile",
							zap.String("route", strings.ReplaceAll(key, "\x00", " ")))
					}
					continue
				}
				return fmt.Errorf("reconcile route index upsert: %w", err)
			}
		}
	}
	for key, row := range existingKeys {
		if _, ok := want[key]; ok || conflicted[key] {
			continue
		}
		if frozen[row.TenantID] {
			// 冻结组织的行保留：恢复后立即可路由，停用窗口内身份持续占用。
			continue
		}
		if err := repo.DeleteRepoRouteIndex(ctx, controlplane.RepoRouteIdentity{
			Provider: row.Provider, BaseURL: row.BaseURL, PathWithNamespace: row.PathWithNamespace,
		}); err != nil {
			return fmt.Errorf("reconcile route index delete: %w", err)
		}
	}
	return nil
}

// runRepoIndexReconciler 周期性对账（低频兜底；写穿为主通道，漂移源包括
// 写穿失败与迁移窗口）。
func (a *App) runRepoIndexReconciler(ctx context.Context) {
	if a.ControlPlane == nil || a.TenantStores == nil {
		return
	}
	ticker := time.NewTicker(repoIndexReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reconcileCtx, cancel := context.WithTimeout(ctx, time.Minute)
			if err := reconcileRepoRouteIndex(reconcileCtx, a.ControlPlane.Repository(), a.TenantStores, a.Logger); err != nil && a.Logger != nil {
				a.Logger.Warn("repo route index reconciliation failed", zap.Error(err))
			}
			cancel()
		}
	}
}
