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
	if result.EffectiveConfig.CodeHostBaseURL != "https://legacy.gitlab.example.com" {
		t.Fatalf("unexpected effective config: %#v", result.EffectiveConfig)
	}
	if result.EffectiveConfig.AgentRunner.ProviderConfigRevision != result.Snapshot.Revision {
		t.Fatalf("agent runner provider revision was not pinned: %#v", result.EffectiveConfig.AgentRunner)
	}
}

func TestResolverIgnoresLegacyEnvironmentWhenSnapshotExists(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	persisted := validRuntimeConfig()
	persisted.CodeHost.BaseURL = "https://sqlite.gitlab.example.com"
	if _, err := repository.Save(context.Background(), StatusReady, SourceUI, persisted); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	legacy := completeLegacyConfig()
	legacy.CodeHostBaseURL = "https://conflicting-env.example.com"

	result := NewResolver(repository).Resolve(context.Background(), testBootstrapConfig(), legacy)

	if result.Mode != ModeReady || result.EffectiveConfig.CodeHostBaseURL != persisted.CodeHost.BaseURL {
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
		CodeHostProvider: "gitlab",
		CodeHostBaseURL:  "https://gitlab.example.com",
		Scheduler:        completeLegacyConfig().Scheduler,
	}

	result := NewResolver(repository).Resolve(context.Background(), testBootstrapConfig(), legacy)

	if result.Mode != ModeSetupPending || result.Snapshot != nil {
		t.Fatalf("unexpected resolution: %#v", result)
	}
	if result.Prefill.AgentRunner.ServiceURL != legacy.AgentRunner.ServiceURL || result.Prefill.CodeHost.BaseURL != legacy.CodeHostBaseURL {
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
	invalid.CodeHost.BaseURL = ""
	if _, err := repository.Save(context.Background(), StatusReady, SourceUI, invalid); err != nil {
		t.Fatalf("seed invalid snapshot: %v", err)
	}

	result := NewResolver(repository).Resolve(context.Background(), testBootstrapConfig(), completeLegacyConfig())

	if result.Mode != ModeConfigurationError || result.Snapshot == nil || !strings.Contains(result.ConfigurationError, "validation") {
		t.Fatalf("unexpected invalid snapshot resolution: %#v", result)
	}
	if result.EffectiveConfig.CodeHostBaseURL != "" {
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
	effective.CodeHostBaseURL = "https://legacy.gitlab.example.com"
	return effective
}

// TestFromLegacyConfigDropsWebhookSecretChannel env 中的 webhook secret
// 经 Load→FromLegacyConfig 导入链被忽略。验签凭据归属注册仓。
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

// TestFromLegacyConfigDropsGitLabBaseURLChannel 快照导入只认
// codehost.provider/base_url,GITLAB_BASE_URL 别名不会隐式映射。
func TestFromLegacyConfigDropsGitLabBaseURLChannel(t *testing.T) {
	runtime, err := FromLegacyConfig(config.Config{CodeHostProvider: ""})
	if err != nil {
		t.Fatalf("import legacy config: %v", err)
	}
	defaulted := ApplyDefaults(runtime)
	if defaulted.CodeHost.Provider != "gitlab" {
		t.Fatalf("provider default: %q", defaulted.CodeHost.Provider)
	}
	if defaulted.CodeHost.BaseURL != "" {
		t.Fatalf("no legacy channel may populate base_url, got %q", defaulted.CodeHost.BaseURL)
	}
}
