package configuration

import (
	"context"
	"testing"
)

// TENANT_DATA_DIR 是租户数据根（基础设施配置，与 CONTROL_PLANE_*
// 同级，不进快照 schema）。ready 产物必须从 legacy（env）回填，与既有修复
// （ready 模式丢 ControlPlane）同型防回归。
func TestResolverReadyCarriesLegacyTenantDataDir(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	if _, err := repository.Save(context.Background(), StatusReady, SourceUI, validRuntimeConfig()); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	legacy := completeLegacyConfig()
	legacy.TenantDataDir = "/srv/exp/tenants"

	result := NewResolver(repository).Resolve(context.Background(), testBootstrapConfig(), legacy)

	if result.Mode != ModeReady {
		t.Fatalf("unexpected resolution: %#v", result)
	}
	if result.EffectiveConfig.TenantDataDir != legacy.TenantDataDir {
		t.Fatalf("EffectiveConfig.TenantDataDir must backfill from legacy env: got %q want %q",
			result.EffectiveConfig.TenantDataDir, legacy.TenantDataDir)
	}
}
