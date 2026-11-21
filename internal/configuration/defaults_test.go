package configuration

import (
	"testing"

	"sdlc-coordinator/internal/config"
)

func TestApplyDefaultsSetsPiAPI(t *testing.T) {
	if got := ApplyDefaults(RuntimeConfig{}).AgentRunner.PiAPI; got != "openai-completions" {
		t.Fatalf("pi api default = %q, want openai-completions", got)
	}
	explicit := RuntimeConfig{AgentRunner: AgentRunnerConfig{PiAPI: "anthropic-messages"}}
	if got := ApplyDefaults(explicit).AgentRunner.PiAPI; got != "anthropic-messages" {
		t.Fatalf("explicit pi api was overwritten: %q", got)
	}
}

func TestToEffectiveConfigCarriesPiAPI(t *testing.T) {
	runtimeConfig := validRuntimeConfig()
	runtimeConfig.AgentRunner.Provider = "pi-cli"
	runtimeConfig.AgentRunner.PiAPI = "anthropic-messages"
	effective, err := ToEffectiveConfig(config.BootstrapConfig{HTTPAddr: ":9090", DatabaseDSN: "file:test.db"}, runtimeConfig)
	if err != nil {
		t.Fatalf("to effective config: %v", err)
	}
	if effective.AgentRunner.PiAPI != "anthropic-messages" {
		t.Fatalf("effective pi api = %q", effective.AgentRunner.PiAPI)
	}
}

func TestEditableAgentRunnerPreservesPiAPI(t *testing.T) {
	runtimeConfig := validRuntimeConfig()
	runtimeConfig.AgentRunner.PiAPI = "openai-responses"
	if got := EditableFromRuntime(runtimeConfig).AgentRunner.PiAPI; got != "openai-responses" {
		t.Fatalf("editable pi api = %q", got)
	}
	merged := mergeUpdate(runtimeConfig, UpdateRequest{Config: EditableRuntimeConfig{
		AgentRunner: EditableAgentRunnerConfig{
			ServiceURL: runtimeConfig.AgentRunner.ServiceURL, Provider: "pi-cli",
			ProviderBaseURL: runtimeConfig.AgentRunner.ProviderBaseURL, Model: runtimeConfig.AgentRunner.Model,
			Effort: runtimeConfig.AgentRunner.Effort, TaskTimeout: runtimeConfig.AgentRunner.TaskTimeout,
			PiAPI: "openai-completions",
		},
		Scheduler: runtimeConfig.Scheduler,
	}})
	if merged.AgentRunner.PiAPI != "openai-completions" {
		t.Fatalf("merged pi api = %q", merged.AgentRunner.PiAPI)
	}
}

func TestFromLegacyConfigCarriesPiAPI(t *testing.T) {
	legacy := config.Config{AgentRunner: config.AgentRunnerSettings{PiAPI: "openai-responses"}}
	runtimeConfig, err := FromLegacyConfig(legacy)
	if err != nil {
		t.Fatalf("from legacy config: %v", err)
	}
	if runtimeConfig.AgentRunner.PiAPI != "openai-responses" {
		t.Fatalf("legacy pi api = %q", runtimeConfig.AgentRunner.PiAPI)
	}
}
