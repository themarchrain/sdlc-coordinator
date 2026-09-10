package configuration

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Mode string

const (
	ModeSetupPending       Mode = "setup_pending"
	ModeReady              Mode = "ready"
	ModeRestartRequired    Mode = "restart_required"
	ModeConfigurationError Mode = "configuration_error"
)

type SnapshotStatus string

const (
	StatusReady           SnapshotStatus = "ready"
	StatusRestartRequired SnapshotStatus = "restart_required"
)

type SnapshotSource string

const (
	SourceUI        SnapshotSource = "ui"
	SourceEnvImport SnapshotSource = "env_import"
)

type RuntimeConfig struct {
	AgentRunner AgentRunnerConfig `json:"agent_runner"`
	Scheduler   SchedulerConfig   `json:"scheduler"`
}

type AgentRunnerConfig struct {
	ServiceURL        string `json:"service_url"`
	WorkspaceRoot     string `json:"workspace_root"`
	Provider          string `json:"provider"`
	ProviderBaseURL   string `json:"provider_base_url"`
	APIKey            string `json:"api_key"`
	CustomHeadersJSON string `json:"custom_headers_json"`
	Model             string `json:"model"`
	Effort            string `json:"effort"`
	TaskTimeout       string `json:"task_timeout"`
}

type CodeHostConfig struct {
	Provider             string   `json:"provider"`
	BaseURL              string   `json:"base_url"`
	ReworkAllowedUserIDs []string `json:"rework_allowed_user_ids"`
}

// UnmarshalJSON 兼容历史快照:rework_allowed_user_ids 曾以数字数组落库,
// 现统一为字符串数组(跨平台数值域),解码时两种形态都接受。
func (c *CodeHostConfig) UnmarshalJSON(raw []byte) error {
	type codeHostConfigAlias CodeHostConfig
	var flexible struct {
		codeHostConfigAlias
		ReworkAllowedUserIDs json.RawMessage `json:"rework_allowed_user_ids"`
	}
	if err := json.Unmarshal(raw, &flexible); err != nil {
		return err
	}
	*c = CodeHostConfig(flexible.codeHostConfigAlias)

	rawIDs := strings.TrimSpace(string(flexible.ReworkAllowedUserIDs))
	if rawIDs == "" || rawIDs == "null" {
		return nil
	}
	var ids []string
	if err := json.Unmarshal(flexible.ReworkAllowedUserIDs, &ids); err == nil {
		c.ReworkAllowedUserIDs = ids
		return nil
	}
	var numericIDs []int
	if err := json.Unmarshal(flexible.ReworkAllowedUserIDs, &numericIDs); err != nil {
		return fmt.Errorf("decode codehost.rework_allowed_user_ids: %w", err)
	}
	ids = make([]string, 0, len(numericIDs))
	for _, id := range numericIDs {
		ids = append(ids, strconv.Itoa(id))
	}
	c.ReworkAllowedUserIDs = ids
	return nil
}

type IMConfig struct {
	Provider         string           `json:"provider"`
	Telegram         IMTelegramConfig `json:"telegram"`
	Feishu           IMFeishuConfig   `json:"feishu"`
	ReviewerMentions []string         `json:"reviewer_mentions"`
}

type IMTelegramConfig struct {
	BotToken      string            `json:"bot_token"`
	APIBase       string            `json:"api_base,omitempty"`
	DefaultChatID string            `json:"default_chat_id,omitempty"`
	MentionMap    map[string]string `json:"mention_map,omitempty"`
}

type IMFeishuConfig struct {
	WebhookURL string            `json:"webhook_url,omitempty"`
	Secret     string            `json:"secret,omitempty"`
	MentionMap map[string]string `json:"mention_map,omitempty"`
}

type SchedulerConfig struct {
	Enabled             bool   `json:"enabled"`
	Interval            string `json:"interval"`
	CallbackBaseURL     string `json:"callback_base_url"`
	WorkspaceRoot       string `json:"workspace_root"`
	DispatchLimit       int    `json:"dispatch_limit"`
	PollLimit           int    `json:"poll_limit"`
	ReconciliationLimit int    `json:"reconciliation_limit"`
	FinalizationLimit   int    `json:"finalization_limit"`
	RecoveryLimit       int    `json:"recovery_limit"`
	EffectLimit         int    `json:"effect_limit"`
}

type ValidationErrors map[string]string

func (e ValidationErrors) Has(field string) bool {
	_, exists := e[field]
	return exists
}

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	fields := make([]string, 0, len(e))
	for field := range e {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, field+": "+e[field])
	}
	return fmt.Sprintf("runtime configuration has %d validation error(s): %s", len(e), strings.Join(parts, "; "))
}

func validAgentRunnerEffort(value string) bool {
	switch strings.TrimSpace(value) {
	case "low", "medium", "high", "xhigh", "max":
		return true
	default:
		return false
	}
}

func validateDurationRange(errs ValidationErrors, field, raw string, minimum, maximum time.Duration) {
	value, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || value < minimum || value > maximum {
		errs[field] = fmt.Sprintf("必须是 %s 到 %s 之间的有效时长", minimum, maximum)
	}
}

func Validate(cfg RuntimeConfig) ValidationErrors {
	errs := ValidationErrors{}
	requireAbsoluteHTTPURL(errs, "agent_runner.service_url", cfg.AgentRunner.ServiceURL, true)
	requireTrimmed(errs, "agent_runner.workspace_root", cfg.AgentRunner.WorkspaceRoot)
	agentProvider := strings.TrimSpace(cfg.AgentRunner.Provider)
	if agentProvider != "codex-cli" && agentProvider != "claude-cli" {
		errs["agent_runner.provider"] = "仅支持 codex-cli 或 claude-cli"
	}
	requireAbsoluteHTTPURL(errs, "agent_runner.provider_base_url", cfg.AgentRunner.ProviderBaseURL, false)
	requireTrimmed(errs, "agent_runner.api_key", cfg.AgentRunner.APIKey)
	requireTrimmed(errs, "agent_runner.model", cfg.AgentRunner.Model)
	if !validAgentRunnerEffort(cfg.AgentRunner.Effort) {
		errs["agent_runner.effort"] = "仅支持 low、medium、high、xhigh 或 max"
	}
	validateDurationRange(errs, "agent_runner.task_timeout", cfg.AgentRunner.TaskTimeout, time.Minute, 4*time.Hour)
	if raw := strings.TrimSpace(cfg.AgentRunner.CustomHeadersJSON); raw != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(raw), &headers); err != nil || headers == nil {
			errs["agent_runner.custom_headers_json"] = "必须是字符串键值对 JSON 对象"
		}
	}

	requireAbsoluteHTTPURL(errs, "scheduler.callback_base_url", cfg.Scheduler.CallbackBaseURL, true)
	requireTrimmed(errs, "scheduler.workspace_root", cfg.Scheduler.WorkspaceRoot)
	validateDuration(errs, "scheduler.interval", cfg.Scheduler.Interval, 5*time.Second)
	validateNonNegative(errs, "scheduler.dispatch_limit", cfg.Scheduler.DispatchLimit)
	validateNonNegative(errs, "scheduler.poll_limit", cfg.Scheduler.PollLimit)
	validateNonNegative(errs, "scheduler.reconciliation_limit", cfg.Scheduler.ReconciliationLimit)
	validateNonNegative(errs, "scheduler.finalization_limit", cfg.Scheduler.FinalizationLimit)
	validateNonNegative(errs, "scheduler.recovery_limit", cfg.Scheduler.RecoveryLimit)
	validateNonNegative(errs, "scheduler.effect_limit", cfg.Scheduler.EffectLimit)

	return errs
}

func requireTrimmed(errs ValidationErrors, field, value string) {
	if strings.TrimSpace(value) == "" {
		errs[field] = "不能为空"
	}
}

func requireAbsoluteHTTPURL(errs ValidationErrors, field, value string, required bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			errs[field] = "不能为空"
		}
		return
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		errs[field] = "必须是完整的 http 或 https 地址"
	}
}

func validateDuration(errs ValidationErrors, field, raw string, minimum time.Duration) {
	value, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || value < minimum {
		errs[field] = fmt.Sprintf("必须是至少 %s 的有效时长", minimum)
	}
}

func validateNonNegative(errs ValidationErrors, field string, value int) {
	if value < 0 {
		errs[field] = "不能小于 0"
	}
}

func validEmail(raw string) bool {
	raw = strings.TrimSpace(raw)
	parsed, err := mail.ParseAddress(raw)
	return err == nil && parsed.Address == raw && strings.Contains(raw, "@")
}
