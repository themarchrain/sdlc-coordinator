package configuration

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"sdlc-coordinator/internal/config"
)

func TestValidationErrorsErrorListsFields(t *testing.T) {
	cfg := ApplyDefaults(RuntimeConfig{})
	errs := Validate(cfg)
	got := errs.Error()
	for _, field := range []string{
		"agent_runner.model",
		"agent_runner.api_key",
		"scheduler.callback_base_url",
	} {
		if !strings.Contains(got, field) {
			t.Fatalf("error should name %s, got %q", field, got)
		}
	}
}

func TestValidateRequiresCoreIntegrationFields(t *testing.T) {
	cfg := ApplyDefaults(RuntimeConfig{})

	errs := Validate(cfg)

	for _, field := range []string{
		"agent_runner.model",
		"agent_runner.api_key",
		"scheduler.callback_base_url",
	} {
		if !errs.Has(field) {
			t.Fatalf("missing validation error for %s: %#v", field, errs)
		}
	}
}

// TestValidateNoLongerRequiresWebhookSecret 运行配置不含全局 webhook_secret:
// 字段已删,编组输出不含该键,配置也不要求该值。
func TestValidateNoLongerRequiresWebhookSecret(t *testing.T) {
	cfg := validRuntimeConfig()
	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "webhook_secret") {
		t.Fatalf("webhook secret must no longer be part of runtime config: %s", encoded)
	}
	if errs := Validate(cfg); len(errs) != 0 {
		t.Fatalf("config without webhook secret must validate: %#v", errs)
	}
}

// TestSnapshotDecodeDropsOrgScopedSections 旧快照 JSON 中的 codehost/im 段
// (含历史 token/webhook_secret 键)在系统段收窄后解码被整体忽略:不报错、
// 不进入运行配置、重编码不再出现。
func TestSnapshotDecodeDropsOrgScopedSections(t *testing.T) {
	var cfg RuntimeConfig
	raw := `{"agent_runner":{},"codehost":{"provider":"gitlab","base_url":"https://gitlab.example.com","token":"legacy-global-secret","webhook_secret":"legacy-webhook-secret"},"im":{"provider":"telegram"},"scheduler":{}}`
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("decode legacy snapshot: %v", err)
	}
	encoded, err := json.Marshal(ApplyDefaults(cfg))
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"legacy-webhook-secret", "webhook_secret", "legacy-global-secret", "codehost", "\"im\""} {
		if strings.Contains(string(encoded), banned) {
			t.Fatalf("org-scoped section was re-emitted (%s): %s", banned, encoded)
		}
	}
}

func TestApplyDefaultsUsesExistingSafeRuntimeDefaults(t *testing.T) {
	got := ApplyDefaults(RuntimeConfig{})

	if got.AgentRunner.ServiceURL != "http://agent-runner:8081" || got.AgentRunner.WorkspaceRoot != "/app/workspaces" || got.AgentRunner.Provider != "codex-cli" || got.AgentRunner.Effort != "high" {
		t.Fatalf("unexpected agent runner defaults: %#v", got.AgentRunner)
	}
	if got.AgentRunner.TaskTimeout != "30m" {
		t.Fatalf("unexpected agent runner timeout default: %#v", got.AgentRunner)
	}
	if got.Scheduler.Interval != "30s" {
		t.Fatalf("unexpected scheduler defaults: %#v", got.Scheduler)
	}
	if got.Scheduler.DispatchLimit != 5 || got.Scheduler.PollLimit != 20 || got.Scheduler.EffectLimit != 0 {
		t.Fatalf("unexpected tick defaults: %#v", got.Scheduler)
	}
}

func TestCodeHostConfigUnmarshalAcceptsLegacyNumericAllowlist(t *testing.T) {
	legacy := []byte(`{"provider":"gitlab","base_url":"https://gitlab.example.com","rework_allowed_user_ids":[7,9],"assignee_email_map":{"7":"owner@example.com"}}`)
	var got CodeHostConfig
	if err := json.Unmarshal(legacy, &got); err != nil {
		t.Fatalf("decode legacy snapshot: %v", err)
	}
	if len(got.ReworkAllowedUserIDs) != 2 || got.ReworkAllowedUserIDs[0] != "7" || got.ReworkAllowedUserIDs[1] != "9" {
		t.Fatalf("expected numeric allowlist normalized to strings, got %#v", got.ReworkAllowedUserIDs)
	}

	current := []byte(`{"provider":"gitlab","rework_allowed_user_ids":["7","9"]}`)
	var currentConfig CodeHostConfig
	if err := json.Unmarshal(current, &currentConfig); err != nil {
		t.Fatalf("decode current snapshot: %v", err)
	}
	if len(currentConfig.ReworkAllowedUserIDs) != 2 || currentConfig.ReworkAllowedUserIDs[0] != "7" {
		t.Fatalf("unexpected string allowlist: %#v", currentConfig.ReworkAllowedUserIDs)
	}
}

func TestValidateRejectsMalformedTypedValues(t *testing.T) {
	cfg := validRuntimeConfig()
	cfg.Scheduler.Interval = "2s"
	cfg.Scheduler.DispatchLimit = -1
	cfg.AgentRunner.ServiceURL = "not-a-url"
	cfg.AgentRunner.CustomHeadersJSON = `{"X-Tenant":7}`
	cfg.AgentRunner.Effort = "extreme"
	cfg.AgentRunner.TaskTimeout = "30s"

	errs := Validate(cfg)
	for _, field := range []string{
		"scheduler.interval",
		"scheduler.dispatch_limit",
		"agent_runner.service_url",
		"agent_runner.custom_headers_json",
		"agent_runner.effort",
		"agent_runner.task_timeout",
	} {
		if !errs.Has(field) {
			t.Fatalf("expected validation error for %s: %#v", field, errs)
		}
	}
}

func TestValidateAcceptsRegisteredCodeLevelProvidersAndRejectsUnknownProvider(t *testing.T) {
	for _, provider := range []string{"codex-cli", "claude-cli"} {
		cfg := validRuntimeConfig()
		cfg.AgentRunner.Provider = provider
		if errs := Validate(cfg); errs.Has("agent_runner.provider") {
			t.Fatalf("registered provider %q was rejected: %#v", provider, errs)
		}
	}
	cfg := validRuntimeConfig()
	cfg.AgentRunner.Provider = "dynamic-plugin"
	if errs := Validate(cfg); !errs.Has("agent_runner.provider") {
		t.Fatalf("unknown provider was accepted: %#v", errs)
	}
}

func TestToEffectiveConfigPreservesBootstrapAndConvertsRuntime(t *testing.T) {
	runtimeConfig := validRuntimeConfig()
	bootstrap := config.BootstrapConfig{
		HTTPAddr:    ":9090",
		DatabaseDSN: "file:test.db",
		LogLevel:    "debug",
		LogDir:      "test-logs",
		Environment: "test",
	}

	got, err := ToEffectiveConfig(bootstrap, runtimeConfig)
	if err != nil {
		t.Fatalf("convert effective config: %v", err)
	}

	if got.HTTPAddr != ":9090" || got.DatabaseDSN != "file:test.db" || got.LogLevel != "debug" {
		t.Fatalf("bootstrap settings changed: %#v", got)
	}
	if got.AgentRunner.ServiceURL != runtimeConfig.AgentRunner.ServiceURL || got.AgentRunner.APIKey != runtimeConfig.AgentRunner.APIKey || got.AgentRunner.CustomHeaders["X-Tenant"] != "example-tenant" {
		t.Fatalf("agent runner settings not converted: %#v", got.AgentRunner)
	}
	if got.AgentRunner.TaskTimeout != 45*time.Minute {
		t.Fatalf("agent runner timeout not converted: %s", got.AgentRunner.TaskTimeout)
	}
	if got.AgentRunner.ProviderBaseURL != "https://gateway.example" {
		t.Fatalf("provider gateway url was not converted: %#v", got.AgentRunner)
	}
	if got.Scheduler.Interval != 45*time.Second {
		t.Fatalf("scheduler duration not converted: %s", got.Scheduler.Interval)
	}
}

func validRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		AgentRunner: AgentRunnerConfig{
			ServiceURL: "http://agent-runner:8081", WorkspaceRoot: "/app/workspaces", Provider: "codex-cli",
			ProviderBaseURL: "https://gateway.example", APIKey: "runner-api-key",
			CustomHeadersJSON: `{"X-Tenant":"example-tenant"}`, Model: "deepseek-v4-flash-0731", Effort: "high", TaskTimeout: "45m",
		},
		Scheduler: SchedulerConfig{
			Enabled:             true,
			Interval:            "45s",
			CallbackBaseURL:     "https://coordinator.example.com",
			WorkspaceRoot:       "/app/workspaces",
			DispatchLimit:       1,
			PollLimit:           5,
			ReconciliationLimit: 5,
			FinalizationLimit:   5,
			RecoveryLimit:       5,
			EffectLimit:         5,
		},
	}
}

func TestToEffectiveConfigKeepsProviderGatewayURL(t *testing.T) {
	runtimeConfig := validRuntimeConfig()

	got, err := ToEffectiveConfig(config.BootstrapConfig{HTTPAddr: ":9090", DatabaseDSN: "file:test.db"}, runtimeConfig)
	if err != nil {
		t.Fatalf("convert effective config: %v", err)
	}
	if got.AgentRunner.ProviderBaseURL != "https://gateway.example" {
		t.Fatalf("provider gateway url must pass through untouched: %#v", got.AgentRunner)
	}
}

func TestRuntimeConfigDropsLegacyTranslatorFields(t *testing.T) {
	base, err := json.Marshal(validRuntimeConfig())
	if err != nil {
		t.Fatalf("marshal runtime config: %v", err)
	}
	var generic map[string]any
	if err := json.Unmarshal(base, &generic); err != nil {
		t.Fatalf("unmarshal runtime config: %v", err)
	}
	agent, _ := generic["agent_runner"].(map[string]any)
	agent["translator_url"] = "http://litellm:4000/v1"
	agent["cli"] = map[string]any{"codex-cli": map[string]any{"normalize_to_chat": false}}
	legacy, err := json.Marshal(generic)
	if err != nil {
		t.Fatalf("marshal legacy payload: %v", err)
	}

	var cfg RuntimeConfig
	if err := json.Unmarshal(legacy, &cfg); err != nil {
		t.Fatalf("legacy snapshot must still unmarshal: %v", err)
	}
	if errs := Validate(cfg); len(errs) != 0 {
		t.Fatalf("legacy snapshot must validate: %#v", errs)
	}
	roundtrip, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("remarshal runtime config: %v", err)
	}
	if strings.Contains(string(roundtrip), "translator_url") || strings.Contains(string(roundtrip), "normalize_to_chat") {
		t.Fatalf("legacy translator fields were re-emitted: %s", roundtrip)
	}
}

// TestRuntimeConfigTypeHasNoControlPlaneField 锁定既有修复口径：控制面是基础设施
// 配置，不得进入运行时快照 schema。
func TestRuntimeConfigTypeHasNoControlPlaneField(t *testing.T) {
	typ := reflect.TypeOf(RuntimeConfig{})
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		tag := typ.Field(i).Tag.Get("json")
		if strings.Contains(strings.ToLower(name), "controlplane") || strings.Contains(strings.ToLower(tag), "control_plane") {
			t.Fatalf("RuntimeConfig must not carry control plane: field %s json=%q", name, tag)
		}
	}
	encoded, err := json.Marshal(RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(encoded)), "control_plane") {
		t.Fatalf("snapshot JSON must omit control_plane: %s", encoded)
	}
}
