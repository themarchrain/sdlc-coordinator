package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/store"

	"github.com/gin-gonic/gin"
)

type portalWorld struct {
	service    *controlplane.Service
	router     *gin.Engine
	superActor *controlplane.Actor
}

func newPortalWorld(t *testing.T) *portalWorld {
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
	handler := NewControlPlaneHandler(service)

	router := gin.New()
	portal := router.Group("/api/portal")
	handler.RegisterAuthRoutes(portal)
	handler.RegisterTenantRoutes(portal.Group("/tenant"))

	ctx := context.Background()
	superID, err := service.CreateSuperAdminUser(ctx, "root", "test-super-password")
	if err != nil {
		t.Fatalf("seed super admin: %v", err)
	}
	world := &portalWorld{service: service, router: router, superActor: &controlplane.Actor{ID: superID, Username: "root"}}
	return world
}

// seedTenant 建组织并返回 id。
func (world *portalWorld) seedTenant(t *testing.T, slug string) uint {
	t.Helper()
	tenant, err := world.service.CreateTenant(context.Background(), world.superActor, controlplane.TenantInput{Slug: slug, DisplayName: slug + " 组织"})
	if err != nil {
		t.Fatalf("seed tenant %s: %v", slug, err)
	}
	return tenant.ID
}

// seedUserWithRole 建号入会并登录，返回会话 token。
func (world *portalWorld) seedUserWithRole(t *testing.T, tenantID uint, username, role string) string {
	t.Helper()
	ctx := context.Background()
	user, err := world.service.CreateUser(ctx, username, "test-member-password")
	if err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	if _, err := world.service.AddMembership(ctx, world.superActor, tenantID, user.ID, role); err != nil {
		t.Fatalf("add membership: %v", err)
	}
	result, err := world.service.Login(ctx, username, "test-member-password", "10.0.0.1")
	if err != nil {
		t.Fatalf("login %s: %v", username, err)
	}
	return result.Token
}

// loginAsSuperAdmin 超管登录拿 token。
func (world *portalWorld) loginAsSuperAdmin(t *testing.T) string {
	t.Helper()
	result, err := world.service.Login(context.Background(), "root", "test-super-password", "10.0.0.2")
	if err != nil {
		t.Fatalf("super admin login: %v", err)
	}
	return result.Token
}

func (world *portalWorld) do(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.AddCookie(&http.Cookie{Name: PortalSessionCookieName, Value: token})
	}
	recorder := httptest.NewRecorder()
	world.router.ServeHTTP(recorder, request)
	return recorder
}

func decodeBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response %s: %v", recorder.Body.String(), err)
	}
	return payload
}

func TestPortalLoginSessionAndLogoutFlow(t *testing.T) {
	world := newPortalWorld(t)
	tenantID := world.seedTenant(t, "acme")
	world.seedUserWithRole(t, tenantID, "flow-user", controlplane.RoleMember)

	unauth := world.do(t, http.MethodGet, "/api/portal/auth/session", "", nil)
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated session status = %d", unauth.Code)
	}

	login := world.do(t, http.MethodPost, "/api/portal/auth/login", "", map[string]string{
		"username": "flow-user", "password": "test-member-password",
	})
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", login.Code, login.Body.String())
	}
	cookies := login.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, item := range cookies {
		if item.Name == PortalSessionCookieName {
			sessionCookie = item
		}
	}
	if sessionCookie == nil || !sessionCookie.HttpOnly {
		t.Fatalf("expected HttpOnly portal session cookie, got %#v", cookies)
	}

	session := world.do(t, http.MethodGet, "/api/portal/auth/session", sessionCookie.Value, nil)
	if session.Code != http.StatusOK {
		t.Fatalf("session status = %d body=%s", session.Code, session.Body.String())
	}
	payload := decodeBody(t, session)
	if payload["user"] == nil || payload["scope"] == nil {
		t.Fatalf("expected user and scope in session payload, got %v", payload)
	}
	scope := payload["scope"].(map[string]any)
	if scope["kind"] != controlplane.ScopeTenant {
		t.Fatalf("expected tenant scope kind, got %v", scope["kind"])
	}

	logout := world.do(t, http.MethodPost, "/api/portal/auth/logout", sessionCookie.Value, nil)
	if logout.Code != http.StatusOK {
		t.Fatalf("logout status = %d", logout.Code)
	}
	afterLogout := world.do(t, http.MethodGet, "/api/portal/auth/session", sessionCookie.Value, nil)
	if afterLogout.Code != http.StatusUnauthorized {
		t.Fatalf("session after logout should be unauthorized, got %d", afterLogout.Code)
	}
}

func TestPortalForcesPasswordChangeBeforeOtherUse(t *testing.T) {
	world := newPortalWorld(t)
	tenantID := world.seedTenant(t, "acme")
	ctx := context.Background()
	user, err := world.service.CreateUser(ctx, "fresh-admin", "test-initial-password")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := world.service.AddMembership(ctx, world.superActor, tenantID, user.ID, controlplane.RoleTenantAdmin); err != nil {
		t.Fatalf("add membership: %v", err)
	}
	if err := world.service.MarkMustChangePassword(ctx, user.ID, true); err != nil {
		t.Fatalf("mark must change: %v", err)
	}
	result, err := world.service.Login(ctx, "fresh-admin", "test-initial-password", "10.0.0.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	blocked := world.do(t, http.MethodGet, "/api/portal/tenant/members", result.Token, nil)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("expected 403 while must change password, got %d body=%s", blocked.Code, blocked.Body.String())
	}
	if errCode := decodeBody(t, blocked)["error"].(map[string]any)["code"]; errCode != "password_change_required" {
		t.Fatalf("expected password_change_required, got %v", errCode)
	}
	changed := world.do(t, http.MethodPost, "/api/portal/auth/change-password", result.Token, map[string]string{
		"current_password": "test-initial-password", "new_password": "test-changed-password",
	})
	if changed.Code != http.StatusOK {
		t.Fatalf("change password status = %d body=%s", changed.Code, changed.Body.String())
	}
	// 改密吊销会话：需重新登录
	relogin := world.do(t, http.MethodPost, "/api/portal/auth/login", "", map[string]string{
		"username": "fresh-admin", "password": "test-changed-password",
	})
	if relogin.Code != http.StatusOK {
		t.Fatalf("relogin status = %d body=%s", relogin.Code, relogin.Body.String())
	}
	var loginPayload struct {
		Scope struct {
			Kind string `json:"kind"`
		} `json:"scope"`
	}
	_ = json.Unmarshal(relogin.Body.Bytes(), &loginPayload)
	if loginPayload.Scope.Kind != controlplane.ScopeTenant {
		t.Fatalf("expected tenant scope after relogin, got %q", loginPayload.Scope.Kind)
	}
}

func TestPortalSelectTenantForMultiTenantUser(t *testing.T) {
	world := newPortalWorld(t)
	tenantA := world.seedTenant(t, "alpha")
	tenantB := world.seedTenant(t, "beta")
	ctx := context.Background()
	user, err := world.service.CreateUser(ctx, "multi-user", "test-member-password")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	for _, tenantID := range []uint{tenantA, tenantB} {
		if _, err := world.service.AddMembership(ctx, world.superActor, tenantID, user.ID, controlplane.RoleMember); err != nil {
			t.Fatalf("add membership: %v", err)
		}
	}
	login := world.do(t, http.MethodPost, "/api/portal/auth/login", "", map[string]string{
		"username": "multi-user", "password": "test-member-password",
	})
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", login.Code, login.Body.String())
	}
	var loginPayload struct {
		Scope struct {
			Kind string `json:"kind"`
		} `json:"scope"`
		Tenants []map[string]any `json:"tenants"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &loginPayload); err != nil {
		t.Fatalf("decode login payload: %v", err)
	}
	if loginPayload.Scope.Kind != controlplane.ScopeUnscoped || len(loginPayload.Tenants) != 2 {
		t.Fatalf("expected unscoped scope with 2 tenants, got kind=%q tenants=%d", loginPayload.Scope.Kind, len(loginPayload.Tenants))
	}
	unscopedCookie := extractCookie(t, login, PortalSessionCookieName)
	if unscopedCookie == "" {
		t.Fatal("expected portal session cookie on unscoped login")
	}
	// 选择组织：会话换发，新 cookie 下发，旧 token 失效
	selectTenant := world.do(t, http.MethodPost, "/api/portal/auth/select-tenant", unscopedCookie, map[string]float64{
		"tenant_id": float64(tenantB),
	})
	if selectTenant.Code != http.StatusOK {
		t.Fatalf("select tenant status = %d body=%s", selectTenant.Code, selectTenant.Body.String())
	}
	reissued := extractCookie(t, selectTenant, PortalSessionCookieName)
	if reissued == "" || reissued == unscopedCookie {
		t.Fatalf("select tenant must re-issue session cookie, got %q", reissued)
	}
	stale := world.do(t, http.MethodGet, "/api/portal/auth/session", unscopedCookie, nil)
	if stale.Code != http.StatusUnauthorized {
		t.Fatalf("old token should be invalid, got %d", stale.Code)
	}
}

// extractCookie 从响应里取指定 cookie 值。
func extractCookie(t *testing.T, recorder *httptest.ResponseRecorder, name string) string {
	t.Helper()
	for _, item := range recorder.Result().Cookies() {
		if item.Name == name {
			return item.Value
		}
	}
	return ""
}

func TestTenantScopeMiddlewareSemantics(t *testing.T) {
	world := newPortalWorld(t)
	tenantID := world.seedTenant(t, "acme")
	memberToken := world.seedUserWithRole(t, tenantID, "plain-member", controlplane.RoleMember)
	adminToken := world.seedUserWithRole(t, tenantID, "org-admin", controlplane.RoleTenantAdmin)
	superToken := world.loginAsSuperAdmin(t)

	// 头条①：超管未走支持访问审计时，租户业务 API 不可达
	superBlocked := world.do(t, http.MethodGet, "/api/portal/tenant/members", superToken, nil)
	if superBlocked.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for super admin without support access, got %d body=%s", superBlocked.Code, superBlocked.Body.String())
	}
	if code := decodeBody(t, superBlocked)["error"].(map[string]any)["code"]; code != "super_admin_scope_required" {
		t.Fatalf("expected super_admin_scope_required, got %v", code)
	}

	// 头条②：member 对管理 API 403
	memberBlocked := world.do(t, http.MethodPost, "/api/portal/tenant/invite-codes", memberToken, map[string]any{
		"expires_in_hours": 24, "max_uses": 5,
	})
	if memberBlocked.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for member on admin API, got %d body=%s", memberBlocked.Code, memberBlocked.Body.String())
	}

	// tenant_admin 可读成员列表
	adminOK := world.do(t, http.MethodGet, "/api/portal/tenant/members", adminToken, nil)
	if adminOK.Code != http.StatusOK {
		t.Fatalf("tenant admin members status = %d body=%s", adminOK.Code, adminOK.Body.String())
	}
}

func TestPortalRegisterEndpoint(t *testing.T) {
	world := newPortalWorld(t)
	tenantID := world.seedTenant(t, "acme")
	ctx := context.Background()
	adminUser, err := world.service.CreateUser(ctx, "org-admin", "test-admin-password")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if _, err := world.service.AddMembership(ctx, world.superActor, tenantID, adminUser.ID, controlplane.RoleTenantAdmin); err != nil {
		t.Fatalf("add membership: %v", err)
	}
	code, err := world.service.IssueInviteCode(ctx, &controlplane.Actor{ID: adminUser.ID, Username: "org-admin"}, tenantID, controlplane.InviteCodeInput{})
	if err != nil {
		t.Fatalf("issue invite: %v", err)
	}
	register := world.do(t, http.MethodPost, "/api/portal/auth/register", "", map[string]string{
		"username": "newcomer", "password": "test-registered-password", "invite_code": code.Plaintext,
	})
	if register.Code != http.StatusOK {
		t.Fatalf("register status = %d body=%s", register.Code, register.Body.String())
	}
	badRegister := world.do(t, http.MethodPost, "/api/portal/auth/register", "", map[string]string{
		"username": "newcomer2", "password": "test-registered-password", "invite_code": "AAAA-AAAA-AAAA",
	})
	if badRegister.Code != http.StatusBadRequest {
		t.Fatalf("invalid invite status = %d body=%s", badRegister.Code, badRegister.Body.String())
	}
}

func TestPortalRateLimitedLoginReturns429(t *testing.T) {
	world := newPortalWorld(t)
	for i := 0; i < 10; i++ {
		world.do(t, http.MethodPost, "/api/portal/auth/login", "", map[string]string{
			"username": "ghost-user", "password": fmt.Sprintf("wrong-%02d-password", i),
		})
	}
	limited := world.do(t, http.MethodPost, "/api/portal/auth/login", "", map[string]string{
		"username": "ghost-user", "password": "another-wrong-pass",
	})
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d body=%s", limited.Code, limited.Body.String())
	}
}
