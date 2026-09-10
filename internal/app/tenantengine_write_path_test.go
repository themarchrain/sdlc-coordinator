package app

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/tenantdb"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// TestTenantWebhookCreatesTaskInTenantStore 是写路径租户化的 headline：
// 租户库中的注册仓行收到 webhook 后，事件入库与任务创建全部落在该租户的
// 租户库；平台主库的 inbound_events/tasks 保持零行（读写不再错位）。
func TestTenantWebhookCreatesTaskInTenantStore(t *testing.T) {
	ctx := context.Background()
	db := openAppTestDB(t)
	cfg := withTestControlPlane(t, config.Config{HTTPAddr: ":0"})
	application, err := New(cfg, zap.NewNop(), db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	// Windows 下租户库句柄不显式关闭会阻塞 TempDir 清理。
	t.Cleanup(func() {
		application.pool.CloseTenantEngines()
		if resolver, ok := application.TenantStores.(*tenantdb.SqliteFileResolver); ok {
			resolver.Close()
		}
	})

	// 建组织（经 provisioner 同步开通租户库并迁移 schema）。
	tenant, err := application.ControlPlane.CreateTenant(ctx, &controlplane.Actor{ID: 1, Username: "root"}, controlplane.TenantInput{
		Slug: "acme", DisplayName: "Acme 组织",
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	// 租户库注册仓行（生产经 admin 面注册+写穿索引；测试直接落库+索引）。
	tenantStore, err := application.TenantStores.Handle(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("open tenant store: %v", err)
	}
	tenantRepoStore := registeredrepo.NewStore(tenantStore.DB)
	if err := tenantRepoStore.Create(ctx, &registeredrepo.RegisteredRepository{
		RepositoryID: "repo_tn", DisplayName: "租户仓", Provider: "gitea", BaseURL: "https://gitea.example",
		CodeHostProjectID: "8899", PathWithNamespace: "acme/widget",
		Token: "token-tn", WebhookSecret: "secret-tn", TokenScopeAttested: true,
		Status: registeredrepo.StatusEnabled,
	}); err != nil {
		t.Fatalf("seed tenant repository: %v", err)
	}
	if err := application.ControlPlane.Repository().UpsertRepoRouteIndex(ctx, controlplane.RepoRouteIndex{
		Provider: "gitea", BaseURL: "https://gitea.example", PathWithNamespace: "acme/widget",
		TenantID: tenant.ID, RepoFingerprint: "repo_tn",
	}); err != nil {
		t.Fatalf("upsert route index: %v", err)
	}

	payload := giteaIssueOpenedPayload(8899, "acme/widget")
	response := performAppRawJSONWithHeaders(t, application.Router, http.MethodPost, "/api/events/gitea", payload,
		map[string]string{"X-Gitea-Signature": hmacSHA256Hex("secret-tn", payload)})
	if response.Code != http.StatusAccepted {
		t.Fatalf("tenant webhook must be accepted, got %d: %s", response.Code, response.Body.String())
	}
	if got := response.Body.String(); !strings.Contains(got, "task_created") {
		t.Fatalf("expected task_created in response, got: %s", got)
	}

	// 任务与入站事件都落在租户库。
	assertTenantRowCount(t, tenantStore.DB, "tasks", 1)
	assertTenantRowCount(t, tenantStore.DB, "inbound_events", 1)
	// 平台主库两表零行：读写错位消除。
	assertTenantRowCount(t, db, "tasks", 0)
	assertTenantRowCount(t, db, "inbound_events", 0)

	// 同一事件重放（同载荷）幂等：租户库任务仍恰一条（dedupe 落在租户库内）。
	replay := performAppRawJSONWithHeaders(t, application.Router, http.MethodPost, "/api/events/gitea", payload,
		map[string]string{"X-Gitea-Signature": hmacSHA256Hex("secret-tn", payload)})
	if replay.Code != http.StatusAccepted {
		t.Fatalf("replay must be accepted, got %d", replay.Code)
	}
	assertTenantRowCount(t, tenantStore.DB, "tasks", 1)
	assertTenantRowCount(t, tenantStore.DB, "inbound_events", 1)
	assertTenantRowCount(t, db, "inbound_events", 0)
}

// assertTenantRowCount 断言指定库的业务表行数。
func assertTenantRowCount(t *testing.T, db *gorm.DB, table string, want int) {
	t.Helper()
	var count int64
	if err := db.Table(table).Count(&count).Error; err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if count != int64(want) {
		t.Fatalf("table %s: want %d rows, got %d", table, want, count)
	}
}

// forTenantID 并发下同租户恒返回同一引擎实例（double-check 先建者胜出）。
func TestEnginePoolForTenantIDIsSingletonUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	db := openAppTestDB(t)
	cfg := withTestControlPlane(t, config.Config{HTTPAddr: ":0"})
	application, err := New(cfg, zap.NewNop(), db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() {
		application.pool.CloseTenantEngines()
		if resolver, ok := application.TenantStores.(*tenantdb.SqliteFileResolver); ok {
			resolver.Close()
		}
	})
	tenant, err := application.ControlPlane.CreateTenant(ctx, &controlplane.Actor{ID: 1, Username: "root"}, controlplane.TenantInput{
		Slug: "singleton", DisplayName: "并发单实例",
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	const workers = 16
	engines := make([]*tenantEngine, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			<-start
			engines[slot] = application.pool.forTenantID(ctx, tenant.ID)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, engine := range engines {
		if engine == nil {
			t.Fatalf("worker %d got nil engine", i)
		}
		if engine != engines[0] {
			t.Fatalf("worker %d got a different engine instance (double-check violated)", i)
		}
	}
}

// 停用组织即时失去引擎（状态门逐调用生效，缓存命中也不例外）；恢复后
// 引擎回归。停用同时删除路由索引行，webhook 租户路由随之失联。
func TestEnginePoolForTenantIDFailsClosedForDisabledTenant(t *testing.T) {
	ctx := context.Background()
	db := openAppTestDB(t)
	cfg := withTestControlPlane(t, config.Config{HTTPAddr: ":0"})
	application, err := New(cfg, zap.NewNop(), db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() {
		application.pool.CloseTenantEngines()
		if resolver, ok := application.TenantStores.(*tenantdb.SqliteFileResolver); ok {
			resolver.Close()
		}
	})
	actor := &controlplane.Actor{ID: 1, Username: "root"}
	tenant, err := application.ControlPlane.CreateTenant(ctx, actor, controlplane.TenantInput{
		Slug: "frozen", DisplayName: "冻结验证",
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if application.pool.forTenantID(ctx, tenant.ID) == nil {
		t.Fatal("active tenant must get an engine")
	}
	if err := application.ControlPlane.Repository().UpsertRepoRouteIndex(ctx, controlplane.RepoRouteIndex{
		Provider: "gitlab", BaseURL: "https://gitlab.example.com", PathWithNamespace: "frozen/repo",
		TenantID: tenant.ID, RepoFingerprint: "repo-frozen",
	}); err != nil {
		t.Fatalf("seed route index: %v", err)
	}

	if _, err := application.ControlPlane.DisableTenant(ctx, actor, tenant.ID); err != nil {
		t.Fatalf("disable tenant: %v", err)
	}
	if application.pool.forTenantID(ctx, tenant.ID) != nil {
		t.Fatal("disabled tenant must lose its engine even when cached")
	}
	if view := application.pool.MCPEngineForTenant(tenant.ID); view.Channel != nil {
		t.Fatal("disabled tenant must have no MCP engine view")
	}
	if _, err := application.ControlPlane.Repository().GetRepoRouteIndex(ctx, controlplane.RepoRouteIdentity{
		Provider: "gitlab", BaseURL: "https://gitlab.example.com", PathWithNamespace: "frozen/repo",
	}); err == nil {
		t.Fatal("disable must remove the tenant's route index rows")
	}

	if _, err := application.ControlPlane.RestoreTenant(ctx, actor, tenant.ID); err != nil {
		t.Fatalf("restore tenant: %v", err)
	}
	if application.pool.forTenantID(ctx, tenant.ID) == nil {
		t.Fatal("restored tenant must get its engine back")
	}
}

// engineForPath 三态：索引指认的租户打不开引擎 → 错误（5xx，不回落
// 平台库）；索引未命中 → 平台处理器（存量过渡期兜底）；正常租户 →
// 该租户引擎的处理器。
func TestEngineForPathDistinguishesMissFromUnavailable(t *testing.T) {
	ctx := context.Background()
	db := openAppTestDB(t)
	cfg := withTestControlPlane(t, config.Config{HTTPAddr: ":0"})
	application, err := New(cfg, zap.NewNop(), db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() {
		application.pool.CloseTenantEngines()
		if resolver, ok := application.TenantStores.(*tenantdb.SqliteFileResolver); ok {
			resolver.Close()
		}
	})
	tenant, err := application.ControlPlane.CreateTenant(ctx, &controlplane.Actor{ID: 1, Username: "root"}, controlplane.TenantInput{
		Slug: "routed", DisplayName: "路由验证",
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	tenantStore, err := application.TenantStores.Handle(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("open tenant store: %v", err)
	}
	if err := registeredrepo.NewStore(tenantStore.DB).Create(ctx, &registeredrepo.RegisteredRepository{
		RepositoryID: "repo-ok", DisplayName: "正常仓", Provider: "gitea", BaseURL: "https://gitea.example",
		PathWithNamespace: "routed/ok", WebhookSecret: "secret-ok", TokenScopeAttested: true,
		Status: registeredrepo.StatusEnabled,
	}); err != nil {
		t.Fatalf("seed tenant repository: %v", err)
	}
	if err := application.ControlPlane.Repository().UpsertRepoRouteIndex(ctx, controlplane.RepoRouteIndex{
		Provider: "gitea", BaseURL: "https://gitea.example", PathWithNamespace: "routed/ok",
		TenantID: tenant.ID, RepoFingerprint: "repo-ok",
	}); err != nil {
		t.Fatalf("seed route index: %v", err)
	}
	// 索引指向不存在的租户：引擎解析必然失败，必须上抛错误。
	if err := application.ControlPlane.Repository().UpsertRepoRouteIndex(ctx, controlplane.RepoRouteIndex{
		Provider: "gitea", BaseURL: "https://gitea.example", PathWithNamespace: "ghost/repo",
		TenantID: 424242, RepoFingerprint: "repo-ghost",
	}); err != nil {
		t.Fatalf("seed ghost index: %v", err)
	}

	if _, err := application.pool.engineForPath(ctx, "gitea", "ghost/repo"); err == nil {
		t.Fatal("engine for a routed but unavailable tenant must surface an error")
	}
	processor, err := application.pool.engineForPath(ctx, "gitea", "nowhere/repo")
	if err != nil || processor != application.pool.platform.processor {
		t.Fatalf("unrouted repo must fall back to the platform processor, got proc=%v err=%v", processor, err)
	}
	if processor, err := application.pool.engineForPath(ctx, "gitea", "routed/ok"); err != nil || processor == nil {
		t.Fatalf("routed active tenant must yield its engine processor, got proc=%v err=%v", processor, err)
	}
}
