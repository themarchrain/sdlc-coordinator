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

// 启动校验（fail-fast）：控制面 runner 指派存在非法形态（孤儿租户引用）
// 时，New/NewReady 必须拒绝启动，与 ErrControlPlaneNotConfigured 同风格。
// 同端点跨租户已由控制面唯一索引在写入层拒绝（不可构造），守卫保留该类
// 校验仅作纵深防御，此处以可直接落库的孤儿租户指派钉住守卫路径。
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
	// 指派引用不存在的租户（孤儿行）：启动守卫必须拒绝启动。
	if err := repository.ReplaceRunnerAssignments(ctx, 999, []string{"http://shared-runner:8081"}); err != nil {
		t.Fatalf("seed orphan assignment: %v", err)
	}
}
