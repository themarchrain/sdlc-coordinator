package app

import (
	"context"
	"net/http"
	"strings"
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
