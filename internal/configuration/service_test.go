package configuration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestServiceUpdateRetainsReplacesAndExplicitlyClearsSecrets(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	initial := validRuntimeConfig()
	initial.AgentRunner.APIKey = "old-runner-key"
	initial.AgentRunner.CustomHeadersJSON = `{"X-Old":"secret"}`
	saved, err := repository.Save(context.Background(), StatusReady, SourceUI, initial)
	if err != nil {
		t.Fatalf("seed runtime configuration: %v", err)
	}
	service := NewService(repository, Resolution{Mode: ModeReady, Prefill: initial, Snapshot: saved})
	input := EditableFromRuntime(initial)
	input.AgentRunner.Model = "updated-model"

	result, validationErrors, err := service.Update(context.Background(), UpdateRequest{
		Config: input, ExpectedRevision: uint64Pointer(1),
		Secrets: SecretChanges{
			AgentRunnerAPIKey:                 stringPointer("new-runner-key"),
			ClearAgentRunnerCustomHeadersJSON: true,
		},
	})
	if err != nil || len(validationErrors) != 0 {
		t.Fatalf("update runtime configuration: result=%#v validation=%#v error=%v", result, validationErrors, err)
	}

	latest, err := repository.Latest(context.Background())
	if err != nil {
		t.Fatalf("load latest snapshot: %v", err)
	}
	if latest.Config.AgentRunner.APIKey != "new-runner-key" || latest.Config.AgentRunner.CustomHeadersJSON != "" {
		t.Fatalf("agent runner secret changes were not applied: %#v", latest.Config.AgentRunner)
	}
	if latest.Config.AgentRunner.Model != "updated-model" || latest.Status != StatusRestartRequired {
		t.Fatalf("unexpected latest configuration: %#v", latest)
	}
	if result.Mode != ModeRestartRequired || result.Revision != latest.Revision || result.Configuration.Revision != latest.Revision {
		t.Fatalf("unexpected update result: %#v", result)
	}
}

// scheduler.callback_base_url / workspace_root 是必填运行参数；UI 或旧客户端
// 可能整包提交 scheduler 而漏带这两个字段，空值时必须保留当前值而不是校验打回。
func TestServiceUpdateKeepsSchedulerFieldsWhenSubmissionOmitsThem(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	initial := validRuntimeConfig()
	saved, err := repository.Save(context.Background(), StatusReady, SourceUI, initial)
	if err != nil {
		t.Fatalf("seed runtime configuration: %v", err)
	}
	service := NewService(repository, Resolution{Mode: ModeReady, Prefill: initial, Snapshot: saved})
	input := EditableFromRuntime(initial)
	input.Scheduler.CallbackBaseURL = ""
	input.Scheduler.WorkspaceRoot = ""

	result, validationErrors, err := service.Update(context.Background(), UpdateRequest{Config: input, ExpectedRevision: uint64Pointer(1)})
	if err != nil || len(validationErrors) != 0 {
		t.Fatalf("update runtime configuration: result=%#v validation=%#v error=%v", result, validationErrors, err)
	}
	if result.Configuration.Config.Scheduler.CallbackBaseURL != "https://coordinator.example.com" ||
		result.Configuration.Config.Scheduler.WorkspaceRoot != "/app/workspaces" {
		t.Fatalf("omitted scheduler fields were not retained: %#v", result.Configuration.Config.Scheduler)
	}
}

func TestServiceUpdateDropsLegacyTranslatorFields(t *testing.T) {
	service := newReadyConfigurationService(t)
	input := service.Get().Config

	result, validationErrors, err := service.Update(context.Background(), UpdateRequest{Config: input, ExpectedRevision: uint64Pointer(1)})
	if err != nil || len(validationErrors) != 0 {
		t.Fatalf("update without translator settings: result=%#v validation=%#v error=%v", result, validationErrors, err)
	}
	raw, err := json.Marshal(result.Configuration.Config)
	if err != nil {
		t.Fatalf("marshal persisted config: %v", err)
	}
	if strings.Contains(string(raw), "translator_url") || strings.Contains(string(raw), "normalize_to_chat") {
		t.Fatalf("legacy translator fields were persisted: %s", raw)
	}
}

func TestServiceViewNeverContainsSecretValues(t *testing.T) {
	service := newReadyConfigurationService(t)

	view := service.Get()

	if !view.Secrets.AgentRunnerAPIKeyConfigured || !view.Secrets.AgentRunnerCustomHeadersConfigured {
		t.Fatalf("configured flags are incomplete: %#v", view.Secrets)
	}
	if view.Config.AgentRunner.ServiceURL == "" || view.Config.AgentRunner.Model == "" {
		t.Fatalf("non-secret agent runner values missing: %#v", view.Config.AgentRunner)
	}
}

func TestServiceRejectsInvalidUpdateWithoutCreatingRevision(t *testing.T) {
	service := newReadyConfigurationService(t)
	input := service.Get().Config
	input.Scheduler.CallbackBaseURL = "not-a-url"

	result, validationErrors, err := service.Update(context.Background(), UpdateRequest{Config: input, ExpectedRevision: uint64Pointer(1)})

	if err != nil {
		t.Fatalf("invalid request should return field errors, got %v", err)
	}
	if result != nil || !validationErrors.Has("scheduler.callback_base_url") {
		t.Fatalf("unexpected invalid update result=%#v validation=%#v", result, validationErrors)
	}
	status := service.Status()
	if status.Revision != 1 || status.Mode != ModeReady {
		t.Fatalf("invalid update changed service state: %#v", status)
	}
}

func TestServiceValidationDoesNotPersistConfiguration(t *testing.T) {
	service := newReadyConfigurationService(t)
	input := service.Get().Config
	input.Scheduler.Interval = "60s"

	validationErrors := service.Validate(UpdateRequest{Config: input})

	if len(validationErrors) != 0 {
		t.Fatalf("expected valid configuration, got %#v", validationErrors)
	}
	if status := service.Status(); status.Revision != 1 || status.Mode != ModeReady {
		t.Fatalf("validation persisted changes: %#v", status)
	}
}

func newReadyConfigurationService(t *testing.T) *Service {
	t.Helper()
	repository := newConfigurationRepositoryTestDB(t)
	runtimeConfig := validRuntimeConfig()
	snapshot, err := repository.Save(context.Background(), StatusReady, SourceUI, runtimeConfig)
	if err != nil {
		t.Fatalf("seed runtime configuration: %v", err)
	}
	return NewService(repository, Resolution{Mode: ModeReady, Prefill: runtimeConfig, Snapshot: snapshot})
}

func stringPointer(value string) *string {
	return &value
}

func uint64Pointer(value uint64) *uint64 {
	return &value
}
