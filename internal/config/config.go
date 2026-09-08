package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"sdlc-coordinator/internal/orchestrator"
)

const (
	defaultSchedulerInterval      = 30 * time.Second
	minimumSchedulerInterval      = 5 * time.Second
	defaultAgentRunnerTaskTimeout = 30 * time.Minute
	minimumAgentRunnerTaskTimeout = time.Minute
	maximumAgentRunnerTaskTimeout = 4 * time.Hour
)

type SchedulerSettings struct {
	Enabled     bool
	Interval    time.Duration
	TickRequest orchestrator.TickRequest
}

type AgentRunnerSettings struct {
	ServiceURL             string
	WorkspaceRoot          string
	Provider               string
	ProviderBaseURL        string
	APIKey                 string
	CustomHeaders          map[string]string
	Model                  string
	Effort                 string
	TaskTimeout            time.Duration
	ProviderConfigRevision uint64
}

// ErrControlPlaneNotConfigured 表示装配控制面时缺少 DSN 或种子超管（
// 二者均为启动必填，不再退内存库、不再无种子起进程）。
var ErrControlPlaneNotConfigured = errors.New("control plane is not configured")

// ControlPlaneSettings 是多租户控制面配置：DSN
// 与种子超管用户名/密码均为启动必填；RESET 仍是已有超管的密码丢失重置通道。
type ControlPlaneSettings struct {
	DSN                string
	SuperAdminUsername string
	SuperAdminPassword string
	SuperAdminReset    bool
}

func (s ControlPlaneSettings) Validate() error {
	if strings.TrimSpace(s.DSN) == "" {
		return fmt.Errorf("%w: CONTROL_PLANE_DSN is required", ErrControlPlaneNotConfigured)
	}
	if strings.TrimSpace(s.SuperAdminUsername) == "" || s.SuperAdminPassword == "" {
		return fmt.Errorf("%w: CONTROL_PLANE_SUPERADMIN_USERNAME and CONTROL_PLANE_SUPERADMIN_PASSWORD are required", ErrControlPlaneNotConfigured)
	}
	return nil
}

type Config struct {
	AgentRunner  AgentRunnerSettings
	HTTPAddr     string
	DatabaseDSN  string
	ControlPlane ControlPlaneSettings
	// TenantDataDir 是租户数据根（基础设施配置与 CONTROL_PLANE_* 同级，
	// 不入运行时快照 schema），env TENANT_DATA_DIR 可覆盖。
	TenantDataDir                string
	CodeHostProvider             string
	CodeHostBaseURL              string
	CodeHostReworkAllowedUserIDs string
	IM                           IMSettings
	ReviewerIMMentions           []string
	LogLevel                     string
	LogDir                       string
	Environment                  string
	Scheduler                    SchedulerSettings
}

// DefaultTenantDataDir 是租户数据根的默认值（相对进程工作目录）。
const DefaultTenantDataDir = "data/tenants"

func Load() Config {
	bootstrap := LoadBootstrap()
	codeHostProvider := strings.ToLower(getEnv("CODEHOST_PROVIDER", ""))
	codeHostBaseURL := getEnv("CODEHOST_BASE_URL", "")
	scheduler := loadSchedulerSettings()

	cfg := Config{
		ControlPlane:  bootstrap.ControlPlane,
		TenantDataDir: strings.TrimSpace(getEnv("TENANT_DATA_DIR", DefaultTenantDataDir)),
		AgentRunner: AgentRunnerSettings{
			ServiceURL:      getEnv("AGENT_RUNNER_SERVICE_URL", "http://agent-runner:8081"),
			WorkspaceRoot:   getEnv("AGENT_RUNNER_WORKSPACE_ROOT", "/app/workspaces"),
			Provider:        getEnv("AGENT_RUNNER_PROVIDER", "codex-cli"),
			ProviderBaseURL: providerEnv("AGENT_PROVIDER_BASE_URL", "CLAUDE_PROVIDER_BASE_URL", ""),
			APIKey:          providerEnv("AGENT_PROVIDER_API_KEY", "CLAUDE_PROVIDER_API_KEY", ""),
			CustomHeaders:   parseStringMap(providerEnv("AGENT_PROVIDER_CUSTOM_HEADERS_JSON", "CLAUDE_PROVIDER_CUSTOM_HEADERS_JSON", "")),
			Model:           providerEnv("AGENT_PROVIDER_MODEL", "CLAUDE_PROVIDER_MODEL", ""),
			Effort:          providerEnv("AGENT_PROVIDER_EFFORT", "CLAUDE_PROVIDER_EFFORT", "high"),
			TaskTimeout:     getEnvDurationBetween("AGENT_RUNNER_TASK_TIMEOUT", defaultAgentRunnerTaskTimeout, minimumAgentRunnerTaskTimeout, maximumAgentRunnerTaskTimeout),
		},
		HTTPAddr:                     bootstrap.HTTPAddr,
		DatabaseDSN:                  bootstrap.DatabaseDSN,
		CodeHostProvider:             codeHostProvider,
		CodeHostBaseURL:              codeHostBaseURL,
		CodeHostReworkAllowedUserIDs: loadReworkAllowedUserIDsEnv(),
		IM: IMSettings{
			Provider: strings.ToLower(strings.TrimSpace(getEnv("IM_PROVIDER", ""))),
			Telegram: IMTelegramSettings{
				BotToken:      getEnv("IM_TELEGRAM_BOT_TOKEN", ""),
				APIBase:       getEnv("IM_TELEGRAM_API_BASE", ""),
				DefaultChatID: getEnv("IM_TELEGRAM_DEFAULT_CHAT_ID", ""),
				MentionMap:    getEnv("IM_TELEGRAM_MENTION_MAP", ""),
			},
			Feishu: IMFeishuSettings{
				WebhookURL: getEnv("IM_FEISHU_WEBHOOK_URL", ""),
				Secret:     getEnv("IM_FEISHU_SECRET", ""),
				MentionMap: getEnv("IM_FEISHU_MENTION_MAP", ""),
			},
		},
		ReviewerIMMentions: parseCommaSeparatedUnique(getEnv("REVIEWER_IM_MENTIONS", "")),
		LogLevel:           bootstrap.LogLevel,
		LogDir:             bootstrap.LogDir,
		Environment:        bootstrap.Environment,
		Scheduler:          scheduler,
	}
	return cfg
}

func loadControlPlaneSettings() ControlPlaneSettings {
	return ControlPlaneSettings{
		DSN:                strings.TrimSpace(getEnv("CONTROL_PLANE_DSN", "")),
		SuperAdminUsername: strings.ToLower(strings.TrimSpace(getEnv("CONTROL_PLANE_SUPERADMIN_USERNAME", ""))),
		SuperAdminPassword: getEnv("CONTROL_PLANE_SUPERADMIN_PASSWORD", ""),
		SuperAdminReset:    getEnvBool("CONTROL_PLANE_SUPERADMIN_RESET", false),
	}
}

func parseStringMap(raw string) map[string]string {
	result := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return result
	}
	_ = json.Unmarshal([]byte(raw), &result)
	return result
}

func loadSchedulerSettings() SchedulerSettings {
	return SchedulerSettings{
		Enabled:  getEnvBool("COORDINATOR_SCHEDULER_ENABLED", false),
		Interval: getSchedulerInterval("COORDINATOR_SCHEDULER_INTERVAL", defaultSchedulerInterval),
		TickRequest: orchestrator.TickRequest{
			DispatchLimit:       getEnvNonNegativeInt("TICK_DISPATCH_LIMIT", 5),
			PollLimit:           getEnvNonNegativeInt("TICK_POLL_LIMIT", 20),
			ReconciliationLimit: getEnvNonNegativeInt("TICK_RECONCILIATION_LIMIT", 10),
			FinalizationLimit:   getEnvNonNegativeInt("TICK_FINALIZATION_LIMIT", 10),
			RecoveryLimit:       getEnvNonNegativeInt("TICK_RECOVERY_LIMIT", 10),
			EffectLimit:         getEnvNonNegativeInt("TICK_EFFECT_LIMIT", 0),
		},
	}
}

func firstConfigured(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func getEnvBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}

func getSchedulerInterval(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value < minimumSchedulerInterval {
		return fallback
	}
	return value
}

func getEnvDurationAtLeast(key string, fallback, minimum time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value < minimum {
		return fallback
	}
	return value
}

func getEnvDurationBetween(key string, fallback, minimum, maximum time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value < minimum || value > maximum {
		return fallback
	}
	return value
}

func getEnvBoundedInt(key string, fallback, minimum, maximum int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value < minimum || value > maximum {
		return fallback
	}
	return value
}

func getEnvNonNegativeInt(key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

func parseCommaSeparatedUnique(raw string) []string {
	values := strings.Split(raw, ",")
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func ParseCodeHostReworkAllowedUserIDs(raw string) ([]int, error) {
	if strings.TrimSpace(raw) == "" {
		return []int{}, nil
	}

	parts := strings.Split(raw, ",")
	result := make([]int, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		id, err := strconv.Atoi(value)
		if value == "" || err != nil || id <= 0 {
			return nil, fmt.Errorf("codehost rework allowed user ids has invalid user id %q", value)
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}

// loadReworkAllowedUserIDsEnv 读取平台级 rework 白名单。CODEHOST_REWORK_ALLOWED_USER_IDS
// 是现行规范名;GITLAB_REWORK_ALLOWED_USER_IDS 为历史名,读到时提示迁移。
func loadReworkAllowedUserIDsEnv() string {
	if value := getEnv("CODEHOST_REWORK_ALLOWED_USER_IDS", ""); value != "" {
		return value
	}
	if legacy := getEnv("GITLAB_REWORK_ALLOWED_USER_IDS", ""); legacy != "" {
		fmt.Fprintln(os.Stderr, "[deprecated] GITLAB_REWORK_ALLOWED_USER_IDS is renamed to CODEHOST_REWORK_ALLOWED_USER_IDS; the legacy value is still applied")
		return legacy
	}
	return ""
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func providerEnv(primary, legacy, fallback string) string {
	if value := getEnv(primary, ""); value != "" {
		return value
	}
	return getEnv(legacy, fallback)
}

// IMSettings 是 IM 通知层配置;Provider 为空/none 表示禁用通知。
type IMSettings struct {
	Provider string
	Telegram IMTelegramSettings
	Feishu   IMFeishuSettings
}

type IMTelegramSettings struct {
	BotToken      string
	APIBase       string
	DefaultChatID string
	MentionMap    string // JSON: 显示名 -> 数字用户 ID
}

type IMFeishuSettings struct {
	WebhookURL string
	Secret     string
	MentionMap string // JSON: 显示名 -> open_id
}
