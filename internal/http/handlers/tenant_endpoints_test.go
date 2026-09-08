package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/store"

	"github.com/gin-gonic/gin"
)

// resolveUserID 按用户名取 id（路径参数用字符串形态）。
func resolveUserID(t *testing.T, service *controlplane.Service, username string) string {
	t.Helper()
	user, err := service.GetUserByUsername(context.Background(), username)
	if err != nil || user == nil {
		t.Fatalf("resolve user %s: %v", username, err)
	}
	return fmt.Sprint(user.ID)
}

// newTenantWorld 在 portalWorld 基础上补齐一个组织与两个账号：
// org-admin（tenant_admin）与 plain-member（member）。
func newTenantWorld(t *testing.T) (*portalWorld, uint, string, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := store.Open("file:tenant-world-" + t.Name() + "?mode=memory&cache=shared")
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
	tenantID := world.seedTenant(t, "acme")
	adminToken := world.seedUserWithRole(t, tenantID, "org-admin", controlplane.RoleTenantAdmin)
	memberToken := world.seedUserWithRole(t, tenantID, "plain-member", controlplane.RoleMember)
	return world, tenantID, adminToken, memberToken
}

func TestTenantMemberManagementEndpoints(t *testing.T) {
	world, _, adminToken, memberToken := newTenantWorld(t)

	profile := world.do(t, http.MethodGet, "/api/portal/tenant/profile", adminToken, nil)
	if profile.Code != http.StatusOK {
		t.Fatalf("profile status = %d body=%s", profile.Code, profile.Body.String())
	}
	profilePayload := decodeBody(t, profile)
	if profilePayload["tenant"] == nil {
		t.Fatalf("expected tenant in profile, got %v", profilePayload)
	}

	members := world.do(t, http.MethodGet, "/api/portal/tenant/members", adminToken, nil)
	if members.Code != http.StatusOK {
		t.Fatalf("members status = %d body=%s", members.Code, members.Body.String())
	}
	var memberPayload struct {
		Members []map[string]any `json:"members"`
	}
	_ = json.Unmarshal(members.Body.Bytes(), &memberPayload)
	if len(memberPayload.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(memberPayload.Members))
	}

	// 不能移除自己
	selfRemove := world.do(t, http.MethodDelete, "/api/portal/tenant/members/"+resolveUserID(t, world.service, "org-admin"), adminToken, nil)
	if selfRemove.Code != http.StatusBadRequest {
		t.Fatalf("self remove status = %d body=%s", selfRemove.Code, selfRemove.Body.String())
	}

	// 移除普通成员：会话即时失效（隔离头条③的 handler 层证据）
	removed := world.do(t, http.MethodDelete, "/api/portal/tenant/members/"+resolveUserID(t, world.service, "plain-member"), adminToken, nil)
	if removed.Code != http.StatusOK {
		t.Fatalf("remove member status = %d body=%s", removed.Code, removed.Body.String())
	}
	probe := world.do(t, http.MethodGet, "/api/portal/auth/session", memberToken, nil)
	if probe.Code != http.StatusUnauthorized {
		t.Fatalf("removed member session should be revoked immediately, got %d", probe.Code)
	}
}

func TestTenantAdminResetsMemberPassword(t *testing.T) {
	world, _, adminToken, memberToken := newTenantWorld(t)
	target := resolveUserID(t, world.service, "plain-member")

	reset := world.do(t, http.MethodPost, "/api/portal/tenant/members/"+target+"/reset-password", adminToken, nil)
	if reset.Code != http.StatusOK {
		t.Fatalf("reset status = %d body=%s", reset.Code, reset.Body.String())
	}
	var resetPayload struct {
		InitialPassword string `json:"initial_password"`
	}
	_ = json.Unmarshal(reset.Body.Bytes(), &resetPayload)
	if len(resetPayload.InitialPassword) < 8 {
		t.Fatalf("expected generated initial password, got %q", resetPayload.InitialPassword)
	}
	// 目标用户既有会话即时吊销
	probe := world.do(t, http.MethodGet, "/api/portal/auth/session", memberToken, nil)
	if probe.Code != http.StatusUnauthorized {
		t.Fatalf("target session should be revoked after password reset, got %d", probe.Code)
	}
	// 新初始密码可登录，且强制改密
	relogin, err := world.service.Login(context.Background(), "plain-member", resetPayload.InitialPassword, "10.0.0.1")
	if err != nil {
		t.Fatalf("login with reset password: %v", err)
	}
	if !relogin.User.MustChangePassword {
		t.Fatal("expected must_change_password after reset")
	}
}

func TestTenantInviteCodeEndpoints(t *testing.T) {
	world, _, adminToken, _ := newTenantWorld(t)

	created := world.do(t, http.MethodPost, "/api/portal/tenant/invite-codes", adminToken, map[string]any{
		"expires_in_hours": 48, "max_uses": 3,
	})
	if created.Code != http.StatusOK {
		t.Fatalf("create invite status = %d body=%s", created.Code, created.Body.String())
	}
	var createdPayload struct {
		Code   string         `json:"code"`
		Record map[string]any `json:"record"`
	}
	_ = json.Unmarshal(created.Body.Bytes(), &createdPayload)
	if createdPayload.Code == "" || createdPayload.Record["code_hash"] != nil {
		t.Fatalf("expected one-time plaintext without hash, got %v", createdPayload)
	}

	listed := world.do(t, http.MethodGet, "/api/portal/tenant/invite-codes", adminToken, nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("list invites status = %d body=%s", listed.Code, listed.Body.String())
	}
	var listPayload struct {
		InviteCodes []map[string]any `json:"invite_codes"`
	}
	_ = json.Unmarshal(listed.Body.Bytes(), &listPayload)
	if len(listPayload.InviteCodes) != 1 || listPayload.InviteCodes[0]["code_hash"] != nil {
		t.Fatalf("expected 1 invite code without hash leak, got %v", listPayload.InviteCodes)
	}

	codeID := int(listPayload.InviteCodes[0]["id"].(float64))
	revoked := world.do(t, http.MethodDelete, "/api/portal/tenant/invite-codes/"+fmt.Sprint(codeID), adminToken, nil)
	if revoked.Code != http.StatusOK {
		t.Fatalf("revoke invite status = %d body=%s", revoked.Code, revoked.Body.String())
	}
}
