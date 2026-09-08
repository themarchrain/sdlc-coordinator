package controlplane

import (
	"context"
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestSeedSuperAdminCreatesOnce(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	err := service.SeedSuperAdmin(ctx, SeedSuperAdminInput{Username: "Root", Password: "test-password-001"})
	if err != nil {
		t.Fatalf("seed super admin: %v", err)
	}
	user, err := service.repository.GetUserByUsername(ctx, "root")
	if err != nil {
		t.Fatalf("load seeded user: %v", err)
	}
	if user == nil || user.Role != UserRoleSuperAdmin {
		t.Fatalf("expected seeded super admin, got %+v", user)
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("test-password-001")) != nil {
		t.Fatal("expected bcrypt hash of seed password")
	}
	logs := service.repository.listAuditLogsByAction(ctx, "superadmin.seeded")
	if len(logs) != 1 {
		t.Fatalf("expected 1 superadmin.seeded audit, got %d", len(logs))
	}
	// 重复启动（未开 Reset）为 no-op：不报错、不重复建号、不写审计
	if err := service.SeedSuperAdmin(ctx, SeedSuperAdminInput{Username: "root", Password: "test-password-001"}); err != nil {
		t.Fatalf("idempotent seed: %v", err)
	}
	if logs := service.repository.listAuditLogsByAction(ctx, "superadmin.seeded"); len(logs) != 1 {
		t.Fatalf("expected still 1 seeded audit, got %d", len(logs))
	}
	// 非法入参拒绝
	if err := service.SeedSuperAdmin(ctx, SeedSuperAdminInput{Username: "x", Password: "test-password-002"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for short username, got %v", err)
	}
	if err := service.SeedSuperAdmin(ctx, SeedSuperAdminInput{Username: "validname", Password: "short"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for weak password, got %v", err)
	}
}

func TestSeedSuperAdminResetChannel(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	if err := service.SeedSuperAdmin(ctx, SeedSuperAdminInput{Username: "root", Password: "test-password-003"}); err != nil {
		t.Fatalf("seed super admin: %v", err)
	}
	result, err := service.Login(ctx, "root", "test-password-003", "10.0.0.1")
	if err != nil {
		t.Fatalf("login before reset: %v", err)
	}
	// 密码丢失：env 重置通道，重置写审计并吊销既有会话
	if err := service.SeedSuperAdmin(ctx, SeedSuperAdminInput{Username: "root", Password: "test-password-004", Reset: true}); err != nil {
		t.Fatalf("reset via seed: %v", err)
	}
	if _, err := service.ValidateSession(ctx, result.Token); err == nil {
		t.Fatal("expected sessions revoked after env reset")
	}
	if _, err := service.Login(ctx, "root", "test-password-003", "10.0.0.1"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password should fail, got %v", err)
	}
	if _, err := service.Login(ctx, "root", "test-password-004", "10.0.0.1"); err != nil {
		t.Fatalf("new password should pass, got %v", err)
	}
	logs := service.repository.listAuditLogsByAction(ctx, "superadmin.password_reset")
	if len(logs) != 1 {
		t.Fatalf("expected 1 superadmin.password_reset audit, got %d", len(logs))
	}
}

func TestSeedSuperAdminAcceptsBcryptHashForm(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password-005"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("precompute hash: %v", err)
	}
	hashed := string(hash)
	if !strings.HasPrefix(hashed, "$2") {
		t.Fatalf("expected bcrypt format, got %q", hashed)
	}
	if err := service.SeedSuperAdmin(ctx, SeedSuperAdminInput{Username: "root", Password: hashed}); err != nil {
		t.Fatalf("seed with hash: %v", err)
	}
	if _, err := service.Login(ctx, "root", "test-password-005", "10.0.0.1"); err != nil {
		t.Fatalf("login with preimage: %v", err)
	}
}
