package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/tenantdb"

	"go.uber.org/zap"
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

// tenantRouteSecretLookup 是 webhook 验签的租户路由解析：
// 最小身份 (provider, path) → 控制面索引 → 打开租户库取仓行 secret。
// 索引未命中回退平台库仓行（存量部署过渡期：平台库仓行仍存活）；
// 两处皆未命中返回错误，由调用方按既有语义 fail-closed（401）。
type tenantRouteSecretLookup struct {
	index    *controlplane.Repository
	resolver tenantdb.Resolver
	platform *registeredrepo.Store
}

func (l *tenantRouteSecretLookup) WebhookSecretByPath(ctx context.Context, provider, path string) (string, error) {
	routes, err := l.index.GetRepoRouteIndexByPath(ctx, provider, path)
	if err != nil {
		return "", err
	}
	if len(routes) > 1 {
		// 跨 base_url 同 path 多行：路由歧义，fail-closed。
		return "", fmt.Errorf("ambiguous repo route for %s/%s", provider, path)
	}
	if len(routes) == 1 {
		store, handleErr := l.resolver.Handle(ctx, routes[0].TenantID)
		if handleErr == nil {
			repo, getErr := registeredrepo.NewStore(store.DB).GetByPath(ctx, provider, path)
			if getErr == nil {
				return strings.TrimSpace(repo.WebhookSecret), nil
			}
		}
		// 租户库未命中或句柄不可用 → 落到平台库兜底。
	}
	repo, err := l.platform.GetByPath(ctx, provider, path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(repo.WebhookSecret), nil
}

// reconcileRepoRouteIndex 以租户库为权威对账路由索引：租户库中
// 的仓决定索引应有的行；指派错租户的行修正，索引多出的行删除。
func reconcileRepoRouteIndex(ctx context.Context, repo *controlplane.Repository, resolver tenantdb.Resolver) error {
	tenants, err := repo.ListTenantSummaries(ctx)
	if err != nil {
		return fmt.Errorf("list tenants: %w", err)
	}
	want := make(map[string]controlplane.RepoRouteIndex)
	for _, tenant := range tenants {
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
			want[key] = controlplane.RepoRouteIndex{
				Provider: row.Provider, BaseURL: row.BaseURL, PathWithNamespace: row.PathWithNamespace,
				TenantID: tenant.ID, RepoFingerprint: row.RepositoryID,
			}
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
			if err := repo.UpsertRepoRouteIndex(ctx, row); err != nil {
				return fmt.Errorf("reconcile route index upsert: %w", err)
			}
		}
	}
	for key, row := range existingKeys {
		if _, ok := want[key]; !ok {
			if err := repo.DeleteRepoRouteIndex(ctx, controlplane.RepoRouteIdentity{
				Provider: row.Provider, BaseURL: row.BaseURL, PathWithNamespace: row.PathWithNamespace,
			}); err != nil {
				return fmt.Errorf("reconcile route index delete: %w", err)
			}
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
			if err := reconcileRepoRouteIndex(reconcileCtx, a.ControlPlane.Repository(), a.TenantStores); err != nil && a.Logger != nil {
				a.Logger.Warn("repo route index reconciliation failed", zap.Error(err))
			}
			cancel()
		}
	}
}
