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

func fmtUint(value uint) string { return fmt.Sprint(value) }

// newSuperWorld：超管 handler 全量接线 + 组织/管理员/成员三个种子角色。
func newSuperWorld(t *testing.T) (*portalWorld, uint, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := store.Open("file:super-world-" + t.Name() + "?mode=memory&cache=shared")
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
	authHandler := NewControlPlaneHandler(service)
	superHandler := NewSuperAdminHandler(service)

	router := gin.New()
	portal := router.Group("/api/portal")
	authHandler.RegisterAuthRoutes(portal)
	authHandler.RegisterTenantRoutes(portal.Group("/tenant"))
	superGroup := router.Group("/api/super-admin")
	superGroup.Use(authHandler.RequirePortalAuth, authHandler.RequireSuperAdmin)
	superHandler.RegisterRoutes(superGroup)

	ctx := context.Background()
	superID, err := service.CreateSuperAdminUser(ctx, "root", "test-super-password")
	if err != nil {
		t.Fatalf("seed super admin: %v", err)
	}
	world := &portalWorld{service: service, router: router, superActor: &controlplane.Actor{ID: superID, Username: "root"}}
	tenantID := world.seedTenant(t, "acme")
	superToken := world.loginAsSuperAdmin(t)
	_ = world.seedUserWithRole(t, tenantID, "org-admin", controlplane.RoleTenantAdmin)
	_ = world.seedUserWithRole(t, tenantID, "plain-member", controlplane.RoleMember)
	return world, tenantID, superToken
}

func TestSuperAdminTenantCRUD(t *testing.T) {
	world, tenantID, superToken := newSuperWorld(t)

	listed := world.do(t, http.MethodGet, "/api/super-admin/tenants", superToken, nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("list tenants status = %d body=%s", listed.Code, listed.Body.String())
	}
	var listPayload struct {
		Tenants []map[string]any `json:"tenants"`
	}
	_ = json.Unmarshal(listed.Body.Bytes(), &listPayload)
	if len(listPayload.Tenants) != 1 || listPayload.Tenants[0]["member_count"].(float64) != 2 {
		t.Fatalf("expected 1 tenant with 2 members, got %v", listPayload.Tenants)
	}

	created := world.do(t, http.MethodPost, "/api/super-admin/tenants", superToken, map[string]string{
		"slug": "beta", "display_name": "Beta 组织",
	})
	if created.Code != http.StatusOK {
		t.Fatalf("create tenant status = %d body=%s", created.Code, created.Body.String())
	}
	duplicated := world.do(t, http.MethodPost, "/api/super-admin/tenants", superToken, map[string]string{
		"slug": "beta", "display_name": "重复",
	})
	if duplicated.Code != http.StatusConflict {
		t.Fatalf("duplicate slug status = %d", duplicated.Code)
	}

	patched := world.do(t, http.MethodPatch, "/api/super-admin/tenants/"+fmtUint(tenantID), superToken, map[string]string{
		"display_name": "改名后的组织",
	})
	if patched.Code != http.StatusOK {
		t.Fatalf("patch tenant status = %d body=%s", patched.Code, patched.Body.String())
	}
	disabled := world.do(t, http.MethodPost, "/api/super-admin/tenants/"+fmtUint(tenantID)+"/disable", superToken, nil)
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable status = %d body=%s", disabled.Code, disabled.Body.String())
	}
	restored := world.do(t, http.MethodPost, "/api/super-admin/tenants/"+fmtUint(tenantID)+"/restore", superToken, nil)
	if restored.Code != http.StatusOK {
		t.Fatalf("restore status = %d body=%s", restored.Code, restored.Body.String())
	}
}

func TestSuperAdminProvisionsTenantAdmin(t *testing.T) {
	world, tenantID, superToken := newSuperWorld(t)

	// 新账号开通：一次性初始密码 + 首登强制改密
	created := world.do(t, http.MethodPost, "/api/super-admin/tenants/"+fmtUint(tenantID)+"/admins", superToken, map[string]string{
		"username": "second-admin",
	})
	if created.Code != http.StatusOK {
		t.Fatalf("provision status = %d body=%s", created.Code, created.Body.String())
	}
	var provision struct {
		UserID          uint   `json:"user_id"`
		Username        string `json:"username"`
		InitialPassword string `json:"initial_password"`
	}
	_ = json.Unmarshal(created.Body.Bytes(), &provision)
	if provision.InitialPassword == "" {
		t.Fatal("expected initial password for new admin")
	}
	relogin, err := world.service.Login(context.Background(), "second-admin", provision.InitialPassword, "10.0.0.1")
	if err != nil {
		t.Fatalf("login with initial password: %v", err)
	}
	if !relogin.User.MustChangePassword {
		t.Fatal("expected must_change_password on provisioned admin")
	}

	// 已有普通用户授管理员：不改密、初始密码为空
	world.service.CreateUser(context.Background(), "existing-user", "test-existing-password")
	granted := world.do(t, http.MethodPost, "/api/super-admin/tenants/"+fmtUint(tenantID)+"/admins", superToken, map[string]string{
		"username": "existing-user",
	})
	if granted.Code != http.StatusOK {
		t.Fatalf("grant existing status = %d body=%s", granted.Code, granted.Body.String())
	}
	var grantedPayload struct {
		InitialPassword string `json:"initial_password"`
	}
	_ = json.Unmarshal(granted.Body.Bytes(), &grantedPayload)
	if grantedPayload.InitialPassword != "" {
		t.Fatal("existing user must keep their password")
	}
}

func TestSuperAdminResetsTenantAdminPassword(t *testing.T) {
	world, tenantID, superToken := newSuperWorld(t)
	target := resolveUserID(t, world.service, "org-admin")
	reset := world.do(t, http.MethodPost, "/api/super-admin/tenants/"+fmtUint(tenantID)+"/admins/"+target+"/reset-password", superToken, nil)
	if reset.Code != http.StatusOK {
		t.Fatalf("reset admin password status = %d body=%s", reset.Code, reset.Body.String())
	}
	var payload struct {
		InitialPassword string `json:"initial_password"`
	}
	_ = json.Unmarshal(reset.Body.Bytes(), &payload)
	if payload.InitialPassword == "" {
		t.Fatal("expected initial password")
	}
	// member 不是管理员：重置拒绝
	member := resolveUserID(t, world.service, "plain-member")
	denied := world.do(t, http.MethodPost, "/api/super-admin/tenants/"+fmtUint(tenantID)+"/admins/"+member+"/reset-password", superToken, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("reset member via admin channel should be forbidden, got %d", denied.Code)
	}
}

func TestSuperAdminInviteCodesAndRunnerAssignments(t *testing.T) {
	world, tenantID, superToken := newSuperWorld(t)
	// 组织管理员签发一枚码
	adminToken := world.seedUserWithRole(t, tenantID, "invite-issuer", controlplane.RoleTenantAdmin)
	issued := world.do(t, http.MethodPost, "/api/portal/tenant/invite-codes", adminToken, map[string]any{"max_uses": 5})
	if issued.Code != http.StatusOK {
		t.Fatalf("issue invite status = %d", issued.Code)
	}
	var issuedPayload struct {
		Record struct {
			ID uint `json:"id"`
		} `json:"record"`
	}
	_ = json.Unmarshal(issued.Body.Bytes(), &issuedPayload)

	listed := world.do(t, http.MethodGet, "/api/super-admin/tenants/"+fmtUint(tenantID)+"/invite-codes", superToken, nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("super list invites status = %d body=%s", listed.Code, listed.Body.String())
	}
	revoked := world.do(t, http.MethodDelete, "/api/super-admin/tenants/"+fmtUint(tenantID)+"/invite-codes/"+fmtUint(issuedPayload.Record.ID), superToken, nil)
	if revoked.Code != http.StatusOK {
		t.Fatalf("super revoke invite status = %d body=%s", revoked.Code, revoked.Body.String())
	}

	assigned := world.do(t, http.MethodPut, "/api/super-admin/tenants/"+fmtUint(tenantID)+"/runner-assignments", superToken, map[string]any{
		"endpoints": []string{"http://runner-acme:8081", "http://runner-acme-2:8081"},
	})
	if assigned.Code != http.StatusOK {
		t.Fatalf("assign runners status = %d body=%s", assigned.Code, assigned.Body.String())
	}
	invalid := world.do(t, http.MethodPut, "/api/super-admin/tenants/"+fmtUint(tenantID)+"/runner-assignments", superToken, map[string]any{
		"endpoints": []string{"ftp://not-allowed"},
	})
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid endpoint should be rejected, got %d", invalid.Code)
	}
	listedRunners := world.do(t, http.MethodGet, "/api/super-admin/tenants/"+fmtUint(tenantID)+"/runner-assignments", superToken, nil)
	if listedRunners.Code != http.StatusOK {
		t.Fatalf("list runners status = %d", listedRunners.Code)
	}
	var runnerPayload struct {
		Endpoints []map[string]any `json:"runner_assignments"`
	}
	_ = json.Unmarshal(listedRunners.Body.Bytes(), &runnerPayload)
	if len(runnerPayload.Endpoints) != 2 {
		t.Fatalf("expected 2 runner assignments, got %d", len(runnerPayload.Endpoints))
	}
}

func TestSuperAdminSupportAccessLifecycle(t *testing.T) {
	world, tenantID, superToken := newSuperWorld(t)

	// 未走支持访问：组织业务 API 不可达（隔离头条①已测，这里验证进入后可达）
	blocked := world.do(t, http.MethodGet, "/api/portal/tenant/members", superToken, nil)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("pre-support access should be forbidden, got %d", blocked.Code)
	}
	// 理由必填
	noReason := world.do(t, http.MethodPost, "/api/super-admin/tenants/"+fmtUint(tenantID)+"/support-access", superToken, map[string]string{"reason": "  "})
	if noReason.Code != http.StatusBadRequest {
		t.Fatalf("empty reason should be rejected, got %d body=%s", noReason.Code, noReason.Body.String())
	}
	entered := world.do(t, http.MethodPost, "/api/super-admin/tenants/"+fmtUint(tenantID)+"/support-access", superToken, map[string]string{
		"reason": "排障：组织反馈成员列表异常",
	})
	if entered.Code != http.StatusOK {
		t.Fatalf("support access status = %d body=%s", entered.Code, entered.Body.String())
	}
	reachable := world.do(t, http.MethodGet, "/api/portal/tenant/members", superToken, nil)
	if reachable.Code != http.StatusOK {
		t.Fatalf("members should be reachable during support access, got %d body=%s", reachable.Code, reachable.Body.String())
	}
	exited := world.do(t, http.MethodDelete, "/api/super-admin/support-access", superToken, nil)
	if exited.Code != http.StatusOK {
		t.Fatalf("exit support access status = %d body=%s", exited.Code, exited.Body.String())
	}
	blockedAgain := world.do(t, http.MethodGet, "/api/portal/tenant/members", superToken, nil)
	if blockedAgain.Code != http.StatusForbidden {
		t.Fatalf("members should be forbidden after exit, got %d", blockedAgain.Code)
	}
	// 审计：entered 与 exited 各一条
	audit := world.do(t, http.MethodGet, "/api/super-admin/audit-log?action=support_access.entered", superToken, nil)
	if audit.Code != http.StatusOK {
		t.Fatalf("audit log status = %d body=%s", audit.Code, audit.Body.String())
	}
	var auditPayload struct {
		Items []map[string]any `json:"items"`
		Total float64          `json:"total"`
	}
	_ = json.Unmarshal(audit.Body.Bytes(), &auditPayload)
	if len(auditPayload.Items) != 1 || auditPayload.Total != 1 {
		t.Fatalf("expected exactly 1 entered audit, got %v", auditPayload)
	}
}

func TestSuperAdminNamespaceRejectsNonSuperAdmin(t *testing.T) {
	world, tenantID, _ := newSuperWorld(t)
	memberToken := world.seedUserWithRole(t, tenantID, "another-member", controlplane.RoleMember)
	denied := world.do(t, http.MethodGet, "/api/super-admin/tenants", memberToken, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("member in super admin namespace should be 403, got %d", denied.Code)
	}
}
