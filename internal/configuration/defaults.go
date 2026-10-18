package configuration

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/orchestrator"
)

func ApplyDefaults(cfg RuntimeConfig) RuntimeConfig {
	if strings.TrimSpace(cfg.AgentRunner.ServiceURL) == "" {
		cfg.AgentRunner.ServiceURL = "http://agent-runner:8081"
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

	result := config.Config{
		AgentRunner: config.AgentRunnerSettings{
			ServiceURL:      strings.TrimRight(strings.TrimSpace(runtimeConfig.AgentRunner.ServiceURL), "/"),
			Provider:        strings.TrimSpace(runtimeConfig.AgentRunner.Provider),
			ProviderBaseURL: strings.TrimRight(strings.TrimSpace(runtimeConfig.AgentRunner.ProviderBaseURL), "/"),
			APIKey:          runtimeConfig.AgentRunner.APIKey,
			CustomHeaders:   customHeaders,
			Model:           strings.TrimSpace(runtimeConfig.AgentRunner.Model),
			Effort:          strings.TrimSpace(runtimeConfig.AgentRunner.Effort),
			TaskTimeout:     agentRunnerTaskTimeout,
		},
		HTTPAddr:     bootstrap.HTTPAddr,
		DatabaseDSN:  bootstrap.DatabaseDSN,
		LogLevel:     bootstrap.LogLevel,
		LogDir:       bootstrap.LogDir,
		Environment:  bootstrap.Environment,
		ControlPlane: bootstrap.ControlPlane,
		Scheduler: config.SchedulerSettings{
			Enabled:  runtimeConfig.Scheduler.Enabled,
			Interval: schedulerInterval,
			TickRequest: orchestrator.TickRequest{
				CallbackBaseURL:     runtimeConfig.Scheduler.CallbackBaseURL,
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
