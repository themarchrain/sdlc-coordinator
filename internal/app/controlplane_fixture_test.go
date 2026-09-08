package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sdlc-coordinator/internal/config"
)

func testControlPlaneSettings(t *testing.T) config.ControlPlaneSettings {
	t.Helper()
	dir, err := os.MkdirTemp("", "cp-test")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return config.ControlPlaneSettings{
		DSN:                "file:" + filepath.Join(dir, "control-plane.db") + "?_busy_timeout=1000",
		SuperAdminUsername: "root",
		SuperAdminPassword: "test-super-password",
	}
}

func withTestControlPlane(t *testing.T, cfg config.Config) config.Config {
	t.Helper()
	if strings.TrimSpace(cfg.ControlPlane.DSN) == "" {
		cfg.ControlPlane.DSN = testControlPlaneSettings(t).DSN
	}
	if cfg.ControlPlane.SuperAdminUsername == "" {
		cfg.ControlPlane.SuperAdminUsername = "root"
	}
	if cfg.ControlPlane.SuperAdminPassword == "" {
		cfg.ControlPlane.SuperAdminPassword = "test-super-password"
	}
	// 租户数据根生产默认指向进程相对路径 data/tenants，测试必须改指临时
	// 目录，避免 Handle 在源码树里落 data/tenants 目录。
	if strings.TrimSpace(cfg.TenantDataDir) == "" {
		cfg.TenantDataDir = t.TempDir()
	}
	return cfg
}

func withTestBootstrap(t *testing.T, cfg config.BootstrapConfig) config.BootstrapConfig {
	t.Helper()
	filled := withTestControlPlane(t, config.Config{ControlPlane: cfg.ControlPlane})
	cfg.ControlPlane = filled.ControlPlane
	return cfg
}
