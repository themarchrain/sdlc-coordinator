package config

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestLoadSchedulerDefaults(t *testing.T) {
	t.Setenv("COORDINATOR_SCHEDULER_ENABLED", "")
	t.Setenv("COORDINATOR_SCHEDULER_INTERVAL", "")

	cfg := Load()

	if cfg.Scheduler.Enabled {
		t.Fatal("expected internal scheduler to be disabled by default")
	}
	if cfg.Scheduler.Interval != 30*time.Second {
		t.Fatalf("expected default scheduler interval 30s, got %s", cfg.Scheduler.Interval)
	}
}

func TestLoadSchedulerAndTickSettings(t *testing.T) {
	t.Setenv("COORDINATOR_SCHEDULER_ENABLED", "true")
	t.Setenv("COORDINATOR_SCHEDULER_INTERVAL", "45s")
	t.Setenv("TICK_DISPATCH_LIMIT", "1")
	t.Setenv("TICK_POLL_LIMIT", "5")
	t.Setenv("TICK_RECONCILIATION_LIMIT", "6")
	t.Setenv("TICK_FINALIZATION_LIMIT", "7")
	t.Setenv("TICK_RECOVERY_LIMIT", "8")
	t.Setenv("TICK_EFFECT_LIMIT", "5")

	cfg := Load()

	if !cfg.Scheduler.Enabled || cfg.Scheduler.Interval != 45*time.Second {
		t.Fatalf("unexpected scheduler settings: %#v", cfg.Scheduler)
	}
	request := cfg.Scheduler.TickRequest
	if request.DispatchLimit != 1 || request.PollLimit != 5 || request.ReconciliationLimit != 6 || request.FinalizationLimit != 7 || request.RecoveryLimit != 8 || request.EffectLimit != 5 {
		t.Fatalf("unexpected tick limits: %#v", request)
	}
}

func TestLoadSchedulerInvalidValuesFallBackToSafeDefaults(t *testing.T) {
	t.Setenv("COORDINATOR_SCHEDULER_ENABLED", "definitely")
	t.Setenv("COORDINATOR_SCHEDULER_INTERVAL", "2s")
	t.Setenv("TICK_DISPATCH_LIMIT", "-1")
	t.Setenv("TICK_EFFECT_LIMIT", "invalid")

	cfg := Load()

	if cfg.Scheduler.Enabled {
		t.Fatal("expected invalid enabled value to fall back to false")
	}
	if cfg.Scheduler.Interval != 30*time.Second {
		t.Fatalf("expected short interval to fall back to 30s, got %s", cfg.Scheduler.Interval)
	}
	if cfg.Scheduler.TickRequest.DispatchLimit != 5 || cfg.Scheduler.TickRequest.EffectLimit != 0 {
		t.Fatalf("unexpected tick fallbacks: %#v", cfg.Scheduler.TickRequest)
	}
}

func TestLoadCodeHostConfig(t *testing.T) {
	t.Setenv("CODEHOST_PROVIDER", "Gitea")
	t.Setenv("CODEHOST_BASE_URL", "http://localhost:3000")

	cfg := Load()

	if cfg.CodeHostProvider != "gitea" {
		t.Fatalf("expected gitea provider, got %q", cfg.CodeHostProvider)
	}
	if cfg.CodeHostBaseURL != "http://localhost:3000" {
		t.Fatalf("expected codehost base url, got %q", cfg.CodeHostBaseURL)
	}
}

func TestLoadAgentRunnerLegacyEnvironment(t *testing.T) {
	t.Setenv("AGENT_RUNNER_SERVICE_URL", "http://runner.internal:8081")
	t.Setenv("AGENT_RUNNER_WORKSPACE_ROOT", "/srv/workspaces")
	t.Setenv("CLAUDE_PROVIDER_BASE_URL", "https://gateway.example")
	t.Setenv("CLAUDE_PROVIDER_API_KEY", "runner-key")
	t.Setenv("CLAUDE_PROVIDER_CUSTOM_HEADERS_JSON", `{"X-Tenant":"example-tenant"}`)
	t.Setenv("CLAUDE_PROVIDER_MODEL", "model-a")
	t.Setenv("CLAUDE_PROVIDER_EFFORT", "xhigh")
	t.Setenv("AGENT_RUNNER_TASK_TIMEOUT", "45m")

	settings := Load().AgentRunner
	if settings.ServiceURL != "http://runner.internal:8081" || settings.WorkspaceRoot != "/srv/workspaces" || settings.APIKey != "runner-key" {
		t.Fatalf("unexpected agent runner environment: %#v", settings)
	}
	if settings.CustomHeaders["X-Tenant"] != "example-tenant" || settings.Model != "model-a" || settings.Effort != "xhigh" {
		t.Fatalf("unexpected agent runner provider environment: %#v", settings)
	}
	if settings.TaskTimeout != 45*time.Minute {
		t.Fatalf("unexpected agent runner task timeout: %s", settings.TaskTimeout)
	}
}

func TestLoadAgentRunnerDefaultsToCodexCLI(t *testing.T) {
	t.Setenv("AGENT_RUNNER_PROVIDER", "")
	if got := Load().AgentRunner.Provider; got != "codex-cli" {
		t.Fatalf("expected codex-cli default, got %q", got)
	}
}

func TestLoadAgentRunnerUsesProviderNeutralEnvironmentBeforeLegacyClaudeNames(t *testing.T) {
	t.Setenv("AGENT_PROVIDER_BASE_URL", "https://responses.example/v1")
	t.Setenv("AGENT_PROVIDER_API_KEY", "codex-key")
	t.Setenv("AGENT_PROVIDER_CUSTOM_HEADERS_JSON", `{"X-Provider":"codex"}`)
	t.Setenv("AGENT_PROVIDER_MODEL", "codex-model")
	t.Setenv("AGENT_PROVIDER_EFFORT", "medium")
	t.Setenv("CLAUDE_PROVIDER_BASE_URL", "https://legacy.example")
	t.Setenv("CLAUDE_PROVIDER_API_KEY", "legacy-key")

	settings := Load().AgentRunner
	if settings.ProviderBaseURL != "https://responses.example/v1" || settings.APIKey != "codex-key" || settings.Model != "codex-model" || settings.Effort != "medium" || settings.CustomHeaders["X-Provider"] != "codex" {
		t.Fatalf("provider-neutral environment was not preferred: %#v", settings)
	}
}

func TestLoadAgentRunnerTaskTimeoutUsesSafeDefault(t *testing.T) {
	t.Setenv("AGENT_RUNNER_TASK_TIMEOUT", "30s")
	if got := Load().AgentRunner.TaskTimeout; got != 30*time.Minute {
		t.Fatalf("expected short timeout to fall back to 30m, got %s", got)
	}
	t.Setenv("AGENT_RUNNER_TASK_TIMEOUT", "5h")
	if got := Load().AgentRunner.TaskTimeout; got != 30*time.Minute {
		t.Fatalf("expected long timeout to fall back to 30m, got %s", got)
	}
}

func TestLoadIMConfig(t *testing.T) {
	t.Setenv("IM_PROVIDER", "Telegram")
	t.Setenv("IM_TELEGRAM_BOT_TOKEN", "bot-token")
	t.Setenv("IM_TELEGRAM_DEFAULT_CHAT_ID", "-1001")
	t.Setenv("IM_FEISHU_WEBHOOK_URL", "https://open.feishu.cn/hook/x")
	t.Setenv("REVIEWER_IM_MENTIONS", "review-owner@example.com, demo.user@example.com, review-owner@example.com")

	cfg := Load()

	if cfg.IM.Provider != "telegram" {
		t.Fatalf("expected normalized telegram provider, got %q", cfg.IM.Provider)
	}
	if cfg.IM.Telegram.BotToken != "bot-token" || cfg.IM.Telegram.DefaultChatID != "-1001" {
		t.Fatalf("unexpected telegram settings: %#v", cfg.IM.Telegram)
	}
	if cfg.IM.Feishu.WebhookURL == "" {
		t.Fatal("feishu settings must load independently of provider")
	}
	if len(cfg.ReviewerIMMentions) != 2 || cfg.ReviewerIMMentions[0] != "review-owner@example.com" || cfg.ReviewerIMMentions[1] != "demo.user@example.com" {
		t.Fatalf("unexpected reviewer IM mentions: %#v", cfg.ReviewerIMMentions)
	}
}

// TestLoadDropsGitLabBaseURLAlias 锁定 GITLAB_BASE_URL 别名通道退役:平台
// 身份唯一归属注册仓行,全局 env 只认 CODEHOST_* 规范名,别名注入不生效。
func TestLoadDropsGitLabBaseURLAlias(t *testing.T) {
	t.Setenv("CODEHOST_PROVIDER", "")
	t.Setenv("CODEHOST_BASE_URL", "")
	t.Setenv("GITLAB_BASE_URL", "https://gitlab.example")

	cfg := Load()

	if cfg.CodeHostProvider != "" || cfg.CodeHostBaseURL != "" {
		t.Fatalf("GITLAB_BASE_URL alias must be retired, got provider=%q base_url=%q",
			cfg.CodeHostProvider, cfg.CodeHostBaseURL)
	}
}

// TestLoadDropsWebhookSecretEnv 锁定全局 secret 通道退役:env 直配
// (GITLAB_WEBHOOK_SECRET/CODEHOST_WEBHOOK_SECRET)不再被读取,
// 验签凭据唯一归属注册仓。
func TestLoadDropsWebhookSecretEnv(t *testing.T) {
	t.Setenv("GITLAB_WEBHOOK_SECRET", "webhook-secret-1")
	t.Setenv("CODEHOST_WEBHOOK_SECRET", "webhook-secret-2")

	cfg := Load()

	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "webhook-secret-1") || strings.Contains(string(encoded), "webhook-secret-2") {
		t.Fatalf("webhook secret env must be retired, got %s", encoded)
	}
}

func TestParseCodeHostReworkAllowedUserIDs(t *testing.T) {
	ids, err := ParseCodeHostReworkAllowedUserIDs(" 99,42,99 ")
	if err != nil {
		t.Fatalf("parse allowed user ids: %v", err)
	}
	if len(ids) != 2 || ids[0] != 99 || ids[1] != 42 {
		t.Fatalf("unexpected allowed user ids: %#v", ids)
	}

	empty, err := ParseCodeHostReworkAllowedUserIDs(" ")
	if err != nil || len(empty) != 0 {
		t.Fatalf("expected empty allowlist, got %#v error=%v", empty, err)
	}
}

func TestParseCodeHostReworkAllowedUserIDsRejectsInvalidValues(t *testing.T) {
	for _, raw := range []string{"0", "-1", "not-a-number", "42,,99"} {
		if _, err := ParseCodeHostReworkAllowedUserIDs(raw); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}

func TestLoadCodeHostReworkAllowedUserIDs(t *testing.T) {
	t.Setenv("CODEHOST_REWORK_ALLOWED_USER_IDS", "99,42")

	cfg := Load()

	if cfg.CodeHostReworkAllowedUserIDs != "99,42" {
		t.Fatalf("unexpected raw rework allowlist: %q", cfg.CodeHostReworkAllowedUserIDs)
	}
}

func TestLoadCodeHostReworkAllowedUserIDsLegacyEnvFallback(t *testing.T) {
	t.Setenv("CODEHOST_REWORK_ALLOWED_USER_IDS", "")
	t.Setenv("GITLAB_REWORK_ALLOWED_USER_IDS", "42")

	cfg := Load()

	if cfg.CodeHostReworkAllowedUserIDs != "42" {
		t.Fatalf("expected legacy env fallback, got %q", cfg.CodeHostReworkAllowedUserIDs)
	}
}

func TestLoadCodeHostReworkAllowedUserIDsPrefersCanonicalEnv(t *testing.T) {
	t.Setenv("CODEHOST_REWORK_ALLOWED_USER_IDS", "7")
	t.Setenv("GITLAB_REWORK_ALLOWED_USER_IDS", "42")

	cfg := Load()

	if cfg.CodeHostReworkAllowedUserIDs != "7" {
		t.Fatalf("expected canonical env to win, got %q", cfg.CodeHostReworkAllowedUserIDs)
	}
}

func TestLoadLoggingConfigDefaults(t *testing.T) {
	t.Setenv("COORDINATOR_LOG_LEVEL", "")
	t.Setenv("COORDINATOR_LOG_DIR", "")

	cfg := Load()

	if cfg.LogLevel != "info" {
		t.Fatalf("expected default log level info, got %q", cfg.LogLevel)
	}
	if cfg.LogDir != "logs" {
		t.Fatalf("expected default log directory logs, got %q", cfg.LogDir)
	}
}

func TestLoadLoggingConfigFromEnvironment(t *testing.T) {
	t.Setenv("COORDINATOR_LOG_LEVEL", "debug")
	t.Setenv("COORDINATOR_LOG_DIR", "D:/coordinator-logs")

	cfg := Load()

	if cfg.LogLevel != "debug" {
		t.Fatalf("expected configured log level debug, got %q", cfg.LogLevel)
	}
	if cfg.LogDir != "D:/coordinator-logs" {
		t.Fatalf("expected configured log directory, got %q", cfg.LogDir)
	}
}

func TestLoadIgnoresLegacyTranslatorEnv(t *testing.T) {
	t.Setenv("AGENT_TRANSLATOR_URL", "http://127.0.0.1:4000/v1")
	t.Setenv("AGENT_PROVIDER_BASE_URL", "https://gateway.example")

	cfg := Load()

	if cfg.AgentRunner.ProviderBaseURL != "https://gateway.example" {
		t.Fatalf("provider base url not loaded: %#v", cfg.AgentRunner)
	}
}
