package app

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/store"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// 装配：ready 装配换真分库句柄工厂——启动遍历
// 控制面租户逐库迁移（失败中止并列出失败租户）；控制面已有的租户在首次
// 装配时由开通钩子补建数据面并回写 db_locator。
func TestNewReadyMigratesTenantStoresFromControlPlane(t *testing.T) {
	settings := testControlPlaneSettings(t)
	tenantID := seedControlPlaneTenant(t, settings)

	tenantDataDir := t.TempDir()
	cfg := withTestControlPlane(t, config.Config{HTTPAddr: ":0", TenantDataDir: tenantDataDir})
	cfg.ControlPlane = settings
	db := openAppMainDB(t)

	application, err := New(cfg, zap.NewNop(), db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() {
		if closer, ok := application.TenantStores.(interface{ Close() }); ok {
			closer.Close()
		}
	})

	tenantFile := filepath.Join(tenantDataDir, strconv.FormatUint(uint64(tenantID), 10), "db.sqlite")
	if _, err := os.Stat(tenantFile); err != nil {
		t.Fatalf("tenant sqlite file must exist after startup migration: %v", err)
	}
	tenantDB, err := store.Open("file:" + filepath.ToSlash(tenantFile))
	if err != nil {
		t.Fatalf("open tenant database: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := tenantDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if !tenantDB.Migrator().HasTable("tasks") {
		t.Fatalf("tenant store must carry migrated business tables")
	}
	// db_locator 回写只在 API 建租户路径（开通钩子，控制面测试锁定）；
	// 直接种行的存量租户由启动迁移补齐数据面，不回写定位串。
}

func openControlPlaneRepository(t *testing.T, settings config.ControlPlaneSettings) *controlplane.Repository {
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
	return repository
}

func seedControlPlaneTenant(t *testing.T, settings config.ControlPlaneSettings) uint {
	t.Helper()
	repository := openControlPlaneRepository(t, settings)
	tenant := &controlplane.Tenant{Slug: "acme-it", DisplayName: "Acme IT"}
	if err := repository.CreateTenant(context.Background(), tenant); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	return tenant.ID
}

func openAppMainDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := store.Open("file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "main.db")) + "?_busy_timeout=1000")
	if err != nil {
		t.Fatalf("open main database: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
