package controlplane

import (
	"context"
	"errors"
	"testing"
	"time"
)

// seedTenantWithAdmin 建组织 + 管理员用户 + tenant_admin 成员关系，返回二元 id。
func seedTenantWithAdmin(t *testing.T, service *Service, slug string) (tenantID, adminID uint) {
	t.Helper()
	tenant, err := service.CreateTenant(context.Background(), &testActor, TenantInput{Slug: slug, DisplayName: slug})
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	user, err := service.CreateUser(context.Background(), "admin-"+slug, "test-password-001")
	if err != nil {
		t.Fatalf("seed admin user: %v", err)
	}
	if _, err := service.AddMembership(context.Background(), &testActor, tenant.ID, user.ID, RoleTenantAdmin); err != nil {
		t.Fatalf("seed admin membership: %v", err)
	}
	return tenant.ID, user.ID
}

func TestInviteCodeIssueAndStorage(t *testing.T) {
	service := newTestService(t)
	tenantID, adminID := seedTenantWithAdmin(t, service, "acme")
	admin := &Actor{ID: adminID, Username: "admin-acme"}

	code, err := service.IssueInviteCode(context.Background(), admin, tenantID, InviteCodeInput{ExpiresInHours: 24, MaxUses: 2})
	if err != nil {
		t.Fatalf("issue invite code: %v", err)
	}
	if len(code.Plaintext) != 14 || code.Plaintext[4] != '-' || code.Plaintext[9] != '-' {
		t.Fatalf("unexpected invite code format %q", code.Plaintext)
	}
	if code.Record.UsedCount != 0 || code.Record.Role != RoleMember {
		t.Fatalf("unexpected invite record %+v", code.Record)
	}
	// 哈希存储：库里查不到明文
	var stored InviteCode
	if err := service.repository.db.Where("tenant_id = ?", tenantID).First(&stored).Error; err != nil {
		t.Fatalf("load stored invite: %v", err)
	}
	if stored.CodeHash == code.Plaintext || stored.CodeHash == "" {
		t.Fatalf("invite code must be stored as hash, got %q", stored.CodeHash)
	}
}

func TestInviteCodeRevoke(t *testing.T) {
	service := newTestService(t)
	tenantID, adminID := seedTenantWithAdmin(t, service, "acme")
	admin := &Actor{ID: adminID, Username: "admin-acme"}
	code, err := service.IssueInviteCode(context.Background(), admin, tenantID, InviteCodeInput{ExpiresInHours: 24, MaxUses: 5})
	if err != nil {
		t.Fatalf("issue invite: %v", err)
	}
	if err := service.RevokeInviteCode(context.Background(), admin, tenantID, code.Record.ID); err != nil {
		t.Fatalf("revoke invite: %v", err)
	}
	revoked, err := service.repository.GetInviteCode(context.Background(), code.Record.ID)
	if err != nil {
		t.Fatalf("reload invite: %v", err)
	}
	if revoked.RevokedAt == nil {
		t.Fatal("expected revoked_at set after revoke")
	}
	// 重复吊销保持幂等成功
	if err := service.RevokeInviteCode(context.Background(), admin, tenantID, code.Record.ID); err != nil {
		t.Fatalf("idempotent revoke: %v", err)
	}
	// 跨组织吊销拒绝
	otherTenantID, _ := seedTenantWithAdmin(t, service, "beta")
	if err := service.RevokeInviteCode(context.Background(), admin, otherTenantID, code.Record.ID); !errors.Is(err, ErrTenantNotFound) {
		t.Fatalf("expected cross-tenant revoke rejection, got %v", err)
	}
}

func TestMembershipLifecycle(t *testing.T) {
	service := newTestService(t)
	tenantID, adminID := seedTenantWithAdmin(t, service, "acme")
	user, err := service.CreateUser(context.Background(), "plain-member", "test-password-011")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	membership, err := service.AddMembership(context.Background(), &testActor, tenantID, user.ID, RoleMember)
	if err != nil {
		t.Fatalf("add membership: %v", err)
	}
	if membership.Role != RoleMember {
		t.Fatalf("expected member, got %q", membership.Role)
	}
	// 同一组织重复入会拒绝
	if _, err := service.AddMembership(context.Background(), &testActor, tenantID, user.ID, RoleMember); err == nil {
		t.Fatal("expected duplicate membership rejection")
	}
	members, err := service.ListMembers(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
	// 移除成员即时吊销其该组织会话
	session := &Session{TokenHash: "hash-remove-me", UserID: user.ID, ActiveTenantID: &tenantID, Role: RoleMember,
		ExpiresAt: time.Now().Add(time.Hour)}
	if err := service.repository.CreateSession(context.Background(), session); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := service.RemoveMember(context.Background(), &Actor{ID: adminID, Username: "admin-acme"}, tenantID, user.ID); err != nil {
		t.Fatalf("remove member: %v", err)
	}
	if _, err := service.repository.GetMembership(context.Background(), user.ID, tenantID); !errors.Is(err, ErrMembershipNotFound) {
		t.Fatalf("expected membership gone after removal, got %v", err)
	}
	var remaining int64
	service.repository.db.Model(&Session{}).Where("user_id = ?", user.ID).Count(&remaining)
	if remaining != 0 {
		t.Fatalf("expected member sessions revoked on removal, got %d", remaining)
	}
	logs := service.repository.listAuditLogsByAction(context.Background(), "member.removed")
	if len(logs) != 1 {
		t.Fatalf("expected member.removed audit, got %d", len(logs))
	}
	// 不能移除自己
	if _, err := service.RemoveMember(context.Background(), &Actor{ID: adminID, Username: "admin-acme"}, tenantID, adminID); !errors.Is(err, ErrCannotRemoveSelf) {
		t.Fatalf("expected ErrCannotRemoveSelf, got %v", err)
	}
}
