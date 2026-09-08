package controlplane

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestCreateTenantRollsBackWhenAuditInsertFails(t *testing.T) {
	service := newTestService(t)
	callbackName := "fail_audit_" + strings.ReplaceAll(t.Name(), "/", "_")
	if err := service.repository.db.Callback().Create().Before("gorm:create").Register(callbackName, func(db *gorm.DB) {
		if db.Statement != nil && db.Statement.Table == "audit_log" {
			db.AddError(errors.New("audit unavailable"))
		}
	}); err != nil {
		t.Fatalf("register audit failure callback: %v", err)
	}
	t.Cleanup(func() {
		_ = service.repository.db.Callback().Create().Remove(callbackName)
	})

	_, err := service.CreateTenant(context.Background(), &testActor, TenantInput{Slug: "acme", DisplayName: "Acme 组织"})
	if err == nil {
		t.Fatal("expected create tenant to fail when audit insert fails")
	}

	var tenants int64
	if err := service.repository.db.Model(&Tenant{}).Count(&tenants).Error; err != nil {
		t.Fatalf("count tenants: %v", err)
	}
	if tenants != 0 {
		t.Fatalf("tenant row leaked after audit failure, count=%d", tenants)
	}
}

func TestCreateTenantRecordsAuditSourceIP(t *testing.T) {
	service := newTestService(t)
	ctx := WithAuditSourceIP(context.Background(), "203.0.113.9")
	tenant, err := service.CreateTenant(ctx, &testActor, TenantInput{Slug: "acme", DisplayName: "Acme 组织"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	logs := service.repository.listAuditLogsByAction(context.Background(), "tenant.created")
	if len(logs) != 1 {
		t.Fatalf("expected 1 tenant.created audit, got %d", len(logs))
	}
	if logs[0].SourceIP != "203.0.113.9" {
		t.Fatalf("audit source_ip = %q, want 203.0.113.9 (tenant_id=%d)", logs[0].SourceIP, tenant.ID)
	}
}

func TestEnterSupportAccessRejectsDisabledTenant(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	tenantID, _ := seedTenantWithAdmin(t, service, "acme")
	superID, err := service.CreateSuperAdminUser(ctx, "root", "test-super-password")
	if err != nil {
		t.Fatalf("seed super admin: %v", err)
	}
	superLogin, err := service.Login(ctx, "root", "test-super-password", "10.0.0.2")
	if err != nil {
		t.Fatalf("super login: %v", err)
	}
	if _, err := service.DisableTenant(ctx, &Actor{ID: superID, Username: "root"}, tenantID); err != nil {
		t.Fatalf("disable tenant: %v", err)
	}
	if err := service.EnterSupportAccess(ctx, superLogin.Token, tenantID, "停用后排障"); !errors.Is(err, ErrTenantDisabled) {
		t.Fatalf("expected ErrTenantDisabled entering support on disabled tenant, got %v", err)
	}
}

func TestPurgeExpiredSessionsDeletesOnlyExpiredRows(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	repository := openTestControlPlane(t)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	service := NewService(repository, WithClock(func() time.Time { return now }))
	ctx := context.Background()
	if _, err := service.CreateSuperAdminUser(ctx, "root", "test-super-password"); err != nil {
		t.Fatalf("seed super admin: %v", err)
	}
	live, err := service.Login(ctx, "root", "test-super-password", "10.0.0.2")
	if err != nil {
		t.Fatalf("live login: %v", err)
	}
	stale, err := service.Login(ctx, "root", "test-super-password", "10.0.0.2")
	if err != nil {
		t.Fatalf("stale login: %v", err)
	}

	staleSession, err := repository.GetActiveSessionByHash(ctx, sessionTokenHash(stale.Token), now.Add(time.Second))
	if err != nil {
		t.Fatalf("load stale session: %v", err)
	}
	staleSession.ExpiresAt = now.Add(-time.Minute)
	if err := repository.SaveSession(ctx, staleSession); err != nil {
		t.Fatalf("expire stale session: %v", err)
	}

	deleted, err := service.PurgeExpiredSessions(ctx)
	if err != nil {
		t.Fatalf("purge expired sessions: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted=%d, want 1", deleted)
	}
	if _, err := repository.GetActiveSessionByHash(ctx, sessionTokenHash(live.Token), now); err != nil {
		t.Fatalf("live session must survive purge: %v", err)
	}
	var remaining int64
	if err := repository.db.Model(&Session{}).Count(&remaining).Error; err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("expected only the live session to remain, got %d", remaining)
	}
}
