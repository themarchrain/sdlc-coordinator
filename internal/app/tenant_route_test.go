package app

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/tenantdb"

	"gorm.io/gorm"
)

// 对账：索引以租户库为权威——租户库里的仓决定索引应有的
// 行；索引多出的行删除，指派错租户的行修正。
func TestReconcileRepoRouteIndexAlignsWithTenantStores(t *testing.T) {
	cpRepo := openControlPlaneRepository(t, testControlPlaneSettings(t))
	ctx := context.Background()
	tenant := &controlplane.Tenant{Slug: "acme-it", DisplayName: "Acme IT"}
	if err := cpRepo.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}

	root := t.TempDir()
	resolver := tenantdb.NewSqliteFileResolver(nil, root, 0)
	t.Cleanup(resolver.Close)
	tenantStore, err := resolver.Handle(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("open tenant store: %v", err)
	}
	repoStore := registeredrepo.NewStore(tenantStore.DB)
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate repo store: %v", err)
	}
	live := &registeredrepo.RegisteredRepository{
		RepositoryID: "repo-live", DisplayName: "Live", Provider: "gitlab",
		BaseURL: "https://gitlab.example.com", PathWithNamespace: "acme/live",
	}
	if err := repoStore.Create(ctx, live); err != nil {
		t.Fatalf("seed tenant repo: %v", err)
	}

	// 索引漂移：live 仓被指派错租户；phantom 行在租户库已不存在。
	if err := cpRepo.UpsertRepoRouteIndex(ctx, controlplane.RepoRouteIndex{
		Provider: "gitlab", BaseURL: "https://gitlab.example.com", PathWithNamespace: "acme/live",
		TenantID: 99, RepoFingerprint: "repo-live",
	}); err != nil {
		t.Fatalf("seed stale index: %v", err)
	}
	if err := cpRepo.UpsertRepoRouteIndex(ctx, controlplane.RepoRouteIndex{
		Provider: "gitlab", BaseURL: "https://gitlab.example.com", PathWithNamespace: "acme/phantom",
		TenantID: tenant.ID, RepoFingerprint: "repo-phantom",
	}); err != nil {
		t.Fatalf("seed phantom index: %v", err)
	}

	if err := reconcileRepoRouteIndex(ctx, cpRepo, resolver, nil); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	route, err := cpRepo.GetRepoRouteIndex(ctx, controlplane.RepoRouteIdentity{
		Provider: "gitlab", BaseURL: "https://gitlab.example.com", PathWithNamespace: "acme/live",
	})
	if err != nil || route.TenantID != tenant.ID {
		t.Fatalf("live route must be corrected to its tenant: %#v err=%v", route, err)
	}
	if _, err := cpRepo.GetRepoRouteIndex(ctx, controlplane.RepoRouteIdentity{
		Provider: "gitlab", BaseURL: "https://gitlab.example.com", PathWithNamespace: "acme/phantom",
	}); err == nil {
		t.Fatalf("phantom route must be removed by reconciliation")
	}
}

// webhook 租户路由：索引命中 → 租户库仓行 secret；索引未命中回退
// 平台库仓行（存量部署过渡期）；两处皆未命中返回错误由调用方 fail-closed。
func TestTenantRouteSecretLookup(t *testing.T) {
	cpRepo := openControlPlaneRepository(t, testControlPlaneSettings(t))
	ctx := context.Background()
	tenant := &controlplane.Tenant{Slug: "acme-it", DisplayName: "Acme IT"}
	if err := cpRepo.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	root := t.TempDir()
	resolver := tenantdb.NewSqliteFileResolver(nil, root, 0)
	t.Cleanup(resolver.Close)
	tenantStore, err := resolver.Handle(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("open tenant store: %v", err)
	}
	repoStore := registeredrepo.NewStore(tenantStore.DB)
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate repo store: %v", err)
	}
	if err := repoStore.Create(ctx, &registeredrepo.RegisteredRepository{
		RepositoryID: "repo-t", DisplayName: "T", Provider: "gitlab",
		BaseURL: "https://gitlab.example.com", PathWithNamespace: "acme/app", WebhookSecret: "tenant-secret",
	}); err != nil {
		t.Fatalf("seed tenant repo: %v", err)
	}
	platformRepoStore := registeredrepo.NewStore(openPlatformStoreDB(t))
	if err := platformRepoStore.Migrate(); err != nil {
		t.Fatalf("migrate platform repo store: %v", err)
	}

	lookup := &tenantRouteSecretLookup{index: cpRepo, resolver: resolver, platform: platformRepoStore}

	// 索引未命中 + 平台库未命中 → fail-closed。
	if secret, err := lookup.WebhookSecretByPath(ctx, "gitlab", "acme/missing"); err == nil || secret != "" {
		t.Fatalf("unknown repo must fail closed, got secret=%q err=%v", secret, err)
	}

	// 索引未命中 → 平台库兜底（存量部署过渡期）。
	if err := platformRepoStore.Create(ctx, &registeredrepo.RegisteredRepository{
		RepositoryID: "repo-legacy", DisplayName: "L", Provider: "gitlab",
		BaseURL: "https://gitlab.example.com", PathWithNamespace: "acme/legacy", WebhookSecret: "legacy-secret",
	}); err != nil {
		t.Fatalf("seed platform repo: %v", err)
	}
	secret, err := lookup.WebhookSecretByPath(ctx, "gitlab", "acme/legacy")
	if err != nil || secret != "legacy-secret" {
		t.Fatalf("platform fallback lookup: secret=%q err=%v", secret, err)
	}

	// 索引命中 → 租户库 secret 优先于平台库。
	if err := cpRepo.UpsertRepoRouteIndex(ctx, controlplane.RepoRouteIndex{
		Provider: "gitlab", BaseURL: "https://gitlab.example.com", PathWithNamespace: "acme/app",
		TenantID: tenant.ID, RepoFingerprint: "repo-t",
	}); err != nil {
		t.Fatalf("seed index: %v", err)
	}
	secret, err = lookup.WebhookSecretByPath(ctx, "gitlab", "acme/app")
	if err != nil || secret != "tenant-secret" {
		t.Fatalf("tenant routed lookup: secret=%q err=%v", secret, err)
	}
}

func openPlatformStoreDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := store.Open("file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "platform.db")))
	if err != nil {
		t.Fatalf("open platform db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// 对账冲突：同身份被两个租户同时声称时保留现状——不翻转、不删除、
// 指纹不动，交由人工裁决。
func TestReconcileRepoRouteIndexKeepsAssignmentOnCrossTenantConflict(t *testing.T) {
	cpRepo := openControlPlaneRepository(t, testControlPlaneSettings(t))
	ctx := context.Background()
	owner := &controlplane.Tenant{Slug: "owner-org", DisplayName: "Owner"}
	rival := &controlplane.Tenant{Slug: "rival-org", DisplayName: "Rival"}
	if err := cpRepo.CreateTenant(ctx, owner); err != nil {
		t.Fatalf("seed owner tenant: %v", err)
	}
	if err := cpRepo.CreateTenant(ctx, rival); err != nil {
		t.Fatalf("seed rival tenant: %v", err)
	}

	root := t.TempDir()
	resolver := tenantdb.NewSqliteFileResolver(nil, root, 0)
	t.Cleanup(resolver.Close)

	identity := controlplane.RepoRouteIdentity{
		Provider: "gitlab", BaseURL: "https://gitlab.example.com", PathWithNamespace: "shared/repo",
	}
	for _, tenantID := range []uint{owner.ID, rival.ID} {
		tenantStore, err := resolver.Handle(ctx, tenantID)
		if err != nil {
			t.Fatalf("open tenant store %d: %v", tenantID, err)
		}
		repoStore := registeredrepo.NewStore(tenantStore.DB)
		if err := repoStore.Migrate(); err != nil {
			t.Fatalf("migrate repo store: %v", err)
		}
		if err := repoStore.Create(ctx, &registeredrepo.RegisteredRepository{
			RepositoryID: fmt.Sprintf("repo-%d", tenantID), DisplayName: "Shared", Provider: identity.Provider,
			BaseURL: identity.BaseURL, PathWithNamespace: identity.PathWithNamespace,
		}); err != nil {
			t.Fatalf("seed tenant repo %d: %v", tenantID, err)
		}
	}

	if err := cpRepo.UpsertRepoRouteIndex(ctx, controlplane.RepoRouteIndex{
		Provider: identity.Provider, BaseURL: identity.BaseURL, PathWithNamespace: identity.PathWithNamespace,
		TenantID: owner.ID, RepoFingerprint: "repo-owner",
	}); err != nil {
		t.Fatalf("seed index: %v", err)
	}

	if err := reconcileRepoRouteIndex(ctx, cpRepo, resolver, nil); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	route, err := cpRepo.GetRepoRouteIndex(ctx, identity)
	if err != nil {
		t.Fatalf("conflicted route must survive: %v", err)
	}
	if route.TenantID != owner.ID || route.RepoFingerprint != "repo-owner" {
		t.Fatalf("conflicted identity must stay with its owner untouched, got tenant=%d fingerprint=%s",
			route.TenantID, route.RepoFingerprint)
	}
}
