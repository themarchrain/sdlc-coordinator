package controlplane

import (
	"context"
	"errors"
	"testing"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	repository := openTestControlPlane(t)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate control plane: %v", err)
	}
	return NewService(repository)
}

var testActor = Actor{ID: 1, Username: "root"}

func TestCreateTenantPersistsAndAudits(t *testing.T) {
	service := newTestService(t)
	tenant, err := service.CreateTenant(context.Background(), &testActor, TenantInput{Slug: "acme", DisplayName: "Acme 组织"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if tenant.ID == 0 {
		t.Fatal("expected persisted tenant id")
	}
	if tenant.Status != TenantStatusActive {
		t.Fatalf("expected active status, got %q", tenant.Status)
	}
	if tenant.DMLocator != "sqlite://data/tenants/acme/db.sqlite" {
		t.Fatalf("unexpected db locator %q", tenant.DMLocator)
	}
	logs := service.repository.listAuditLogsByAction(context.Background(), "tenant.created")
	if len(logs) != 1 {
		t.Fatalf("expected 1 tenant.created audit log, got %d", len(logs))
	}
	if logs[0].ActorUsername != "root" || logs[0].TenantID == nil || *logs[0].TenantID != tenant.ID {
		t.Fatalf("unexpected audit log %+v", logs[0])
	}
}

func TestCreateTenantRejectsInvalidSlug(t *testing.T) {
	service := newTestService(t)
	longSlug := make([]rune, 33)
	for i := range longSlug {
		longSlug[i] = 'a'
	}
	for _, slug := range []string{"", "ab", "-abc", "abc-", "a b", string(longSlug)} {
		if _, err := service.CreateTenant(context.Background(), &testActor, TenantInput{Slug: slug, DisplayName: "名称"}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("slug %q: expected ErrInvalidInput, got %v", slug, err)
		}
	}
	if _, err := service.CreateTenant(context.Background(), &testActor, TenantInput{Slug: "acme", DisplayName: "  "}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty display name: expected ErrInvalidInput, got %v", err)
	}
}

func TestCreateTenantNormalizesSlugAndName(t *testing.T) {
	service := newTestService(t)
	tenant, err := service.CreateTenant(context.Background(), &testActor, TenantInput{Slug: "Acme", DisplayName: " Acme 组织 "})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if tenant.Slug != "acme" {
		t.Fatalf("expected normalized slug acme, got %q", tenant.Slug)
	}
	if tenant.DisplayName != "Acme 组织" {
		t.Fatalf("expected trimmed display name, got %q", tenant.DisplayName)
	}
}

func TestCreateTenantRejectsDuplicateSlug(t *testing.T) {
	service := newTestService(t)
	if _, err := service.CreateTenant(context.Background(), &testActor, TenantInput{Slug: "acme", DisplayName: "第一"}); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if _, err := service.CreateTenant(context.Background(), &testActor, TenantInput{Slug: "acme", DisplayName: "第二"}); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("expected ErrSlugTaken, got %v", err)
	}
}

func TestDisableAndRestoreTenantAudits(t *testing.T) {
	service := newTestService(t)
	tenant, err := service.CreateTenant(context.Background(), &testActor, TenantInput{Slug: "acme", DisplayName: "Acme"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	disabled, err := service.DisableTenant(context.Background(), &testActor, tenant.ID)
	if err != nil {
		t.Fatalf("disable tenant: %v", err)
	}
	if disabled.Status != TenantStatusDisabled {
		t.Fatalf("expected disabled, got %q", disabled.Status)
	}
	if _, err := service.DisableTenant(context.Background(), &testActor, tenant.ID); !errors.Is(err, ErrTenantAlreadyDisabled) {
		t.Fatalf("expected ErrTenantAlreadyDisabled, got %v", err)
	}
	restored, err := service.RestoreTenant(context.Background(), &testActor, tenant.ID)
	if err != nil {
		t.Fatalf("restore tenant: %v", err)
	}
	if restored.Status != TenantStatusActive {
		t.Fatalf("expected active, got %q", restored.Status)
	}
	if _, err := service.RestoreTenant(context.Background(), &testActor, tenant.ID); !errors.Is(err, ErrTenantNotDisabled) {
		t.Fatalf("expected ErrTenantNotDisabled, got %v", err)
	}
	disableLogs := service.repository.listAuditLogsByAction(context.Background(), "tenant.disabled")
	restoreLogs := service.repository.listAuditLogsByAction(context.Background(), "tenant.restored")
	if len(disableLogs) != 1 || len(restoreLogs) != 1 {
		t.Fatalf("expected exactly one disable/restore audit log, got %d/%d", len(disableLogs), len(restoreLogs))
	}
}

func TestUpdateTenantDisplayNameAudits(t *testing.T) {
	service := newTestService(t)
	tenant, err := service.CreateTenant(context.Background(), &testActor, TenantInput{Slug: "acme", DisplayName: "旧名"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	updated, err := service.UpdateTenant(context.Background(), &testActor, tenant.ID, "新名字")
	if err != nil {
		t.Fatalf("update tenant: %v", err)
	}
	if updated.DisplayName != "新名字" {
		t.Fatalf("expected updated display name, got %q", updated.DisplayName)
	}
	logs := service.repository.listAuditLogsByAction(context.Background(), "tenant.updated")
	if len(logs) != 1 {
		t.Fatalf("expected 1 tenant.updated audit log, got %d", len(logs))
	}
	if _, err := service.UpdateTenant(context.Background(), &testActor, 9999, "不存在"); !errors.Is(err, ErrTenantNotFound) {
		t.Fatalf("expected ErrTenantNotFound, got %v", err)
	}
}
