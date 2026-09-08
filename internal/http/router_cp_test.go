package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/store"

	"github.com/gin-gonic/gin"
)

const portalCookieInRouterTests = "coordinator_portal_session"

// newControlPlaneRouter 装配带控制面服务的完整路由（orch 为 nil，业务面照旧），
// 返回路由与已种子的控制面服务。
func newControlPlaneRouter(t *testing.T) (*gin.Engine, *controlplane.Service, *controlplane.Actor) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := store.Open(filepath.Join(t.TempDir(), "control-plane.db"))
	if err != nil {
		t.Fatalf("open control plane db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	repository := controlplane.NewRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate control plane: %v", err)
	}
	service := controlplane.NewService(repository)
	router := NewRouter(nil, WithControlPlaneService(service))

	ctx := context.Background()
	superID, err := service.CreateSuperAdminUser(ctx, "root", "test-super-password")
	if err != nil {
		t.Fatalf("seed super admin: %v", err)
	}
	return router, service, &controlplane.Actor{ID: superID, Username: "root"}
}

func requestWithPortalCookie(method, path, token string, body []byte) *http.Request {
	if body == nil {
		body = []byte{}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	if token != "" {
		request.AddCookie(&http.Cookie{Name: portalCookieInRouterTests, Value: token})
	}
	return request
}

func TestRouterIsolatesSuperAdminWithoutSupportAccess(t *testing.T) {
	router, service, superActor := newControlPlaneRouter(t)
	ctx := context.Background()
	tenant, err := service.CreateTenant(ctx, superActor, controlplane.TenantInput{Slug: "acme", DisplayName: "Acme"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	user, err := service.CreateUser(ctx, "member-one", "test-member-password")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := service.AddMembership(ctx, superActor, tenant.ID, user.ID, controlplane.RoleMember); err != nil {
		t.Fatalf("add membership: %v", err)
	}
	// member 会话经完整路由链访问组织 API（对照组：应可达）
	memberLogin, err := service.Login(ctx, "member-one", "test-member-password", "10.0.0.1")
	if err != nil {
		t.Fatalf("member login: %v", err)
	}
	memberProbe := httptest.NewRecorder()
	router.ServeHTTP(memberProbe, requestWithPortalCookie(http.MethodGet, "/api/portal/tenant/profile", memberLogin.Token, nil))
	if memberProbe.Code != http.StatusOK {
		t.Fatalf("member profile via router = %d body=%s", memberProbe.Code, memberProbe.Body.String())
	}

	// 隔离头条①（router 级）：超管未走支持访问审计，任何组织业务 API 不可达
	superLogin, err := service.Login(ctx, "root", "test-super-password", "10.0.0.2")
	if err != nil {
		t.Fatalf("super login: %v", err)
	}
	for _, path := range []string{
		"/api/portal/tenant/profile",
		"/api/portal/tenant/members",
		"/api/portal/tenant/invite-codes",
	} {
		probe := httptest.NewRecorder()
		router.ServeHTTP(probe, requestWithPortalCookie(http.MethodGet, path, superLogin.Token, nil))
		if probe.Code != http.StatusForbidden {
			t.Fatalf("super admin %s = %d, want 403 body=%s", path, probe.Code, probe.Body.String())
		}
	}
	// 走支持访问（理由必填、写审计）后：组织业务 API 可达
	enterBody, _ := json.Marshal(map[string]string{"reason": "router 级隔离验证"})
	enter := httptest.NewRecorder()
	enterRequest := requestWithPortalCookie(http.MethodPost,
		"/api/super-admin/tenants/"+strconv.FormatUint(uint64(tenant.ID), 10)+"/support-access",
		superLogin.Token, enterBody)
	enterRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(enter, enterRequest)
	if enter.Code != http.StatusOK {
		t.Fatalf("support access via router = %d body=%s", enter.Code, enter.Body.String())
	}
	reachable := httptest.NewRecorder()
	router.ServeHTTP(reachable, requestWithPortalCookie(http.MethodGet, "/api/portal/tenant/profile", superLogin.Token, nil))
	if reachable.Code != http.StatusOK {
		t.Fatalf("profile during support access = %d body=%s", reachable.Code, reachable.Body.String())
	}
}

func TestRouterMemberCannotReachTenantAdminAPIs(t *testing.T) {
	router, service, superActor := newControlPlaneRouter(t)
	ctx := context.Background()
	tenant, err := service.CreateTenant(ctx, superActor, controlplane.TenantInput{Slug: "acme", DisplayName: "Acme"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	user, err := service.CreateUser(ctx, "plain-member", "test-member-password")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := service.AddMembership(ctx, superActor, tenant.ID, user.ID, controlplane.RoleMember); err != nil {
		t.Fatalf("add membership: %v", err)
	}
	login, err := service.Login(ctx, "plain-member", "test-member-password", "10.0.0.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	// 隔离头条②（router 级）：member 对管理 API 403
	for _, spec := range []struct{ method, path string }{
		{http.MethodPost, "/api/portal/tenant/invite-codes"},
		{http.MethodDelete, "/api/portal/tenant/members/" + strconv.FormatUint(uint64(user.ID), 10)},
		{http.MethodGet, "/api/portal/tenant/members"},
	} {
		probe := httptest.NewRecorder()
		router.ServeHTTP(probe, requestWithPortalCookie(spec.method, spec.path, login.Token, nil))
		if probe.Code != http.StatusForbidden {
			t.Fatalf("member %s %s = %d, want 403 body=%s", spec.method, spec.path, probe.Code, probe.Body.String())
		}
	}
	// member 进超管命名空间：拒绝
	superProbe := httptest.NewRecorder()
	router.ServeHTTP(superProbe, requestWithPortalCookie(http.MethodGet, "/api/super-admin/tenants", login.Token, nil))
	if superProbe.Code != http.StatusForbidden {
		t.Fatalf("member in super admin namespace = %d, want 403 body=%s", superProbe.Code, superProbe.Body.String())
	}
}

func TestRouterRevokesRemovedMemberSessionImmediately(t *testing.T) {
	router, service, superActor := newControlPlaneRouter(t)
	ctx := context.Background()
	tenant, err := service.CreateTenant(ctx, superActor, controlplane.TenantInput{Slug: "acme", DisplayName: "Acme"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	user, err := service.CreateUser(ctx, "removed-soon", "test-member-password")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := service.AddMembership(ctx, superActor, tenant.ID, user.ID, controlplane.RoleMember); err != nil {
		t.Fatalf("add membership: %v", err)
	}
	memberLogin, err := service.Login(ctx, "removed-soon", "test-member-password", "10.0.0.1")
	if err != nil {
		t.Fatalf("member login: %v", err)
	}
	before := httptest.NewRecorder()
	router.ServeHTTP(before, requestWithPortalCookie(http.MethodGet, "/api/portal/tenant/profile", memberLogin.Token, nil))
	if before.Code != http.StatusOK {
		t.Fatalf("profile before removal = %d body=%s", before.Code, before.Body.String())
	}

	// 隔离头条③（router 级）：移除成员 → 会话即时失效
	if _, err := service.RemoveMember(ctx, superActor, tenant.ID, user.ID); err != nil {
		t.Fatalf("remove member: %v", err)
	}
	after := httptest.NewRecorder()
	router.ServeHTTP(after, requestWithPortalCookie(http.MethodGet, "/api/portal/tenant/profile", memberLogin.Token, nil))
	if after.Code != http.StatusUnauthorized {
		t.Fatalf("removed member session = %d, want 401 body=%s", after.Code, after.Body.String())
	}
}

// gin 默认信任一切代理，直连形态下 X-Forwarded-For 完全攻击者可控，
// 登录限速键（用户名+ClientIP）可被逐请求轮换 XFF 整体绕过。路由构造必须
// SetTrustedProxies(nil)，使 ClientIP 退化为直连对端地址。
func TestRouterIgnoresSpoofedForwardedFor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(nil)
	router.GET("/__ip_probe", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})
	request := httptest.NewRequest(http.MethodGet, "/__ip_probe", nil)
	request.Header.Set("X-Forwarded-For", "123.45.67.89")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Body.String() == "123.45.67.89" {
		t.Fatalf("router trusts attacker-controlled X-Forwarded-For, got %q", recorder.Body.String())
	}
}
