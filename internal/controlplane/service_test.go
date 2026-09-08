package controlplane

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLoginCreatesTenantScopedSession(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	tenantID, _ := seedTenantWithAdmin(t, service, "acme")
	// 普通成员账号
	user, err := service.CreateUser(ctx, "Member01", "test-password-001")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := service.AddMembership(ctx, &testActor, tenantID, user.ID, RoleMember); err != nil {
		t.Fatalf("add membership: %v", err)
	}
	if _, err := service.Login(ctx, "member01", "wrong-password", "10.0.0.1"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
	result, err := service.Login(ctx, "  Member01  ", "test-password-001", "10.0.0.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result.Scope.Kind != ScopeTenant {
		t.Fatalf("expected tenant scope, got %q", result.Scope.Kind)
	}
	if result.Scope.Tenant == nil || result.Scope.Tenant.ID != tenantID {
		t.Fatalf("expected bound tenant %d, got %+v", tenantID, result.Scope.Tenant)
	}
	if result.Scope.Role != RoleMember {
		t.Fatalf("expected member role, got %q", result.Scope.Role)
	}
	sessionCtx, err := service.ValidateSession(ctx, result.Token)
	if err != nil {
		t.Fatalf("validate session: %v", err)
	}
	if sessionCtx.User.ID != user.ID || sessionCtx.EffectiveTenant.ID != tenantID {
		t.Fatalf("unexpected session context %+v", sessionCtx)
	}
}

func TestLoginRateLimitPerUsernameAndIP(t *testing.T) {
	repository := openTestControlPlane(t)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	service := NewService(repository, WithRateLimit(time.Hour, 3))
	ctx := context.Background()
	tenantID, _ := seedTenantWithAdmin(t, service, "acme")
	user, err := service.CreateUser(ctx, "limited", "test-password-002")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := service.AddMembership(ctx, &testActor, tenantID, user.ID, RoleMember); err != nil {
		t.Fatalf("add membership: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := service.Login(ctx, "limited", "wrong-password", "10.0.0.9"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: expected ErrInvalidCredentials, got %v", i+1, err)
		}
	}
	// 正确密码也被限速拒绝
	if _, err := service.Login(ctx, "limited", "test-password-002", "10.0.0.9"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
	// 换 IP 不受限速影响（按用户名+IP 组合计数）
	if _, err := service.Login(ctx, "limited", "test-password-002", "10.0.0.10"); err != nil {
		t.Fatalf("different ip should not be limited: %v", err)
	}
	// 换用户名不受限速影响
	other, err := service.CreateUser(ctx, "other-user", "test-password-003")
	if err != nil {
		t.Fatalf("create other user: %v", err)
	}
	if _, err := service.AddMembership(ctx, &testActor, tenantID, other.ID, RoleMember); err != nil {
		t.Fatalf("add membership: %v", err)
	}
	if _, err := service.Login(ctx, "other-user", "test-password-003", "10.0.0.9"); err != nil {
		t.Fatalf("different username should not be limited: %v", err)
	}
}

func TestLoginMultiTenantRequiresSelection(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	tenantA, _ := seedTenantWithAdmin(t, service, "alpha")
	tenantB, _ := seedTenantWithAdmin(t, service, "beta")
	user, err := service.CreateUser(ctx, "multi-user", "test-password-004")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := service.AddMembership(ctx, &testActor, tenantA, user.ID, RoleMember); err != nil {
		t.Fatalf("add membership a: %v", err)
	}
	if _, err := service.AddMembership(ctx, &testActor, tenantB, user.ID, RoleTenantAdmin); err != nil {
		t.Fatalf("add membership b: %v", err)
	}
	result, err := service.Login(ctx, "multi-user", "test-password-004", "10.0.0.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result.Scope.Kind != ScopeUnscoped {
		t.Fatalf("expected unscoped scope, got %q", result.Scope.Kind)
	}
	if len(result.Tenants) != 2 {
		t.Fatalf("expected 2 selectable tenants, got %d", len(result.Tenants))
	}
	// 选择组织 = 换发会话：新 token 生效，旧 token 失效
	switched, err := service.SelectTenant(ctx, result.Token, tenantB)
	if err != nil {
		t.Fatalf("select tenant: %v", err)
	}
	if switched.Scope.Kind != ScopeTenant || switched.Scope.Role != RoleTenantAdmin {
		t.Fatalf("expected tenant admin scope, got %+v", switched.Scope)
	}
	if _, err := service.ValidateSession(ctx, result.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected old token invalidated, got %v", err)
	}
	sessionCtx, err := service.ValidateSession(ctx, switched.Token)
	if err != nil {
		t.Fatalf("validate new token: %v", err)
	}
	if sessionCtx.EffectiveTenant.ID != tenantB {
		t.Fatalf("expected tenant b, got %d", sessionCtx.EffectiveTenant.ID)
	}
	// 非成员组织选择拒绝
	if _, err := service.SelectTenant(ctx, switched.Token, tenantA+100); err == nil {
		t.Fatal("expected non-member tenant selection rejection")
	}
}

func TestSessionValidationRevokesOnMembershipChange(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	tenantID, adminID := seedTenantWithAdmin(t, service, "acme")
	admin := &Actor{ID: adminID, Username: "admin-acme"}
	user, err := service.CreateUser(ctx, "revoked-user", "test-password-005")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := service.AddMembership(ctx, &testActor, tenantID, user.ID, RoleMember); err != nil {
		t.Fatalf("add membership: %v", err)
	}
	result, err := service.Login(ctx, "revoked-user", "test-password-005", "10.0.0.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	// 绕过服务层直删成员关系，专测「会话校验回查 memberships」防线；
	// 服务层 RemoveMember 的删行吊销已在 TestMembershipLifecycle 覆盖。
	membership, err := service.repository.GetMembership(ctx, user.ID, tenantID)
	if err != nil {
		t.Fatalf("load membership: %v", err)
	}
	if err := service.repository.DeleteMembership(ctx, membership); err != nil {
		t.Fatalf("delete membership: %v", err)
	}
	if _, err := service.ValidateSession(ctx, result.Token); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("expected ErrSessionRevoked after removal, got %v", err)
	}
	if _, err := service.RemoveMember(ctx, admin, tenantID, user.ID); !errors.Is(err, ErrMembershipNotFound) {
		t.Fatalf("expected ErrMembershipNotFound on repeated removal, got %v", err)
	}
}

func TestSessionValidationRevokesOnRoleDemotion(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	tenantID, _ := seedTenantWithAdmin(t, service, "acme")
	user, err := service.CreateUser(ctx, "demoted-user", "test-password-006")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := service.AddMembership(ctx, &testActor, tenantID, user.ID, RoleTenantAdmin); err != nil {
		t.Fatalf("add membership: %v", err)
	}
	result, err := service.Login(ctx, "demoted-user", "test-password-006", "10.0.0.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	// 角色降级（直接改成员关系模拟）→ 会话即时失效
	membership, err := service.repository.GetMembership(ctx, user.ID, tenantID)
	if err != nil {
		t.Fatalf("load membership: %v", err)
	}
	membership.Role = RoleMember
	if err := service.repository.SaveMembership(ctx, membership); err != nil {
		t.Fatalf("demote: %v", err)
	}
	if _, err := service.ValidateSession(ctx, result.Token); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("expected ErrSessionRevoked after demotion, got %v", err)
	}
}

func TestSessionValidationRejectsDisabledTenant(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	tenantID, _ := seedTenantWithAdmin(t, service, "acme")
	user, err := service.CreateUser(ctx, "blocked-user", "test-password-007")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := service.AddMembership(ctx, &testActor, tenantID, user.ID, RoleMember); err != nil {
		t.Fatalf("add membership: %v", err)
	}
	result, err := service.Login(ctx, "blocked-user", "test-password-007", "10.0.0.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	// 直改组织状态（绕过 DisableTenant 的删行吊销），专测校验期 fail-closed：
	// 租户禁用语义要求禁用组织的作用域会话拒绝。
	tenant, err := service.repository.GetTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("load tenant: %v", err)
	}
	tenant.Status = TenantStatusDisabled
	if err := service.repository.SaveTenant(ctx, tenant); err != nil {
		t.Fatalf("disable tenant row: %v", err)
	}
	if _, err := service.ValidateSession(ctx, result.Token); !errors.Is(err, ErrTenantDisabled) {
		t.Fatalf("expected ErrTenantDisabled, got %v", err)
	}
	// 服务层 DisableTenant（含删行吊销）语义
	if _, err := service.DisableTenant(ctx, &testActor, tenantID); !errors.Is(err, ErrTenantAlreadyDisabled) {
		t.Fatalf("expected ErrTenantAlreadyDisabled, got %v", err)
	}
	tenant.Status = TenantStatusActive
	if err := service.repository.SaveTenant(ctx, tenant); err != nil {
		t.Fatalf("restore tenant row: %v", err)
	}
	if _, err := service.DisableTenant(ctx, &testActor, tenantID); err != nil {
		t.Fatalf("disable via service: %v", err)
	}
	if _, err := service.ValidateSession(ctx, result.Token); err == nil {
		t.Fatal("expected session gone after service-level disable")
	}
}

func TestChangePasswordFlow(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	tenantID, _ := seedTenantWithAdmin(t, service, "acme")
	user, err := service.CreateUser(ctx, "changer", "test-password-008")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := service.AddMembership(ctx, &testActor, tenantID, user.ID, RoleMember); err != nil {
		t.Fatalf("add membership: %v", err)
	}
	if _, err := service.CreateUser(ctx, "changer", "test-password-009"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("expected ErrUsernameTaken, got %v", err)
	}
	if _, err := service.CreateUser(ctx, "weak", "short"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected weak password rejection, got %v", err)
	}
	if err := service.ChangePassword(ctx, user.ID, "wrong-current", "test-password-010"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
	if err := service.ChangePassword(ctx, user.ID, "test-password-008", "test-password-010"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	if _, err := service.Login(ctx, "changer", "test-password-008", "10.0.0.1"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password should fail, got %v", err)
	}
	if _, err := service.Login(ctx, "changer", "test-password-010", "10.0.0.1"); err != nil {
		t.Fatalf("new password should pass, got %v", err)
	}
}

func TestSuperAdminLoginScope(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	superID, err := service.CreateSuperAdminUser(ctx, "superroot", "test-password-011")
	if err != nil {
		t.Fatalf("create super admin: %v", err)
	}
	result, err := service.Login(ctx, "superroot", "test-password-011", "10.0.0.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result.Scope.Kind != ScopeSuperAdmin {
		t.Fatalf("expected super admin scope, got %q", result.Scope.Kind)
	}
	if result.Scope.Tenant != nil {
		t.Fatal("super admin session must not carry active tenant")
	}
	sessionCtx, err := service.ValidateSession(ctx, result.Token)
	if err != nil {
		t.Fatalf("validate session: %v", err)
	}
	if sessionCtx.EffectiveTenant != nil || !sessionCtx.SuperAdminMode {
		t.Fatalf("unexpected super admin context %+v", sessionCtx)
	}
	// 超管账号不得持有 memberships
	tenantID, _ := seedTenantWithAdmin(t, service, "acme")
	if _, err := service.AddMembership(ctx, &testActor, tenantID, superID, RoleMember); err == nil {
		t.Fatal("expected membership rejection for super admin account")
	}
}

func TestLogoutDeletesSession(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	tenantID, _ := seedTenantWithAdmin(t, service, "acme")
	user, err := service.CreateUser(ctx, "logout-user", "test-password-012")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := service.AddMembership(ctx, &testActor, tenantID, user.ID, RoleMember); err != nil {
		t.Fatalf("add membership: %v", err)
	}
	result, err := service.Login(ctx, "logout-user", "test-password-012", "10.0.0.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := service.Logout(ctx, result.Token); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := service.ValidateSession(ctx, result.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected session gone after logout, got %v", err)
	}
}
