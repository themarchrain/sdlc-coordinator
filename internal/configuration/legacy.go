package configuration

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"sdlc-coordinator/internal/config"
)

func FromLegacyConfig(legacy config.Config) (RuntimeConfig, error) {
	provider := strings.ToLower(strings.TrimSpace(legacy.CodeHostProvider))
	baseURL := strings.TrimSpace(legacy.CodeHostBaseURL)
	reworkAllowedInts, reworkErr := config.ParseCodeHostReworkAllowedUserIDs(legacy.CodeHostReworkAllowedUserIDs)
	reworkAllowed := make([]string, 0, len(reworkAllowedInts))
	for _, id := range reworkAllowedInts {
		reworkAllowed = append(reworkAllowed, strconv.Itoa(id))
	}
	customHeadersJSON := ""
	if len(legacy.AgentRunner.CustomHeaders) > 0 {
		if raw, err := json.Marshal(legacy.AgentRunner.CustomHeaders); err == nil {
			customHeadersJSON = string(raw)
		}
	}

	runtimeConfig := RuntimeConfig{
		AgentRunner: AgentRunnerConfig{
			ServiceURL: legacy.AgentRunner.ServiceURL, WorkspaceRoot: legacy.AgentRunner.WorkspaceRoot,
			Provider: legacy.AgentRunner.Provider, ProviderBaseURL: legacy.AgentRunner.ProviderBaseURL,
			APIKey: legacy.AgentRunner.APIKey, CustomHeadersJSON: customHeadersJSON,
			Model: legacy.AgentRunner.Model, Effort: legacy.AgentRunner.Effort, TaskTimeout: durationString(legacy.AgentRunner.TaskTimeout),
		},
		CodeHost: CodeHostConfig{
			Provider:             provider,
			BaseURL:              baseURL,
			ReworkAllowedUserIDs: reworkAllowed,
		},
		IM: IMConfig{
			ReviewerMentions: append([]string(nil), legacy.ReviewerIMMentions...),
		},
		Scheduler: SchedulerConfig{
			Enabled:             legacy.Scheduler.Enabled,
			Interval:            durationString(legacy.Scheduler.Interval),
			CallbackBaseURL:     legacy.Scheduler.TickRequest.CallbackBaseURL,
			WorkspaceRoot:       legacy.Scheduler.TickRequest.WorkspaceRoot,
			DispatchLimit:       legacy.Scheduler.TickRequest.DispatchLimit,
			PollLimit:           legacy.Scheduler.TickRequest.PollLimit,
			ReconciliationLimit: legacy.Scheduler.TickRequest.ReconciliationLimit,
			FinalizationLimit:   legacy.Scheduler.TickRequest.FinalizationLimit,
			RecoveryLimit:       legacy.Scheduler.TickRequest.RecoveryLimit,
			EffectLimit:         legacy.Scheduler.TickRequest.EffectLimit,
		},
	}
	runtimeConfig = ApplyDefaults(runtimeConfig)
	if reworkErr != nil {
		return runtimeConfig, fmt.Errorf("parse legacy rework allowlist: %w", reworkErr)
	}
	return runtimeConfig, nil
}

func durationString(value interface{ String() string }) string {
	return value.String()
}
