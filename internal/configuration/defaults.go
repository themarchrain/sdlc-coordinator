package configuration

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/orchestrator"
)

func ApplyDefaults(cfg RuntimeConfig) RuntimeConfig {
	if strings.TrimSpace(cfg.AgentRunner.ServiceURL) == "" {
		cfg.AgentRunner.ServiceURL = "http://agent-runner:8081"
	}
	if strings.TrimSpace(cfg.AgentRunner.WorkspaceRoot) == "" {
		cfg.AgentRunner.WorkspaceRoot = "/app/workspaces"
	}
	if strings.TrimSpace(cfg.AgentRunner.Provider) == "" {
		cfg.AgentRunner.Provider = "codex-cli"
	}
	if strings.TrimSpace(cfg.AgentRunner.Effort) == "" {
		cfg.AgentRunner.Effort = "high"
	}
	if strings.TrimSpace(cfg.AgentRunner.TaskTimeout) == "" {
		cfg.AgentRunner.TaskTimeout = "30m"
	}
	if strings.TrimSpace(cfg.CodeHost.Provider) == "" {
		cfg.CodeHost.Provider = "gitlab"
	}
	if cfg.CodeHost.ReworkAllowedUserIDs == nil {
		cfg.CodeHost.ReworkAllowedUserIDs = []string{}
	}
	if cfg.IM.ReviewerMentions == nil {
		cfg.IM.ReviewerMentions = []string{}
	}
	if cfg.IM.Telegram.MentionMap == nil {
		cfg.IM.Telegram.MentionMap = map[string]string{}
	}
	if cfg.IM.Feishu.MentionMap == nil {
		cfg.IM.Feishu.MentionMap = map[string]string{}
	}

	schedulerWasEmpty := cfg.Scheduler == (SchedulerConfig{})
	if strings.TrimSpace(cfg.Scheduler.Interval) == "" {
		cfg.Scheduler.Interval = "30s"
	}
	if schedulerWasEmpty {
		cfg.Scheduler.DispatchLimit = 5
		cfg.Scheduler.PollLimit = 20
		cfg.Scheduler.ReconciliationLimit = 10
		cfg.Scheduler.FinalizationLimit = 10
		cfg.Scheduler.RecoveryLimit = 10
		cfg.Scheduler.EffectLimit = 0
	}

	return cfg
}

func ToEffectiveConfig(bootstrap config.BootstrapConfig, runtimeConfig RuntimeConfig) (config.Config, error) {
	runtimeConfig = ApplyDefaults(runtimeConfig)
	if errs := Validate(runtimeConfig); len(errs) > 0 {
		return config.Config{}, errs
	}
	schedulerInterval, err := time.ParseDuration(runtimeConfig.Scheduler.Interval)
	if err != nil {
		return config.Config{}, fmt.Errorf("parse scheduler interval: %w", err)
	}
	agentRunnerTaskTimeout, err := time.ParseDuration(runtimeConfig.AgentRunner.TaskTimeout)
	if err != nil {
		return config.Config{}, fmt.Errorf("parse agent runner task timeout: %w", err)
	}
	customHeaders := map[string]string{}
	if raw := strings.TrimSpace(runtimeConfig.AgentRunner.CustomHeadersJSON); raw != "" {
		if err := json.Unmarshal([]byte(raw), &customHeaders); err != nil {
			return config.Config{}, fmt.Errorf("decode agent runner custom headers: %w", err)
		}
	}

	provider := strings.ToLower(strings.TrimSpace(runtimeConfig.CodeHost.Provider))
	result := config.Config{
		AgentRunner: config.AgentRunnerSettings{
			ServiceURL:      strings.TrimRight(strings.TrimSpace(runtimeConfig.AgentRunner.ServiceURL), "/"),
			WorkspaceRoot:   strings.TrimSpace(runtimeConfig.AgentRunner.WorkspaceRoot),
			Provider:        strings.TrimSpace(runtimeConfig.AgentRunner.Provider),
			ProviderBaseURL: strings.TrimRight(strings.TrimSpace(runtimeConfig.AgentRunner.ProviderBaseURL), "/"),
			APIKey:          runtimeConfig.AgentRunner.APIKey,
			CustomHeaders:   customHeaders,
			Model:           strings.TrimSpace(runtimeConfig.AgentRunner.Model),
			Effort:          strings.TrimSpace(runtimeConfig.AgentRunner.Effort),
			TaskTimeout:     agentRunnerTaskTimeout,
		},
		HTTPAddr:                     bootstrap.HTTPAddr,
		DatabaseDSN:                  bootstrap.DatabaseDSN,
		CodeHostProvider:             provider,
		CodeHostBaseURL:              strings.TrimRight(strings.TrimSpace(runtimeConfig.CodeHost.BaseURL), "/"),
		CodeHostReworkAllowedUserIDs: joinNumericIDs(runtimeConfig.CodeHost.ReworkAllowedUserIDs),
		IM:                           encodeIMSettings(runtimeConfig.IM),
		ReviewerIMMentions:           append([]string(nil), runtimeConfig.IM.ReviewerMentions...),
		LogLevel:                     bootstrap.LogLevel,
		LogDir:                       bootstrap.LogDir,
		Environment:                  bootstrap.Environment,
		ControlPlane:                 bootstrap.ControlPlane,
		Scheduler: config.SchedulerSettings{
			Enabled:  runtimeConfig.Scheduler.Enabled,
			Interval: schedulerInterval,
			TickRequest: orchestrator.TickRequest{
				CallbackBaseURL:     runtimeConfig.Scheduler.CallbackBaseURL,
				WorkspaceRoot:       runtimeConfig.Scheduler.WorkspaceRoot,
				DispatchLimit:       runtimeConfig.Scheduler.DispatchLimit,
				PollLimit:           runtimeConfig.Scheduler.PollLimit,
				ReconciliationLimit: runtimeConfig.Scheduler.ReconciliationLimit,
				FinalizationLimit:   runtimeConfig.Scheduler.FinalizationLimit,
				RecoveryLimit:       runtimeConfig.Scheduler.RecoveryLimit,
				EffectLimit:         runtimeConfig.Scheduler.EffectLimit,
			},
		},
	}
	return result, nil
}

func joinNumericIDs(values []string) string {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			unique[trimmed] = struct{}{}
		}
	}
	numbers := make([]int, 0, len(unique))
	for value := range unique {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			continue
		}
		numbers = append(numbers, parsed)
	}
	sort.Ints(numbers)
	parts := make([]string, 0, len(numbers))
	for _, value := range numbers {
		parts = append(parts, strconv.Itoa(value))
	}
	return strings.Join(parts, ",")
}

func encodeIMSettings(im IMConfig) config.IMSettings {
	provider := strings.ToLower(strings.TrimSpace(im.Provider))
	return config.IMSettings{
		Provider: provider,
		Telegram: config.IMTelegramSettings{
			BotToken:      im.Telegram.BotToken,
			APIBase:       im.Telegram.APIBase,
			DefaultChatID: im.Telegram.DefaultChatID,
			MentionMap:    encodeStringMap(im.Telegram.MentionMap),
		},
		Feishu: config.IMFeishuSettings{
			WebhookURL: im.Feishu.WebhookURL,
			Secret:     im.Feishu.Secret,
			MentionMap: encodeStringMap(im.Feishu.MentionMap),
		},
	}
}

func encodeStringMap(source map[string]string) string {
	if len(source) == 0 {
		return ""
	}
	encoded, err := json.Marshal(source)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// IsValidPlatformUserID 校验平台用户 ID 的字符串形态:非空、纯数字、非零。
// 三平台数值域不同,配置面统一以字符串承载。
func IsValidPlatformUserID(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "0" {
		return false
	}
	for _, r := range trimmed {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
