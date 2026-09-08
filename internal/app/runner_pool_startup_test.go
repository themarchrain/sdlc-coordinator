package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/store"

	"go.uber.org/zap"
)

// 启动校验（fail-fast）：控制面 runner 指派存在非法形态（同端点跨
// 租户）时，New/NewReady 必须拒绝启动，与 ErrControlPlaneNotConfigured 同风格。
func TestNewReadyRejectsStartupOnInvalidRunnerAssignments(t *testing.T) {
	settings := testControlPlaneSettings(t)
	seedInvalidRunnerAssignments(t, settings)

	db, err := store.Open("file:" + filepath.Join(t.TempDir(), "main.db") + "?_busy_timeout=1000")
	if err != nil {
		t.Fatalf("open main database: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	cfg := config.Config{HTTPAddr: ":0", ControlPlane: settings, TenantDataDir: t.TempDir()}

	_, err = New(cfg, zap.NewNop(), db)
	if err == nil {
		t.Fatalf("expected startup rejection on invalid runner assignments")
	}
	if !strings.Contains(err.Error(), "runner") {
		t.Fatalf("rejection must name the runner assignment problem, got %v", err)
	}
}

func seedInvalidRunnerAssignments(t *testing.T, settings config.ControlPlaneSettings) {
	t.Helper()
	db, err := store.Open(settings.DSN)
	if err != nil {
		t.Fatalf("open control plane database: %v", err)
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
	ctx := context.Background()
	for _, tenant := range []*controlplane.Tenant{
		{ID: 1, Slug: "acme", DisplayName: "Acme"},
		{ID: 2, Slug: "globex", DisplayName: "Globex"},
	} {
		if err := repository.CreateTenant(ctx, tenant); err != nil {
			t.Fatalf("create tenant %s: %v", tenant.Slug, err)
		}
	}
	// 同一端点指派给两个租户：卷防线二前提被破坏，必须拒绝启动。
	for _, tenantID := range []uint{1, 2} {
		if err := repository.ReplaceRunnerAssignments(ctx, tenantID, []string{"http://shared-runner:8081"}); err != nil {
			t.Fatalf("seed assignments for tenant %d: %v", tenantID, err)
		}
	}
}
