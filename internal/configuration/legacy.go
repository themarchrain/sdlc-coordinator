package configuration

import (
	"encoding/json"

	"sdlc-coordinator/internal/config"
)

// FromLegacyConfig 只搬运系统段（Agent Runner 与调度）；历史环境变量中的
// 组织段键（代码仓库、IM）已随配置双层化作废，不再导入。
func FromLegacyConfig(legacy config.Config) (RuntimeConfig, error) {
	customHeadersJSON := ""
	if len(legacy.AgentRunner.CustomHeaders) > 0 {
		if raw, err := json.Marshal(legacy.AgentRunner.CustomHeaders); err == nil {
			customHeadersJSON = string(raw)
		}
	}

	runtimeConfig := RuntimeConfig{
		AgentRunner: AgentRunnerConfig{
			ServiceURL: legacy.AgentRunner.ServiceURL,
			Provider:   legacy.AgentRunner.Provider, ProviderBaseURL: legacy.AgentRunner.ProviderBaseURL,
			APIKey: legacy.AgentRunner.APIKey, CustomHeadersJSON: customHeadersJSON,
			Model: legacy.AgentRunner.Model, Effort: legacy.AgentRunner.Effort, TaskTimeout: durationString(legacy.AgentRunner.TaskTimeout),
		},
		Scheduler: SchedulerConfig{
			Enabled:             legacy.Scheduler.Enabled,
			Interval:            durationString(legacy.Scheduler.Interval),
			CallbackBaseURL:     legacy.Scheduler.TickRequest.CallbackBaseURL,
			DispatchLimit:       legacy.Scheduler.TickRequest.DispatchLimit,
			PollLimit:           legacy.Scheduler.TickRequest.PollLimit,
			ReconciliationLimit: legacy.Scheduler.TickRequest.ReconciliationLimit,
			FinalizationLimit:   legacy.Scheduler.TickRequest.FinalizationLimit,
			RecoveryLimit:       legacy.Scheduler.TickRequest.RecoveryLimit,
			EffectLimit:         legacy.Scheduler.TickRequest.EffectLimit,
		},
	}
	return ApplyDefaults(runtimeConfig), nil
}

func durationString(value interface{ String() string }) string {
	return value.String()
}
