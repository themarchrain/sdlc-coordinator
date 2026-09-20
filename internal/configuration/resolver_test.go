package configuration

import (
	"context"
	"strings"
	"testing"

	"encoding/json"
	"sdlc-coordinator/internal/config"
)

func TestResolverImportsCompleteLegacyEnvironmentOnlyWhenTableIsEmpty(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	resolver := NewResolver(repository)

	result := resolver.Resolve(context.Background(), testBootstrapConfig(), completeLegacyConfig())

	if result.Mode != ModeReady || result.Snapshot == nil {
		t.Fatalf("unexpected resolution: %#v", result)
	}
	if result.Snapshot.Source != SourceEnvImport || result.Snapshot.Revision != 1 || result.Snapshot.Status != StatusReady {
		t.Fatalf("legacy environment was not imported: %#v", result.Snapshot)
	}
	if result.EffectiveConfig.AgentRunner.ServiceURL != "http://agent-runner:8081" {
		t.Fatalf("unexpected effective config: %#v", result.EffectiveConfig)
	}
	if result.EffectiveConfig.AgentRunner.ProviderConfigRevision != result.Snapshot.Revision {
		t.Fatalf("agent runner provider revision was not pinned: %#v", result.EffectiveConfig.AgentRunner)
	}
}

func TestResolverIgnoresLegacyEnvironmentWhenSnapshotExists(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	persisted := validRuntimeConfig()
	persisted.AgentRunner.Model = "sqlite-model"
	if _, err := repository.Save(context.Background(), StatusReady, SourceUI, persisted); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	legacy := completeLegacyConfig()
	legacy.AgentRunner.Model = "conflicting-env-model"

	result := NewResolver(repository).Resolve(context.Background(), testBootstrapConfig(), legacy)

	if result.Mode != ModeReady || result.EffectiveConfig.AgentRunner.Model != persisted.AgentRunner.Model {
		t.Fatalf("legacy environment overrode SQLite: %#v", result)
	}
	if result.Snapshot == nil || result.Snapshot.Revision != 1 || result.Snapshot.Source != SourceUI {
		t.Fatalf("unexpected snapshot: %#v", result.Snapshot)
	}
}

func TestResolverReturnsSetupPendingWithPartialLegacyPrefill(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	legacy := config.Config{
		AgentRunner: config.AgentRunnerSettings{
			ServiceURL: "http://agent-runner:8081",
			Provider:   "claude-cli",
			Model:      "deepseek-v4-flash-0731",
		},
		Scheduler: completeLegacyConfig().Scheduler,
	}

	result := NewResolver(repository).Resolve(context.Background(), testBootstrapConfig(), legacy)

	if result.Mode != ModeSetupPending || result.Snapshot != nil {
		t.Fatalf("unexpected resolution: %#v", result)
	}
	if result.Prefill.AgentRunner.ServiceURL != legacy.AgentRunner.ServiceURL || result.Prefill.AgentRunner.Model != legacy.AgentRunner.Model {
		t.Fatalf("partial values were not preserved for prefill: %#v", result.Prefill)
	}
	if _, err := repository.Latest(context.Background()); err != ErrSnapshotNotFound {
		t.Fatalf("partial legacy config must not create snapshot, got %v", err)
	}
}

func TestResolverMovesSavedRestartRequiredSnapshotToReadyOnNewProcess(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	saved, err := repository.Save(context.Background(), StatusRestartRequired, SourceUI, validRuntimeConfig())
	if err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	result := NewResolver(repository).Resolve(context.Background(), testBootstrapConfig(), config.Config{})

	if result.Mode != ModeReady || result.Snapshot == nil || result.Snapshot.Revision != saved.Revision || result.Snapshot.Status != StatusReady || result.Snapshot.AppliedAt == nil {
		t.Fatalf("saved configuration was not marked applied: %#v", result)
	}
}

func TestResolverReturnsConfigurationErrorForInvalidStoredSnapshot(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	invalid := validRuntimeConfig()
	invalid.AgentRunner.Model = ""
	if _, err := repository.Save(context.Background(), StatusReady, SourceUI, invalid); err != nil {
		t.Fatalf("seed invalid snapshot: %v", err)
	}

	result := NewResolver(repository).Resolve(context.Background(), testBootstrapConfig(), completeLegacyConfig())

	if result.Mode != ModeConfigurationError || result.Snapshot == nil || !strings.Contains(result.ConfigurationError, "validation") {
		t.Fatalf("unexpected invalid snapshot resolution: %#v", result)
	}
	if result.EffectiveConfig.AgentRunner.Model != "" {
		t.Fatal("invalid stored snapshot must not assemble runtime config")
	}
}

func testBootstrapConfig() config.BootstrapConfig {
	return config.BootstrapConfig{
		HTTPAddr:    ":8080",
		DatabaseDSN: "file:test.db",
		LogLevel:    "info",
		LogDir:      "logs",
		Environment: "test",
	}
}

func completeLegacyConfig() config.Config {
	effective, err := ToEffectiveConfig(testBootstrapConfig(), validRuntimeConfig())
	if err != nil {
		panic(err)
	}
	return effective
}

func legacyControlPlane() config.ControlPlaneSettings {
	return config.ControlPlaneSettings{
		DSN:                "file:/app/data/control-plane.db?_foreign_keys=on",
		SuperAdminUsername: "seed-root",
		SuperAdminPassword: "seed-pass-not-default",
		SuperAdminReset:    true,
	}
}

func assertEffectiveControlPlane(t *testing.T, got config.ControlPlaneSettings, want config.ControlPlaneSettings) {
	t.Helper()
	if got.DSN != want.DSN || got.SuperAdminUsername != want.SuperAdminUsername ||
		got.SuperAdminPassword != want.SuperAdminPassword || got.SuperAdminReset != want.SuperAdminReset {
		t.Fatalf("EffectiveConfig.ControlPlane mismatch: got %#v want %#v", got, want)
	}
}

// TestResolverReadyExistingSnapshotCarriesLegacyControlPlane 锁定 ready 路径
// 一：库中已有运行时快照时，控制面配置仍从 legacy（env）回填。控制面是基础
// 设施配置，不进快照 schema；丢弃会让生产永远落到内存库且不建种子超管。
func TestResolverReadyExistingSnapshotCarriesLegacyControlPlane(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	if _, err := repository.Save(context.Background(), StatusReady, SourceUI, validRuntimeConfig()); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	legacy := completeLegacyConfig()
	legacy.ControlPlane = legacyControlPlane()

	result := NewResolver(repository).Resolve(context.Background(), testBootstrapConfig(), legacy)

	if result.Mode != ModeReady {
		t.Fatalf("unexpected resolution: %#v", result)
	}
	assertEffectiveControlPlane(t, result.EffectiveConfig.ControlPlane, legacy.ControlPlane)
}

// TestResolverReadyFirstStartAutoSnapshotCarriesLegacyControlPlane 锁定 ready
// 路径二：无快照时 FromLegacyConfig 自动落一条再走 resolveSnapshot。首启之后
// 永远 ready，控制面同样必须从 legacy 回填，不得因导入快照被整体丢弃。
func TestResolverReadyFirstStartAutoSnapshotCarriesLegacyControlPlane(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	legacy := completeLegacyConfig()
	legacy.ControlPlane = legacyControlPlane()

	result := NewResolver(repository).Resolve(context.Background(), testBootstrapConfig(), legacy)

	if result.Mode != ModeReady || result.Snapshot == nil || result.Snapshot.Source != SourceEnvImport {
		t.Fatalf("unexpected first-start resolution: %#v", result)
	}
	assertEffectiveControlPlane(t, result.EffectiveConfig.ControlPlane, legacy.ControlPlane)
}

// TestFromLegacyConfigDropsWebhookSecretChannel 锁定全局 secret 通道退役:
// env 直配(CODEHOST_WEBHOOK_SECRET/GITLAB_WEBHOOK_SECRET)经 Load→FromLegacyConfig
// 导入链被忽略,验签凭据唯一归属注册仓,env 导入不再产生任何 webhook secret。
func TestFromLegacyConfigDropsWebhookSecretChannel(t *testing.T) {
	t.Setenv("GITLAB_WEBHOOK_SECRET", "old-secret")
	t.Setenv("CODEHOST_WEBHOOK_SECRET", "new-secret")
	runtime, err := FromLegacyConfig(config.Load())
	if err != nil {
		t.Fatalf("import legacy config: %v", err)
	}
	encoded, err := json.Marshal(ApplyDefaults(runtime))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "new-secret") || strings.Contains(string(encoded), "old-secret") ||
		strings.Contains(string(encoded), "webhook_secret") {
		t.Fatalf("legacy webhook secret channel was re-emitted: %s", encoded)
	}
}
