package configuration

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type EditableRuntimeConfig struct {
	AgentRunner EditableAgentRunnerConfig `json:"agent_runner"`
	Scheduler   SchedulerConfig           `json:"scheduler"`
}

type EditableAgentRunnerConfig struct {
	ServiceURL      string `json:"service_url"`
	WorkspaceRoot   string `json:"workspace_root"`
	Provider        string `json:"provider"`
	ProviderBaseURL string `json:"provider_base_url"`
	Model           string `json:"model"`
	Effort          string `json:"effort"`
	TaskTimeout     string `json:"task_timeout"`
}

type EditableCodeHostConfig struct {
	Provider             string   `json:"provider"`
	BaseURL              string   `json:"base_url"`
	ReworkAllowedUserIDs []string `json:"rework_allowed_user_ids"`
}

type EditableIMConfig struct {
	Provider         string             `json:"provider"`
	Telegram         EditableIMTelegram `json:"telegram"`
	Feishu           EditableIMFeishu   `json:"feishu"`
	ReviewerMentions []string           `json:"reviewer_mentions"`
}

type EditableIMTelegram struct {
	APIBase       string            `json:"api_base,omitempty"`
	DefaultChatID string            `json:"default_chat_id,omitempty"`
	MentionMap    map[string]string `json:"mention_map,omitempty"`
}

type EditableIMFeishu struct {
	WebhookURL string            `json:"webhook_url,omitempty"`
	MentionMap map[string]string `json:"mention_map,omitempty"`
}

type SecretStatus struct {
	AgentRunnerAPIKeyConfigured        bool `json:"agent_runner_api_key_configured"`
	AgentRunnerCustomHeadersConfigured bool `json:"agent_runner_custom_headers_configured"`
}

type ConfigurationView struct {
	Config  EditableRuntimeConfig `json:"config"`
	Secrets SecretStatus          `json:"secrets"`
}

type StatusView struct {
	Mode               Mode           `json:"mode"`
	Revision           uint64         `json:"revision"`
	Source             SnapshotSource `json:"source,omitempty"`
	AppliedAt          *time.Time     `json:"applied_at,omitempty"`
	ConfigurationError string         `json:"configuration_error,omitempty"`
}

type SecretChanges struct {
	AgentRunnerAPIKey            *string `json:"agent_runner_api_key,omitempty"`
	AgentRunnerCustomHeadersJSON *string `json:"agent_runner_custom_headers_json,omitempty"`

	ClearAgentRunnerAPIKey            bool `json:"clear_agent_runner_api_key,omitempty"`
	ClearAgentRunnerCustomHeadersJSON bool `json:"clear_agent_runner_custom_headers_json,omitempty"`
}

type UpdateRequest struct {
	Config  EditableRuntimeConfig `json:"config"`
	Secrets SecretChanges         `json:"secrets"`
}

type UpdateResult struct {
	StatusView
	Configuration ConfigurationView `json:"configuration"`
}

type Service struct {
	mu                 sync.RWMutex
	repository         *Repository
	mode               Mode
	current            RuntimeConfig
	snapshot           *Snapshot
	configurationError string
}

func NewService(repository *Repository, resolution Resolution) *Service {
	return &Service{
		repository:         repository,
		mode:               resolution.Mode,
		current:            resolution.Prefill,
		snapshot:           resolution.Snapshot,
		configurationError: resolution.ConfigurationError,
	}
}

func (service *Service) Status() StatusView {
	service.mu.RLock()
	defer service.mu.RUnlock()
	return service.statusLocked()
}

func (service *Service) Get() ConfigurationView {
	service.mu.RLock()
	defer service.mu.RUnlock()
	return MaskRuntimeConfig(service.current)
}

func (service *Service) Runtime() RuntimeConfig {
	if service == nil {
		return RuntimeConfig{}
	}
	service.mu.RLock()
	defer service.mu.RUnlock()
	return service.current
}

func (service *Service) Validate(request UpdateRequest) ValidationErrors {
	service.mu.RLock()
	baseline := service.current
	service.mu.RUnlock()
	candidate := mergeUpdate(baseline, request)
	return Validate(candidate)
}

func (service *Service) Update(ctx context.Context, request UpdateRequest) (*UpdateResult, ValidationErrors, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	candidate := mergeUpdate(service.current, request)
	validationErrors := Validate(candidate)
	if len(validationErrors) > 0 {
		return nil, validationErrors, nil
	}
	snapshot, err := service.repository.Save(ctx, StatusRestartRequired, SourceUI, candidate)
	if err != nil {
		return nil, nil, fmt.Errorf("save runtime configuration: %w", err)
	}
	service.current = candidate
	service.snapshot = snapshot
	service.mode = ModeRestartRequired
	service.configurationError = ""
	return &UpdateResult{
		StatusView:    service.statusLocked(),
		Configuration: MaskRuntimeConfig(candidate),
	}, nil, nil
}

func EditableFromRuntime(cfg RuntimeConfig) EditableRuntimeConfig {
	return EditableRuntimeConfig{
		AgentRunner: EditableAgentRunnerConfig{
			ServiceURL: cfg.AgentRunner.ServiceURL, WorkspaceRoot: cfg.AgentRunner.WorkspaceRoot,
			Provider: cfg.AgentRunner.Provider, ProviderBaseURL: cfg.AgentRunner.ProviderBaseURL,
			Model: cfg.AgentRunner.Model, Effort: cfg.AgentRunner.Effort, TaskTimeout: cfg.AgentRunner.TaskTimeout,
		},
		Scheduler: cfg.Scheduler,
	}
}

func MaskRuntimeConfig(cfg RuntimeConfig) ConfigurationView {
	return ConfigurationView{
		Config: EditableFromRuntime(cfg),
		Secrets: SecretStatus{
			AgentRunnerAPIKeyConfigured:        strings.TrimSpace(cfg.AgentRunner.APIKey) != "",
			AgentRunnerCustomHeadersConfigured: strings.TrimSpace(cfg.AgentRunner.CustomHeadersJSON) != "",
		},
	}
}

func (service *Service) statusLocked() StatusView {
	status := StatusView{Mode: service.mode, ConfigurationError: service.configurationError}
	if service.snapshot != nil {
		status.Revision = service.snapshot.Revision
		status.Source = service.snapshot.Source
		status.AppliedAt = service.snapshot.AppliedAt
	}
	return status
}

func mergeUpdate(current RuntimeConfig, request UpdateRequest) RuntimeConfig {
	result := RuntimeConfig{
		AgentRunner: AgentRunnerConfig{
			ServiceURL: request.Config.AgentRunner.ServiceURL, WorkspaceRoot: request.Config.AgentRunner.WorkspaceRoot,
			Provider: request.Config.AgentRunner.Provider, ProviderBaseURL: request.Config.AgentRunner.ProviderBaseURL,
			APIKey: current.AgentRunner.APIKey, CustomHeadersJSON: current.AgentRunner.CustomHeadersJSON,
			Model: request.Config.AgentRunner.Model, Effort: request.Config.AgentRunner.Effort, TaskTimeout: request.Config.AgentRunner.TaskTimeout,
		},
		Scheduler: func() SchedulerConfig {
			// 这两项是必填运行参数；UI 或旧客户端可能整包提交 scheduler 而漏带它们，
			// 空值时保留当前值，避免合法配置被一次不完整提交打回。
			scheduler := request.Config.Scheduler
			scheduler.CallbackBaseURL = keepTrimmed(scheduler.CallbackBaseURL, current.Scheduler.CallbackBaseURL)
			scheduler.WorkspaceRoot = keepTrimmed(scheduler.WorkspaceRoot, current.Scheduler.WorkspaceRoot)
			return scheduler
		}(),
	}
	applySecretChange(&result.AgentRunner.APIKey, request.Secrets.AgentRunnerAPIKey, request.Secrets.ClearAgentRunnerAPIKey)
	applySecretChange(&result.AgentRunner.CustomHeadersJSON, request.Secrets.AgentRunnerCustomHeadersJSON, request.Secrets.ClearAgentRunnerCustomHeadersJSON)
	return ApplyDefaults(result)
}

func keepTrimmed(next, current string) string {
	if strings.TrimSpace(next) == "" {
		return current
	}
	return strings.TrimSpace(next)
}

func applySecretChange(target *string, replacement *string, clear bool) {
	if clear {
		*target = ""
		return
	}
	if replacement != nil && strings.TrimSpace(*replacement) != "" {
		*target = *replacement
	}
}
