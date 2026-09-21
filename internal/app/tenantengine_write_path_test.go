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
// 租户库；平台主库无 inbound_events/tasks 表（读写不再错位）。
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
	// 平台主库无业务表（无表=零行的结构性加强版）：读写错位消除。
	assertPlatformBusinessTableAbsent(t, db, "tasks")
	assertPlatformBusinessTableAbsent(t, db, "inbound_events")

	// 同一事件重放（同载荷）幂等：租户库任务仍恰一条（dedupe 落在租户库内）。
	replay := performAppRawJSONWithHeaders(t, application.Router, http.MethodPost, "/api/events/gitea", payload,
		map[string]string{"X-Gitea-Signature": hmacSHA256Hex("secret-tn", payload)})
	if replay.Code != http.StatusAccepted {
		t.Fatalf("replay must be accepted, got %d", replay.Code)
	}
	assertTenantRowCount(t, tenantStore.DB, "tasks", 1)
	assertTenantRowCount(t, tenantStore.DB, "inbound_events", 1)
	assertPlatformBusinessTableAbsent(t, db, "inbound_events")
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

// assertPlatformBusinessTableAbsent 锁定平台库不建业务表（结构性断言，
// 比行数清零更强：无表即无落点）。
func assertPlatformBusinessTableAbsent(t *testing.T, db *gorm.DB, table string) {
	t.Helper()
	if db.Migrator().HasTable(table) {
		t.Fatalf("platform store must not create table %s", table)
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

// 停用组织即时失去引擎（状态门逐调用生效，缓存命中也不例外）；路由认领
// 保留不删，停用期间已验签的 webhook 按可重试拒绝（引擎不可用 5xx），
// 平台库保持零写入；恢复后引擎回归、路由立即可用。
func TestEnginePoolForTenantIDFailsClosedForDisabledTenant(t *testing.T) {
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
	// 租户库仓行使停用期间的验签仍可解析（行与认领都保留）。
	tenantStore, err := application.TenantStores.Handle(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("open tenant store: %v", err)
	}
	if err := registeredrepo.NewStore(tenantStore.DB).Create(ctx, &registeredrepo.RegisteredRepository{
		RepositoryID: "repo-frozen", DisplayName: "冻结仓", Provider: "gitlab", BaseURL: "https://gitlab.example.com",
		CodeHostProjectID: "6600", PathWithNamespace: "frozen/repo",
		Token: "token-frozen", WebhookSecret: "secret-frozen", TokenScopeAttested: true,
		Status: registeredrepo.StatusEnabled,
	}); err != nil {
		t.Fatalf("seed tenant repository: %v", err)
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
	}); err != nil {
		t.Fatal("disable must keep the tenant's route index claims")
	}

	// 停用期间已验签的 webhook：引擎不可用 → 5xx（可重试），不写平台库。
	payload := []byte(`{"project":{"path_with_namespace":"frozen/repo"},"object_kind":"issue"}`)
	frozen := performAppRawJSONWithHeaders(t, application.Router, http.MethodPost, "/api/events/gitlab", payload,
		map[string]string{"X-Gitlab-Token": "secret-frozen"})
	if frozen.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled tenant webhook must fail closed with 503, got %d: %s", frozen.Code, frozen.Body.String())
	}
	assertPlatformBusinessTableAbsent(t, db, "inbound_events")
	assertTenantRowCount(t, tenantStore.DB, "inbound_events", 0)

	if _, err := application.ControlPlane.RestoreTenant(ctx, actor, tenant.ID); err != nil {
		t.Fatalf("restore tenant: %v", err)
	}
	if application.pool.forTenantID(ctx, tenant.ID) == nil {
		t.Fatal("restored tenant must get its engine back")
	}
}

// engineViewForPath 三态：索引指认的租户打不开引擎 → 错误（5xx，不回落
// 平台库）；索引未命中 → 平台视图（存量过渡期兜底）；正常租户 →
// 该租户引擎的视图。
func TestEngineViewForPathDistinguishesMissFromUnavailable(t *testing.T) {
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

	if _, err := application.pool.engineViewForPath(ctx, "gitea", "ghost/repo", ""); err == nil {
		t.Fatal("engine for a routed but unavailable tenant must surface an error")
	}
	// 未路由仓：验签层已按未注册 401 拒收，引擎解析同样不得回落平台视图。
	if _, err := application.pool.engineViewForPath(ctx, "gitea", "nowhere/repo", ""); err == nil {
		t.Fatal("unrouted repo must surface an error instead of a platform view")
	}
	if view, err := application.pool.engineViewForPath(ctx, "gitea", "routed/ok", ""); err != nil || view == nil || view.Processor == nil {
		t.Fatalf("routed active tenant must yield its engine view, got view=%v err=%v", view, err)
	}
}
