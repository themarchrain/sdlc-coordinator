package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"sdlc-coordinator/internal/controlplane"
)

func TestAdminAPIRejectsUnauthenticatedAndLegacyAdminCookie(t *testing.T) {
	world := newFullyWiredWorld(t)

	unauth := world.portalDo(t, http.MethodGet, "/api/admin/configuration/status", "", nil)
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d body=%s", unauth.Code, unauth.Body.String())
	}
	if code := errorCode(t, unauth); code != "unauthenticated" {
		t.Fatalf("unauthenticated code = %q", code)
	}

	legacy := world.do(t, http.MethodGet, "/api/admin/overview", "coordinator_admin_session", "stale-legacy-token", nil)
	if legacy.Code != http.StatusUnauthorized {
		t.Fatalf("legacy admin cookie must not grant /api/admin after the cutover, got %d body=%s", legacy.Code, legacy.Body.String())
	}
}

func TestAdminAPIMemberCanReadStatusAndRepositoriesWithoutSecrets(t *testing.T) {
	world := newFullyWiredWorld(t)
	tenantID := world.seedTenant(t, "acme")
	adminToken := world.seedUserWithRole(t, tenantID, "t2-admin", controlplane.RoleTenantAdmin)
	memberToken := world.seedUserWithRole(t, tenantID, "t2-member", controlplane.RoleMember)

	created := world.portalDo(t, http.MethodPost, "/api/admin/repositories", adminToken, map[string]any{
		"display_name":         "Coordinator",
		"path_with_namespace":  "group/coordinator",
		"provider":             "gitlab",
		"base_url":             "https://gitlab.example.com",
		"token":                "repo-token-value",
		"token_scope_attested": true,
		"webhook_secret":       "repo-hook-secret-value",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("tenant_admin create repository status = %d body=%s", created.Code, created.Body.String())
	}
	repoID, _ := decodeJSONMap(t, created)["repository_id"].(string)
	if repoID == "" {
		t.Fatalf("expected repository_id, got %s", created.Body.String())
	}

	status := world.portalDo(t, http.MethodGet, "/api/admin/configuration/status", memberToken, nil)
	if status.Code != http.StatusOK {
		t.Fatalf("member configuration/status = %d body=%s", status.Code, status.Body.String())
	}
	statusBody := decodeJSONMap(t, status)
	for _, key := range []string{"config", "secrets"} {
		if _, exists := statusBody[key]; exists {
			t.Fatalf("configuration/status must not contain %q: %#v", key, statusBody)
		}
	}

	listed := world.portalDo(t, http.MethodGet, "/api/admin/repositories", memberToken, nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("member list repositories = %d body=%s", listed.Code, listed.Body.String())
	}
	assertJSONLacksKeys(t, decodeJSONMap(t, listed), "token", "webhook_secret")

	detail := world.portalDo(t, http.MethodGet, "/api/admin/repositories/"+repoID, memberToken, nil)
	if detail.Code != http.StatusOK {
		t.Fatalf("member get repository = %d body=%s", detail.Code, detail.Body.String())
	}
	assertJSONLacksKeys(t, decodeJSONMap(t, detail), "token", "webhook_secret")
}

func TestAdminAPIMemberDeniedWritesAndTenantAdminOnlyRoutes(t *testing.T) {
	world := newFullyWiredWorld(t)
	tenantID := world.seedTenant(t, "acme")
	adminToken := world.seedUserWithRole(t, tenantID, "t2-admin", controlplane.RoleTenantAdmin)
	memberToken := world.seedUserWithRole(t, tenantID, "t2-member", controlplane.RoleMember)

	create := world.portalDo(t, http.MethodPost, "/api/admin/repositories", memberToken, map[string]any{
		"display_name":         "Blocked",
		"path_with_namespace":  "group/blocked",
		"provider":             "gitlab",
		"base_url":             "https://gitlab.example.com",
		"token":                "should-not-write",
		"token_scope_attested": true,
		"webhook_secret":       "should-not-write",
	})
	if create.Code != http.StatusForbidden {
		t.Fatalf("member create repository = %d, want 403 body=%s", create.Code, create.Body.String())
	}

	people := world.portalDo(t, http.MethodGet, "/api/admin/people", memberToken, nil)
	if people.Code != http.StatusForbidden {
		t.Fatalf("member list people = %d, want 403 body=%s", people.Code, people.Body.String())
	}

	config := world.portalDo(t, http.MethodGet, "/api/admin/configuration", memberToken, nil)
	if config.Code != http.StatusForbidden {
		t.Fatalf("member get configuration = %d, want 403 body=%s", config.Code, config.Body.String())
	}

	adminPeople := world.portalDo(t, http.MethodGet, "/api/admin/people", adminToken, nil)
	if adminPeople.Code != http.StatusOK {
		t.Fatalf("tenant_admin list people = %d body=%s", adminPeople.Code, adminPeople.Body.String())
	}

	adminConfig := world.portalDo(t, http.MethodGet, "/api/admin/configuration", adminToken, nil)
	if adminConfig.Code != http.StatusForbidden {
		t.Fatalf("tenant_admin get configuration = %d, want 403 body=%s", adminConfig.Code, adminConfig.Body.String())
	}
}

// 业务面 API 对超管收窄为 support_only（红线头条升级）——
// 未走支持访问一律 403；运维/观测/配置面（overview/metrics/configuration/
// agents/operations/connectors）保持 allow；支持访问中业务面放行（带审计）。
func TestAdminAPISuperAdminBusinessAPISupportOnly(t *testing.T) {
	world := newFullyWiredWorld(t)
	tenantID := world.seedTenant(t, "acme")
	token := world.loginSuperAdmin(t)

	status := world.portalDo(t, http.MethodGet, "/api/admin/configuration/status", token, nil)
	if status.Code != http.StatusOK {
		t.Fatalf("super_admin configuration/status = %d body=%s", status.Code, status.Body.String())
	}

	config := world.portalDo(t, http.MethodGet, "/api/admin/configuration", token, nil)
	if config.Code != http.StatusOK {
		t.Fatalf("super_admin get configuration = %d body=%s", config.Code, config.Body.String())
	}

	schedulerDefaults := world.portalDo(t, http.MethodGet, "/api/admin/scheduler/defaults", token, nil)
	if schedulerDefaults.Code != http.StatusOK {
		t.Fatalf("super_admin scheduler defaults = %d body=%s", schedulerDefaults.Code, schedulerDefaults.Body.String())
	}

	// 未支持访问：业务面 403（查询/观测/连接器四个面与任务/人员/需求同收）。
	overview := world.portalDo(t, http.MethodGet, "/api/admin/overview", token, nil)
	if overview.Code != http.StatusForbidden {
		t.Fatalf("super_admin without support overview = %d, want 403 body=%s", overview.Code, overview.Body.String())
	}
	metrics := world.portalDo(t, http.MethodGet, "/api/admin/metrics", token, nil)
	if metrics.Code != http.StatusForbidden {
		t.Fatalf("super_admin without support metrics = %d, want 403", metrics.Code)
	}
	agents := world.portalDo(t, http.MethodGet, "/api/admin/agents/executions/summary", token, nil)
	if agents.Code != http.StatusForbidden {
		t.Fatalf("super_admin without support agent summary = %d, want 403", agents.Code)
	}
	connectors := world.portalDo(t, http.MethodGet, "/api/admin/connectors", token, nil)
	if connectors.Code != http.StatusForbidden {
		t.Fatalf("super_admin without support list connectors = %d, want 403", connectors.Code)
	}
	tasks := world.portalDo(t, http.MethodGet, "/api/admin/tasks?page=1&page_size=10", token, nil)
	if tasks.Code != http.StatusForbidden {
		t.Fatalf("super_admin without support list tasks = %d, want 403 body=%s", tasks.Code, tasks.Body.String())
	}
	repos := world.portalDo(t, http.MethodPost, "/api/admin/repositories", token, map[string]any{})
	if repos.Code != http.StatusForbidden {
		t.Fatalf("super_admin without support create repository = %d, want 403", repos.Code)
	}
	people := world.portalDo(t, http.MethodGet, "/api/admin/people", token, nil)
	if people.Code != http.StatusForbidden {
		t.Fatalf("super_admin without support list people = %d, want 403", people.Code)
	}
	requirements := world.portalDo(t, http.MethodGet, "/api/admin/requirements", token, nil)
	if requirements.Code != http.StatusForbidden {
		t.Fatalf("super_admin without support list requirements = %d, want 403", requirements.Code)
	}

	// 支持访问中：业务面放行（动作入审计）。
	ctx := context.Background()
	if err := world.ControlPlane.EnterSupportAccess(ctx, token, tenantID, "支持访问收窄"); err != nil {
		t.Fatalf("enter support access: %v", err)
	}
	tasksSupported := world.portalDo(t, http.MethodGet, "/api/admin/tasks?page=1&page_size=10", token, nil)
	if tasksSupported.Code != http.StatusOK {
		t.Fatalf("super_admin with support list tasks = %d body=%s", tasksSupported.Code, tasksSupported.Body.String())
	}
	overviewSupported := world.portalDo(t, http.MethodGet, "/api/admin/overview", token, nil)
	if overviewSupported.Code != http.StatusOK {
		t.Fatalf("super_admin with support overview = %d body=%s", overviewSupported.Code, overviewSupported.Body.String())
	}
	peopleSupported := world.portalDo(t, http.MethodGet, "/api/admin/people", token, nil)
	if peopleSupported.Code != http.StatusOK {
		t.Fatalf("super_admin with support list people = %d body=%s", peopleSupported.Code, peopleSupported.Body.String())
	}
}

func TestAdminAPIUnscopedSessionCannotUseTenantAdminSurface(t *testing.T) {
	world := newFullyWiredWorld(t)
	tenantA := world.seedTenant(t, "alpha")
	tenantB := world.seedTenant(t, "beta")
	ctx := context.Background()
	ctxUser, err := world.ControlPlane.CreateUser(ctx, "t2-multi", testPortalPassword)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	actor := world.ensureSuperActor(t)
	if _, err := world.ControlPlane.AddMembership(ctx, actor, tenantA, ctxUser.ID, controlplane.RoleMember); err != nil {
		t.Fatalf("membership a: %v", err)
	}
	if _, err := world.ControlPlane.AddMembership(ctx, actor, tenantB, ctxUser.ID, controlplane.RoleTenantAdmin); err != nil {
		t.Fatalf("membership b: %v", err)
	}
	result, err := world.ControlPlane.Login(ctx, "t2-multi", testPortalPassword, "10.0.0.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result.Scope.Kind != controlplane.ScopeUnscoped {
		t.Fatalf("expected unscoped login, got %q", result.Scope.Kind)
	}

	resp := world.portalDo(t, http.MethodGet, "/api/admin/tasks?page=1&page_size=10", result.Token, nil)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("unscoped admin access = %d, want 403 body=%s", resp.Code, resp.Body.String())
	}
	if code := errorCode(t, resp); code != "tenant_scope_required" {
		t.Fatalf("unscoped error code = %q, want tenant_scope_required", code)
	}
}

func TestAdminAPIStillServesPortalAuthWithPortalCookie(t *testing.T) {
	world := newFullyWiredWorld(t)
	tenantID := world.seedTenant(t, "acme")
	token := world.seedUserWithRole(t, tenantID, "t2-session", controlplane.RoleMember)
	session := world.portalDo(t, http.MethodGet, "/api/portal/auth/session", token, nil)
	if session.Code != http.StatusOK {
		t.Fatalf("portal session = %d body=%s", session.Code, session.Body.String())
	}
}

// 组织段运行配置三端点的矩阵行为:member 403、tenant_admin 可读写、
// 未支持访问的超管 403、支持访问中放行。
func TestAdminAPITenantConfigurationMatrix(t *testing.T) {
	world := newFullyWiredWorld(t)
	tenantID := world.seedTenant(t, "acme")
	adminToken := world.seedUserWithRole(t, tenantID, "t3-admin", controlplane.RoleTenantAdmin)
	memberToken := world.seedUserWithRole(t, tenantID, "t3-member", controlplane.RoleMember)
	superToken := world.loginSuperAdmin(t)

	member := world.portalDo(t, http.MethodGet, "/api/admin/tenant-configuration", memberToken, nil)
	if member.Code != http.StatusForbidden {
		t.Fatalf("member get tenant configuration = %d, want 403 body=%s", member.Code, member.Body.String())
	}

	admin := world.portalDo(t, http.MethodGet, "/api/admin/tenant-configuration", adminToken, nil)
	if admin.Code != http.StatusOK || !strings.Contains(admin.Body.String(), `"configured":false`) {
		t.Fatalf("tenant_admin get tenant configuration = %d body=%s", admin.Code, admin.Body.String())
	}

	super := world.portalDo(t, http.MethodGet, "/api/admin/tenant-configuration", superToken, nil)
	if super.Code != http.StatusForbidden {
		t.Fatalf("super_admin without support get tenant configuration = %d, want 403", super.Code)
	}

	ctx := context.Background()
	if err := world.ControlPlane.EnterSupportAccess(ctx, superToken, tenantID, "组织配置排障"); err != nil {
		t.Fatalf("enter support access: %v", err)
	}
	superSupported := world.portalDo(t, http.MethodGet, "/api/admin/tenant-configuration", superToken, nil)
	if superSupported.Code != http.StatusOK {
		t.Fatalf("super_admin with support get tenant configuration = %d body=%s", superSupported.Code, superSupported.Body.String())
	}
}
