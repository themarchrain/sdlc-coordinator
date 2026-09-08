package controlplane

import (
	"context"
	"errors"
	"testing"
)

// ===== 登录「唯一可用组织」必须绑过滤后的活跃组织，而非 memberships[0] =====

// TestLoginSingleActiveTenantBindsActiveOne 锁定「一活一停」登录路径：先加入的
// 组织（membership id 更小、排序在前）被停用后，tenantOptionsFor 只剩一个活跃
// 组织，此时必须直绑该活跃组织。历史缺陷用 memberships[0]（指向已停用组织）
// 发会话，ValidateSession 回查组织状态必 403，且 scope.kind=tenant 使前端不进
// 选择组织页，登出重登同样卡死。
func TestLoginSingleActiveTenantBindsActiveOne(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	tenantA, _ := seedTenantWithAdmin(t, service, "alpha")
	tenantB, _ := seedTenantWithAdmin(t, service, "beta")
	if tenantA >= tenantB {
		t.Fatalf("test precondition: alpha must hold the smaller tenant id, got %d >= %d", tenantA, tenantB)
	}
	user, err := service.CreateUser(ctx, "one-live-user", "test-password-007")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	// 先加 alpha（membership id 更小、ListMembershipsByUser 排在前），后加 beta
	if _, err := service.AddMembership(ctx, &testActor, tenantA, user.ID, RoleMember); err != nil {
		t.Fatalf("add membership a: %v", err)
	}
	if _, err := service.AddMembership(ctx, &testActor, tenantB, user.ID, RoleTenantAdmin); err != nil {
		t.Fatalf("add membership b: %v", err)
	}
	if _, err := service.DisableTenant(ctx, &testActor, tenantA); err != nil {
		t.Fatalf("disable tenant a: %v", err)
	}
	result, err := service.Login(ctx, "one-live-user", "test-password-007", "10.0.0.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result.Scope.Kind != ScopeTenant {
		t.Fatalf("expected tenant scope, got %q", result.Scope.Kind)
	}
	if result.Scope.Tenant == nil || result.Scope.Tenant.ID != tenantB {
		t.Fatalf("expected direct bind to the sole active tenant %d, got %+v", tenantB, result.Scope.Tenant)
	}
	if result.Scope.Role != RoleTenantAdmin {
		t.Fatalf("expected role from the active membership, got %q", result.Scope.Role)
	}
	sessionCtx, err := service.ValidateSession(ctx, result.Token)
	if err != nil {
		t.Fatalf("session bound to active tenant must validate: %v", err)
	}
	if sessionCtx.EffectiveTenant == nil || sessionCtx.EffectiveTenant.ID != tenantB {
		t.Fatalf("expected effective tenant %d, got %+v", tenantB, sessionCtx.EffectiveTenant)
	}
}

// ===== 种子通道对非超管同名账号 fail-fast；RESET 仅作用于已有超管 =====

// TestSeedSuperAdminRejectsNonSuperAdminSameName 锁定：env 声称的超管用户名已被
// 普通账号占用时，无论 RESET 与否都必须报错（fail-fast 拒绝启动），不得静默
// no-op，更不得改写普通账号密码、吊销其会话或写出虚假的 password_reset 审计。
func TestSeedSuperAdminRejectsNonSuperAdminSameName(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	tenantID, _ := seedTenantWithAdmin(t, service, "acme")
	user, err := service.CreateUser(ctx, "plainadmin", "original-pass-001")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := service.AddMembership(ctx, &testActor, tenantID, user.ID, RoleMember); err != nil {
		t.Fatalf("add membership: %v", err)
	}
	login, err := service.Login(ctx, "plainadmin", "original-pass-001", "10.0.0.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	// 同名普通账号在场：未开 RESET 不得静默 no-op，开了 RESET 不得改写
	for _, reset := range []bool{false, true} {
		err := service.SeedSuperAdmin(ctx, SeedSuperAdminInput{Username: "plainadmin", Password: "hijacked-pass-01", Reset: reset})
		if !errors.Is(err, ErrSeedUsernameConflict) {
			t.Fatalf("reset=%v: expected ErrSeedUsernameConflict, got %v", reset, err)
		}
	}
	// 原密码仍可登录：密码未被改写、既有会话未被吊销
	if _, err := service.Login(ctx, "plainadmin", "original-pass-001", "10.0.0.2"); err != nil {
		t.Fatalf("original password must keep working: %v", err)
	}
	if _, err := service.ValidateSession(ctx, login.Token); err != nil {
		t.Fatalf("existing session must survive failed seed: %v", err)
	}
	if logs := service.repository.listAuditLogsByAction(ctx, "superadmin.password_reset"); len(logs) != 0 {
		t.Fatalf("expected no password_reset audit for non-super-admin, got %d", len(logs))
	}
	// 对照：同用户名真超管在场时，未开 RESET 仍为 no-op，开 RESET 仍走重置
	if err := service.SeedSuperAdmin(ctx, SeedSuperAdminInput{Username: "root", Password: "seed-root-pass-01"}); err != nil {
		t.Fatalf("seed root: %v", err)
	}
	if err := service.SeedSuperAdmin(ctx, SeedSuperAdminInput{Username: "root", Password: "seed-root-pass-01"}); err != nil {
		t.Fatalf("idempotent seed on real super admin: %v", err)
	}
}
