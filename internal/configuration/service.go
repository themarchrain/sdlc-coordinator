package configuration

import (
	"context"
	"encoding/json"
	"errors"
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
	Provider        string `json:"provider"`
	PiAPI           string `json:"pi_api"`
	ProviderBaseURL string `json:"provider_base_url"`
	Model           string `json:"model"`
	Effort          string `json:"effort"`
	TaskTimeout     string `json:"task_timeout"`
}

type EditableCodeHostConfig struct {
	Provider             string   `json:"provider"`
	BaseURL              string   `json:"base_url"`
	ReworkAllowedUserIDs []string `json:"rework_allowed_user_ids"`
	AutoMergeEnabled     bool     `json:"auto_merge_enabled"`
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
	Revision uint64                `json:"revision"`
	Config   EditableRuntimeConfig `json:"config"`
	Secrets  SecretStatus          `json:"secrets"`
}

// RevisionView 是历史 revision 的脱敏视图：config/secrets 仅在 payload 可按
// 当前结构解码时返回；restorable 是按当前校验规则重演恢复路径后的预检结论，
// 不可恢复时 unrestorable_reason 给出可读原因（恢复入口据此 fail closed）。
type RevisionView struct {
	Revision           uint64                 `json:"revision"`
	Status             SnapshotStatus         `json:"status"`
	Source             SnapshotSource         `json:"source"`
	AppliedAt          *time.Time             `json:"applied_at,omitempty"`
	CreatedAt          time.Time              `json:"created_at"`
	Restorable         bool                   `json:"restorable"`
	UnrestorableReason string                 `json:"unrestorable_reason,omitempty"`
	Config             *EditableRuntimeConfig `json:"config,omitempty"`
	Secrets            *SecretStatus          `json:"secrets,omitempty"`
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
	Config           EditableRuntimeConfig `json:"config"`
	Secrets          SecretChanges         `json:"secrets"`
	ExpectedRevision *uint64               `json:"expected_revision"`
}

var ErrExpectedRevisionRequired = errors.New("expected runtime configuration revision is required")

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
	view := MaskRuntimeConfig(service.current)
	view.Revision = service.statusLocked().Revision
	return view
}

func (service *Service) Runtime() RuntimeConfig {
	if service == nil {
		return RuntimeConfig{}
	}
	service.mu.RLock()
	defer service.mu.RUnlock()
	return service.current
}

func (service *Service) HasNewerRevision(ctx context.Context) (bool, error) {
	status := service.Status()
	latest, err := service.repository.Latest(ctx)
	if errors.Is(err, ErrSnapshotNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return latest.Revision > status.Revision, nil
}

// Revisions 分页返回脱敏历史视图（revision 倒序）；page/pageSize 的合法化
// 由 HTTP 层负责，调用方须传 page>=1、pageSize>=1。
func (service *Service) Revisions(ctx context.Context, page, pageSize int) ([]RevisionView, int64, error) {
	records, total, err := service.repository.ListRecords(ctx, (page-1)*pageSize, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list runtime configuration revisions: %w", err)
	}
	views := make([]RevisionView, 0, len(records))
	for _, record := range records {
		views = append(views, revisionViewFromRecord(record))
	}
	return views, total, nil
}

func (service *Service) Revision(ctx context.Context, revision uint64) (*RevisionView, error) {
	record, err := service.repository.GetRecord(ctx, revision)
	if err != nil {
		return nil, err
	}
	view := revisionViewFromRecord(*record)
	return &view, nil
}

// revisionViewFromRecord 重演恢复路径的前半段做预检：payload 先按当前结构
// 解码（失败即 schema 过期），再 ApplyDefaults+Validate 按当前校验规则重验
// ——两者都拒绝恢复；视图本身始终脱敏，密钥只保留 configured 状态。
func revisionViewFromRecord(record SnapshotRecord) RevisionView {
	view := RevisionView{
		Revision:  record.Revision,
		Status:    SnapshotStatus(record.Status),
		Source:    SnapshotSource(record.Source),
		AppliedAt: record.AppliedAt,
		CreatedAt: record.CreatedAt,
	}
	var cfg RuntimeConfig
	if err := json.Unmarshal([]byte(record.PayloadJSON), &cfg); err != nil {
		view.UnrestorableReason = "历史快照 schema 过期，无法按当前结构解析"
		return view
	}
	cfg = ApplyDefaults(cfg)
	masked := MaskRuntimeConfig(cfg)
	view.Config = &masked.Config
	view.Secrets = &masked.Secrets
	if validationErrors := Validate(cfg); len(validationErrors) > 0 {
		view.UnrestorableReason = "按当前校验规则不合格：" + formatValidationErrors(validationErrors)
		return view
	}
	view.Restorable = true
	return view
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
	if request.ExpectedRevision == nil {
		return nil, nil, ErrExpectedRevisionRequired
	}
	// 合并基线必须取库内最新快照而非本进程缓存：负载均衡下 A 改字段 X 后
	// B 的提交可能仍带着 B 的旧缓存，按旧缓存整包发新 revision 会把 X
	// 恢复成旧值（多实例配置回退）。库内最新快照是唯一权威基线，CAS 期望
	// 版本与其绑定——保存窗口内的并发提交由 ErrRevisionConflict 拒绝。
	baseline := service.current
	expectedRevision := uint64(0)
	latest, err := service.repository.Latest(ctx)
	switch {
	case err == nil:
		baseline = latest.Config
		expectedRevision = latest.Revision
	case errors.Is(err, ErrSnapshotNotFound):
	default:
		return nil, nil, fmt.Errorf("load latest runtime configuration: %w", err)
	}
	if *request.ExpectedRevision != expectedRevision {
		return nil, nil, ErrRevisionConflict
	}
	candidate := mergeUpdate(baseline, request)
	validationErrors := Validate(candidate)
	if len(validationErrors) > 0 {
		return nil, validationErrors, nil
	}
	snapshot, err := service.repository.SaveGuarded(ctx, StatusRestartRequired, SourceUI, candidate, expectedRevision)
	if err != nil {
		return nil, nil, fmt.Errorf("save runtime configuration: %w", err)
	}
	service.current = candidate
	service.snapshot = snapshot
	service.mode = ModeRestartRequired
	service.configurationError = ""
	configuration := MaskRuntimeConfig(candidate)
	configuration.Revision = snapshot.Revision
	return &UpdateResult{
		StatusView:    service.statusLocked(),
		Configuration: configuration,
	}, nil, nil
}

func EditableFromRuntime(cfg RuntimeConfig) EditableRuntimeConfig {
	return EditableRuntimeConfig{
		AgentRunner: EditableAgentRunnerConfig{
			ServiceURL: cfg.AgentRunner.ServiceURL,
			Provider:   cfg.AgentRunner.Provider, PiAPI: cfg.AgentRunner.PiAPI,
			ProviderBaseURL: cfg.AgentRunner.ProviderBaseURL,
			Model:           cfg.AgentRunner.Model, Effort: cfg.AgentRunner.Effort, TaskTimeout: cfg.AgentRunner.TaskTimeout,
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
			ServiceURL: request.Config.AgentRunner.ServiceURL,
			Provider:   request.Config.AgentRunner.Provider, PiAPI: request.Config.AgentRunner.PiAPI,
			ProviderBaseURL: request.Config.AgentRunner.ProviderBaseURL,
			APIKey:          current.AgentRunner.APIKey, CustomHeadersJSON: current.AgentRunner.CustomHeadersJSON,
			Model: request.Config.AgentRunner.Model, Effort: request.Config.AgentRunner.Effort, TaskTimeout: request.Config.AgentRunner.TaskTimeout,
		},
		Scheduler: func() SchedulerConfig {
			scheduler := request.Config.Scheduler
			scheduler.CallbackBaseURL = keepTrimmed(scheduler.CallbackBaseURL, current.Scheduler.CallbackBaseURL)
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
