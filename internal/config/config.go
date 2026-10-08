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
	InternalToken          string
	RemoteWorkspace        bool
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
	// PortalCookieSecure 控制门户会话 cookie 的 Secure 属性。nil（未设置
	// PORTAL_COOKIE_SECURE，或值不合法）按安全默认 true；明文 http 部署
	// 可显式设 0/false 关闭。
	PortalCookieSecure *bool
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
	TenantDataDir string
	LogLevel      string
	LogDir        string
	Environment   string
	Scheduler     SchedulerSettings
	// ResultChannelSecret 是 MCP 结果回传通道的部署级签名密钥（hex 64=
	// 32 字节）：多实例部署共用同一值，A 实例签发的 token B 实例可验，
	// 滚动重启不失效；未配置时进程内存随机（单机开发，重启即失效）。
	ResultChannelSecret string
}

// DefaultTenantDataDir 是租户数据根的默认值（相对进程工作目录）。
const DefaultTenantDataDir = "data/tenants"

func Load() Config {
	bootstrap := LoadBootstrap()
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
			InternalToken:   strings.TrimSpace(getEnv("AGENT_RUNNER_INTERNAL_TOKEN", "")),
			RemoteWorkspace: getEnvBool("AGENT_RUNNER_REMOTE_WORKSPACE", false),
		},
		HTTPAddr:    bootstrap.HTTPAddr,
		DatabaseDSN: bootstrap.DatabaseDSN,
		LogLevel:    bootstrap.LogLevel,
		LogDir:      bootstrap.LogDir,
		Environment: bootstrap.Environment,
		Scheduler:   scheduler,

		ResultChannelSecret: strings.TrimSpace(getEnv("RESULT_CHANNEL_SECRET", "")),
	}
	return cfg
}

func loadControlPlaneSettings() ControlPlaneSettings {
	return ControlPlaneSettings{
		DSN:                strings.TrimSpace(getEnv("CONTROL_PLANE_DSN", "")),
		SuperAdminUsername: strings.ToLower(strings.TrimSpace(getEnv("CONTROL_PLANE_SUPERADMIN_USERNAME", ""))),
		SuperAdminPassword: getEnv("CONTROL_PLANE_SUPERADMIN_PASSWORD", ""),
		SuperAdminReset:    getEnvBool("CONTROL_PLANE_SUPERADMIN_RESET", false),
		PortalCookieSecure: getEnvOptionalBool("PORTAL_COOKIE_SECURE"),
	}
}

// getEnvOptionalBool 返回 nil 表示环境变量未设置或值不合法（消费方取
// 安全默认，不让坏配置静默放宽）。
func getEnvOptionalBool(key string) *bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return nil
	}
	return &value
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
			GateLimit:           getEnvNonNegativeInt("TICK_GATE_LIMIT", 5),
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
