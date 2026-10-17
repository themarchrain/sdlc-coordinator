package handlers

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/store"
)

// newMultiAccountWorld 是多账号切换测试的最小装配：门户认证路由 + 两个成员
// 会话。portalWorld 定义在 controlplane_test.go，本文件只补专用场景。
func newMultiAccountWorld(t *testing.T) (*portalWorld, string, string) {
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
	handler.RegisterAuthRoutes(router.Group("/api/portal"))

	ctx := context.Background()
	superID, err := service.CreateSuperAdminUser(ctx, "root", "test-super-password")
	if err != nil {
		t.Fatalf("seed super admin: %v", err)
	}
	world := &portalWorld{service: service, router: router, superActor: &controlplane.Actor{ID: superID, Username: "root"}, db: db}

	tenant, err := service.CreateTenant(ctx, world.superActor, controlplane.TenantInput{Slug: "acme", DisplayName: "acme 组织"})
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	userA, err := service.CreateUser(ctx, "switch-user-a", "test-member-password")
	if err != nil {
		t.Fatalf("seed user a: %v", err)
	}
	if _, err := service.AddMembership(ctx, world.superActor, tenant.ID, userA.ID, controlplane.RoleMember); err != nil {
		t.Fatalf("seed membership a: %v", err)
	}
	userB, err := service.CreateUser(ctx, "switch-user-b", "test-member-password")
	if err != nil {
		t.Fatalf("seed user b: %v", err)
	}
	if _, err := service.AddMembership(ctx, world.superActor, tenant.ID, userB.ID, controlplane.RoleMember); err != nil {
		t.Fatalf("seed membership b: %v", err)
	}
	loginA, err := service.Login(ctx, "switch-user-a", "test-member-password", "10.7.0.1")
	if err != nil {
		t.Fatalf("login a: %v", err)
	}
	loginB, err := service.Login(ctx, "switch-user-b", "test-member-password", "10.7.0.2")
	if err != nil {
		t.Fatalf("login b: %v", err)
	}
	return world, loginA.Token, loginB.Token
}

// TestSwitchAccountRewritesSessionCookie 锁定核心语义：切换成功后响应下发的
// 会话 cookie 就是目标 token，且用它可以查到目标账号的会话。
func TestSwitchAccountRewritesSessionCookie(t *testing.T) {
	world, tokenA, tokenB := newMultiAccountWorld(t)

	// 未认证携带无效 cookie：中间件先拒。
	denied := world.do(t, http.MethodPost, "/api/portal/auth/switch-account", "", map[string]string{"token": tokenB})
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated switch status = %d", denied.Code)
	}

	switched := world.do(t, http.MethodPost, "/api/portal/auth/switch-account", tokenA, map[string]string{"token": tokenB})
	if switched.Code != http.StatusOK {
		t.Fatalf("switch status = %d body=%s", switched.Code, switched.Body.String())
	}
	if got := extractCookie(t, switched, PortalSessionCookieName); got != tokenB {
		t.Fatalf("session cookie must carry the target token after switching")
	}

	session := world.do(t, http.MethodGet, "/api/portal/auth/session", tokenB, nil)
	if session.Code != http.StatusOK {
		t.Fatalf("session with switched token status = %d", session.Code)
	}
	payload := decodeBody(t, session)
	user := payload["user"].(map[string]any)
	if user["username"] != "switch-user-b" {
		t.Fatalf("expected switched identity switch-user-b, got %v", user["username"])
	}
}

func TestSwitchAccountRejectsUnknownToken(t *testing.T) {
	world, tokenA, _ := newMultiAccountWorld(t)
	resp := world.do(t, http.MethodPost, "/api/portal/auth/switch-account", tokenA, map[string]string{"token": "bogus-token"})
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("unknown target token must be rejected, got %d", resp.Code)
	}
	if extractCookie(t, resp, PortalSessionCookieName) != "" {
		t.Fatalf("failed switch must not rewrite the session cookie")
	}
}

func TestRevokeSavedClearsCookieAndRevokesTokens(t *testing.T) {
	world, tokenA, tokenB := newMultiAccountWorld(t)

	resp := world.do(t, http.MethodPost, "/api/portal/auth/revoke-saved", tokenA, map[string]any{
		"tokens": []string{tokenA, tokenB, "ghost-token"},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("revoke saved status = %d body=%s", resp.Code, resp.Body.String())
	}
	cleared := false
	for _, item := range resp.Result().Cookies() {
		if item.Name == PortalSessionCookieName && item.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatalf("revoke saved should clear the session cookie, got %#v", resp.Result().Cookies())
	}
	for _, token := range []string{tokenA, tokenB} {
		session := world.do(t, http.MethodGet, "/api/portal/auth/session", token, nil)
		if session.Code != http.StatusUnauthorized {
			t.Fatalf("revoked token must no longer authenticate, got %d", session.Code)
		}
	}
}

func TestRevokeSavedAcceptsEmptyTokenList(t *testing.T) {
	world, tokenA, _ := newMultiAccountWorld(t)
	resp := world.do(t, http.MethodPost, "/api/portal/auth/revoke-saved", tokenA, map[string]any{"tokens": []string{}})
	if resp.Code != http.StatusOK {
		t.Fatalf("empty list status = %d body=%s", resp.Code, resp.Body.String())
	}
	// 空列表只清本端 cookie：当前会话本身已被服务端删除语义覆盖（cookie 清除）。
	if got := extractCookie(t, resp, PortalSessionCookieName); got != "" {
		t.Fatalf("empty list should still clear the session cookie")
	}
}

// TestLoginResponseCarriesSessionToken 锁定注册表数据来源：登录响应体必须
// 带 session_token，且 session 查询端点不得回显。
func TestLoginResponseCarriesSessionToken(t *testing.T) {
	world, _, _ := newMultiAccountWorld(t)

	login := world.do(t, http.MethodPost, "/api/portal/auth/login", "", map[string]string{
		"username": "switch-user-a", "password": "test-member-password",
	})
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d", login.Code)
	}
	payload := decodeBody(t, login)
	token, ok := payload["session_token"].(string)
	if !ok || token == "" {
		t.Fatalf("login payload must carry session_token, got %v", payload["session_token"])
	}
	if token != extractCookie(t, login, PortalSessionCookieName) {
		t.Fatalf("session_token must match the issued session cookie")
	}

	session := world.do(t, http.MethodGet, "/api/portal/auth/session", token, nil)
	if session.Code != http.StatusOK {
		t.Fatalf("session status = %d", session.Code)
	}
	if _, exists := decodeBody(t, session)["session_token"]; exists {
		t.Fatalf("session endpoint must not echo the session token")
	}
}
