package app

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/tenantdb"
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

// webhook 租户路由：索引命中 → 租户库仓行 secret；索引未命中即未注册
// 返回空串（401），基础设施抖动返回错误（调用方 5xx 重试）。
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

	lookup := &tenantRouteSecretLookup{index: cpRepo, resolver: resolver}

	// 索引未命中 → 空串无错误（未注册，调用方 401）。
	if secret, err := lookup.WebhookSecretByPath(ctx, "gitlab", "acme/missing", ""); err != nil || secret != "" {
		t.Fatalf("unknown repo must be empty-without-error, got secret=%q err=%v", secret, err)
	}

	// 索引命中但租户库无仓行（索引漂移）→ 同未注册处理，等对账清理索引。
	if err := cpRepo.UpsertRepoRouteIndex(ctx, controlplane.RepoRouteIndex{
		Provider: "gitlab", BaseURL: "https://gitlab.example.com", PathWithNamespace: "acme/gone",
		TenantID: tenant.ID, RepoFingerprint: "repo-gone",
	}); err != nil {
		t.Fatalf("seed drifted index: %v", err)
	}
	if secret, err := lookup.WebhookSecretByPath(ctx, "gitlab", "acme/gone", ""); err != nil || secret != "" {
		t.Fatalf("drifted index must be empty-without-error, got secret=%q err=%v", secret, err)
	}

	// 索引命中 → 租户库仓行 secret。
	if err := cpRepo.UpsertRepoRouteIndex(ctx, controlplane.RepoRouteIndex{
		Provider: "gitlab", BaseURL: "https://gitlab.example.com", PathWithNamespace: "acme/app",
		TenantID: tenant.ID, RepoFingerprint: "repo-t",
	}); err != nil {
		t.Fatalf("seed index: %v", err)
	}
	secret, err := lookup.WebhookSecretByPath(ctx, "gitlab", "acme/app", "")
	if err != nil || secret != "tenant-secret" {
		t.Fatalf("tenant routed lookup: secret=%q err=%v", secret, err)
	}
}

// 基础设施故障（租户库查询失败）必须上抛错误（调用方 5xx 让代码托管
// 重试），不得折叠成「未注册」的 401——401 不会被代码托管重试，故障
// 窗口内的事件会被永久丢弃。
func TestTenantRouteSecretLookupSurfacesStoreFailures(t *testing.T) {
	cpRepo := openControlPlaneRepository(t, testControlPlaneSettings(t))
	ctx := context.Background()
	tenant := &controlplane.Tenant{Slug: "acme-dying", DisplayName: "Dying"}
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
	if err := cpRepo.UpsertRepoRouteIndex(ctx, controlplane.RepoRouteIndex{
		Provider: "gitlab", BaseURL: "https://gitlab.example.com", PathWithNamespace: "acme/down",
		TenantID: tenant.ID, RepoFingerprint: "repo-down",
	}); err != nil {
		t.Fatalf("seed index: %v", err)
	}
	lookup := &tenantRouteSecretLookup{index: cpRepo, resolver: resolver}

	// 索引已指认租户，但租户库句柄失效：查询故障必须上抛（5xx 语义）。
	if sqlDB, err := tenantStore.DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
	if _, err := lookup.WebhookSecretByPath(ctx, "gitlab", "acme/down", ""); err == nil {
		t.Fatal("tenant store failure must surface an error, not an empty 401 secret")
	}
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

// 冻结保留：停用组织的索引行在对账后原样保留；其身份被活跃组织认领时
// 不翻转（保现状）。
func TestReconcileRepoRouteIndexKeepsFrozenTenantRows(t *testing.T) {
	cpRepo := openControlPlaneRepository(t, testControlPlaneSettings(t))
	ctx := context.Background()
	owner := &controlplane.Tenant{Slug: "frozen-org", DisplayName: "Frozen"}
	rival := &controlplane.Tenant{Slug: "rival-org", DisplayName: "Rival"}
	if err := cpRepo.CreateTenant(ctx, owner); err != nil {
		t.Fatalf("seed owner tenant: %v", err)
	}
	if err := cpRepo.CreateTenant(ctx, rival); err != nil {
		t.Fatalf("seed rival tenant: %v", err)
	}
	owner.Status = controlplane.TenantStatusDisabled
	if err := cpRepo.SaveTenant(ctx, owner); err != nil {
		t.Fatalf("disable owner: %v", err)
	}

	root := t.TempDir()
	resolver := tenantdb.NewSqliteFileResolver(nil, root, 0)
	t.Cleanup(resolver.Close)
	rivalStore, err := resolver.Handle(ctx, rival.ID)
	if err != nil {
		t.Fatalf("open rival store: %v", err)
	}
	rivalRepoStore := registeredrepo.NewStore(rivalStore.DB)
	if err := rivalRepoStore.Migrate(); err != nil {
		t.Fatalf("migrate rival repo store: %v", err)
	}
	// 活跃组织在租户库里认领冻结组织的身份（绕过注册预检的直写形态）。
	if err := rivalRepoStore.Create(ctx, &registeredrepo.RegisteredRepository{
		RepositoryID: "repo-rival", DisplayName: "Rival", Provider: "gitlab",
		BaseURL: "https://gitlab.example.com", PathWithNamespace: "frozen/shared",
	}); err != nil {
		t.Fatalf("seed rival repo: %v", err)
	}

	claimed := controlplane.RepoRouteIdentity{Provider: "gitlab", BaseURL: "https://gitlab.example.com", PathWithNamespace: "frozen/shared"}
	orphan := controlplane.RepoRouteIdentity{Provider: "gitlab", BaseURL: "https://gitlab.example.com", PathWithNamespace: "frozen/orphan"}
	for _, seed := range []controlplane.RepoRouteIndex{
		{Provider: claimed.Provider, BaseURL: claimed.BaseURL, PathWithNamespace: claimed.PathWithNamespace,
			TenantID: owner.ID, RepoFingerprint: "repo-owner"},
		{Provider: orphan.Provider, BaseURL: orphan.BaseURL, PathWithNamespace: orphan.PathWithNamespace,
			TenantID: owner.ID, RepoFingerprint: "repo-orphan"},
	} {
		if err := cpRepo.UpsertRepoRouteIndex(ctx, seed); err != nil {
			t.Fatalf("seed index: %v", err)
		}
	}

	if err := reconcileRepoRouteIndex(ctx, cpRepo, resolver, nil); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	route, err := cpRepo.GetRepoRouteIndex(ctx, claimed)
	if err != nil || route.TenantID != owner.ID {
		t.Fatalf("frozen tenant's claimed identity must stay with the frozen owner, got tenant=%d err=%v", route.TenantID, err)
	}
	if _, err := cpRepo.GetRepoRouteIndex(ctx, orphan); err != nil {
		t.Fatalf("frozen tenant's rows must survive reconciliation, got %v", err)
	}
}

// 撞名消歧：两个组织在不同实例合法注册了同 (provider, path) 的仓（索引
// 两行共存是合法形态）。webhook 载荷自带仓库完整链接，据此推导实例地址
// 归因到正确组织；推导失败或零行匹配维持歧义拒绝（fail-closed，不猜
// 不兜底）。单行命中时链接不参与逻辑（快路径行为不变）。
func TestResolveTenantByPathDisambiguatesByInstanceURL(t *testing.T) {
	cpRepo := openControlPlaneRepository(t, testControlPlaneSettings(t))
	ctx := context.Background()
	alpha := &controlplane.Tenant{Slug: "alpha-org", DisplayName: "Alpha"}
	beta := &controlplane.Tenant{Slug: "beta-org", DisplayName: "Beta"}
	for _, tenant := range []*controlplane.Tenant{alpha, beta} {
		if err := cpRepo.CreateTenant(ctx, tenant); err != nil {
			t.Fatalf("seed tenant %s: %v", tenant.Slug, err)
		}
	}
	shared := "shared/repo"
	for _, seed := range []controlplane.RepoRouteIndex{
		{Provider: "gitlab", BaseURL: "https://gitlab.alpha.example.com", PathWithNamespace: shared,
			TenantID: alpha.ID, RepoFingerprint: "repo-alpha"},
		{Provider: "gitlab", BaseURL: "https://gitlab.beta.example.com", PathWithNamespace: shared,
			TenantID: beta.ID, RepoFingerprint: "repo-beta"},
		{Provider: "gitlab", BaseURL: "https://gitlab.alpha.example.com", PathWithNamespace: "solo/repo",
			TenantID: alpha.ID, RepoFingerprint: "repo-solo"},
	} {
		if err := cpRepo.UpsertRepoRouteIndex(ctx, seed); err != nil {
			t.Fatalf("seed index: %v", err)
		}
	}

	// 载荷链接指向其中一行的实例 → 归因到该行。
	if tenantID, err := resolveTenantByPath(ctx, cpRepo, "gitlab", shared, "https://gitlab.alpha.example.com/shared/repo"); err != nil || tenantID != alpha.ID {
		t.Fatalf("alpha instance must route to alpha, got tenant=%d err=%v", tenantID, err)
	}
	if tenantID, err := resolveTenantByPath(ctx, cpRepo, "gitlab", shared, "https://gitlab.beta.example.com/shared/repo"); err != nil || tenantID != beta.ID {
		t.Fatalf("beta instance must route to beta, got tenant=%d err=%v", tenantID, err)
	}
	// 显式默认端口是同一实例的另一种写法，双侧归一化后必须命中。
	if tenantID, err := resolveTenantByPath(ctx, cpRepo, "gitlab", shared, "https://gitlab.beta.example.com:443/shared/repo"); err != nil || tenantID != beta.ID {
		t.Fatalf("explicit default port must still disambiguate, got tenant=%d err=%v", tenantID, err)
	}
	// 指向第三方实例（两行都不匹配）→ 歧义拒绝。
	if _, err := resolveTenantByPath(ctx, cpRepo, "gitlab", shared, "https://gitlab.gamma.example.com/shared/repo"); err == nil {
		t.Fatal("third-party instance must keep the ambiguity error")
	}
	// 载荷缺完整链接 → 歧义拒绝。
	if _, err := resolveTenantByPath(ctx, cpRepo, "gitlab", shared, ""); err == nil {
		t.Fatal("missing repo url must keep the ambiguity error")
	}
	// 链接与路径后缀不匹配 → 歧义拒绝。
	if _, err := resolveTenantByPath(ctx, cpRepo, "gitlab", shared, "https://gitlab.alpha.example.com/other/repo"); err == nil {
		t.Fatal("suffix mismatch must keep the ambiguity error")
	}
	// 单行命中：链接为空也不参与逻辑（快路径行为不变）。
	if tenantID, err := resolveTenantByPath(ctx, cpRepo, "gitlab", "solo/repo", ""); err != nil || tenantID != alpha.ID {
		t.Fatalf("single row must ignore the repo url, got tenant=%d err=%v", tenantID, err)
	}
}

// 写穿端口的哨兵翻译：控制面的身份占用哨兵（UpsertRepoRouteIndex 对
// 跨租户换主返回）必须翻译为注册面哨兵——service 的败者回滚分界只认
// registeredrepo.ErrRouteIdentityClaimed，感知不到控制面包。
func TestControlPlaneRouteIndexTranslatesClaimedSentinel(t *testing.T) {
	cpRepo := openControlPlaneRepository(t, testControlPlaneSettings(t))
	ctx := context.Background()
	owner := &controlplane.Tenant{Slug: "owner-org", DisplayName: "Owner"}
	rival := &controlplane.Tenant{Slug: "rival-org", DisplayName: "Rival"}
	for _, tenant := range []*controlplane.Tenant{owner, rival} {
		if err := cpRepo.CreateTenant(ctx, tenant); err != nil {
			t.Fatalf("seed tenant %s: %v", tenant.Slug, err)
		}
	}
	sync := &controlPlaneRouteIndex{repo: cpRepo}
	entry := registeredrepo.RouteIndexEntry{
		Provider: "gitlab", BaseURL: "https://gitlab.example.com",
		PathWithNamespace: "acme/app", RepoFingerprint: "repo-a",
	}
	if err := sync.Upsert(ctx, owner.ID, entry); err != nil {
		t.Fatalf("first claim must succeed: %v", err)
	}
	err := sync.Upsert(ctx, rival.ID, entry)
	if !errors.Is(err, registeredrepo.ErrRouteIdentityClaimed) {
		t.Fatalf("cross-tenant upsert must surface the registration-scope claimed sentinel, got %v", err)
	}
}
