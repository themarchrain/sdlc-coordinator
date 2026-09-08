package controlplane

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRegisterWithInviteAutoLogin(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	tenantID, adminID := seedTenantWithAdmin(t, service, "acme")
	admin := &Actor{ID: adminID, Username: "admin-acme"}
	code, err := service.IssueInviteCode(ctx, admin, tenantID, InviteCodeInput{ExpiresInHours: 24, MaxUses: 2})
	if err != nil {
		t.Fatalf("issue invite: %v", err)
	}
	result, err := service.RegisterWithInviteCode(ctx, "Newbie ", "test-password-002", code.Plaintext)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// 注册即自动登录，会话绑定该组织
	if result.Scope.Kind != ScopeTenant || result.Scope.Tenant == nil || result.Scope.Tenant.ID != tenantID {
		t.Fatalf("expected auto-login bound to tenant, got %+v", result.Scope)
	}
	if result.Scope.Role != RoleMember {
		t.Fatalf("expected member role, got %q", result.Scope.Role)
	}
	if _, err := service.ValidateSession(ctx, result.Token); err != nil {
		t.Fatalf("validate session after register: %v", err)
	}
	// used_count 递增
	reloaded, err := service.repository.GetInviteCode(ctx, code.Record.ID)
	if err != nil {
		t.Fatalf("reload invite: %v", err)
	}
	if reloaded.UsedCount != 1 {
		t.Fatalf("expected used_count 1, got %d", reloaded.UsedCount)
	}
}

func TestRegisterRejectsBadInvites(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	tenantID, adminID := seedTenantWithAdmin(t, service, "acme")
	admin := &Actor{ID: adminID, Username: "admin-acme"}

	expired, err := service.IssueInviteCode(ctx, admin, tenantID, InviteCodeInput{ExpiresInHours: 24, MaxUses: 5})
	if err != nil {
		t.Fatalf("issue invite: %v", err)
	}
	// 直改库拨快过期时间（签发层已拒绝负值，模拟时间流逝走库）。
	expiredRow, err := service.repository.GetInviteCode(ctx, expired.Record.ID)
	if err != nil {
		t.Fatalf("load invite: %v", err)
	}
	expiredRow.ExpiresAt = time.Now().UTC().Add(-time.Hour)
	if err := service.repository.SaveInviteCode(ctx, expiredRow); err != nil {
		t.Fatalf("expire invite: %v", err)
	}
	if _, err := service.RegisterWithInviteCode(ctx, "late-user", "test-password-003", expired.Plaintext); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("expected ErrInviteInvalid for expired code, got %v", err)
	}

	limited, err := service.IssueInviteCode(ctx, admin, tenantID, InviteCodeInput{ExpiresInHours: 24, MaxUses: 1})
	if err != nil {
		t.Fatalf("issue limited invite: %v", err)
	}
	if _, err := service.RegisterWithInviteCode(ctx, "first-user", "test-password-004", limited.Plaintext); err != nil {
		t.Fatalf("first redemption: %v", err)
	}
	if _, err := service.RegisterWithInviteCode(ctx, "second-user", "test-password-005", limited.Plaintext); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("expected ErrInviteInvalid for exhausted code, got %v", err)
	}

	revocable, err := service.IssueInviteCode(ctx, admin, tenantID, InviteCodeInput{ExpiresInHours: 24, MaxUses: 5})
	if err != nil {
		t.Fatalf("issue revocable invite: %v", err)
	}
	if err := service.RevokeInviteCode(ctx, admin, tenantID, revocable.Record.ID); err != nil {
		t.Fatalf("revoke invite: %v", err)
	}
	if _, err := service.RegisterWithInviteCode(ctx, "revoked-user", "test-password-006", revocable.Plaintext); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("expected ErrInviteInvalid for revoked code, got %v", err)
	}

	if _, err := service.RegisterWithInviteCode(ctx, "junk-user", "test-password-007", "AAAA-AAAA-AAAA"); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("expected ErrInviteInvalid for unknown code, got %v", err)
	}
	// 大小写与连字符混排的同一枚码应可解析（规范化）
	flexible, err := service.IssueInviteCode(ctx, admin, tenantID, InviteCodeInput{ExpiresInHours: 24, MaxUses: 1})
	if err != nil {
		t.Fatalf("issue flexible invite: %v", err)
	}
	mangled := make([]rune, 0, len(flexible.Plaintext))
	for i, ch := range flexible.Plaintext {
		switch {
		case ch == '-':
			mangled = append(mangled, '_')
		case i%2 == 0 && ch >= 'A' && ch <= 'Z':
			mangled = append(mangled, ch-'A'+'a')
		default:
			mangled = append(mangled, ch)
		}
	}
	if _, err := service.RegisterWithInviteCode(ctx, "flex-user", "test-password-013", string(mangled)); err != nil {
		t.Fatalf("normalized code should redeem: %v", err)
	}
}

func TestRegisterRejectsDuplicateUsernameAndDisabledTenant(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	tenantID, adminID := seedTenantWithAdmin(t, service, "acme")
	admin := &Actor{ID: adminID, Username: "admin-acme"}
	code, err := service.IssueInviteCode(ctx, admin, tenantID, InviteCodeInput{ExpiresInHours: 24, MaxUses: 5})
	if err != nil {
		t.Fatalf("issue invite: %v", err)
	}
	if _, err := service.RegisterWithInviteCode(ctx, "dup-user", "test-password-008", code.Plaintext); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if _, err := service.RegisterWithInviteCode(ctx, "dup-user", "test-password-009", code.Plaintext); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("expected ErrUsernameTaken, got %v", err)
	}
	if _, err := service.DisableTenant(ctx, &testActor, tenantID); err != nil {
		t.Fatalf("disable tenant: %v", err)
	}
	if _, err := service.RegisterWithInviteCode(ctx, "blocked-user", "test-password-010", code.Plaintext); !errors.Is(err, ErrTenantDisabled) {
		t.Fatalf("expected ErrTenantDisabled, got %v", err)
	}
}
