package connectors

import (
	"context"
	"time"
)

type Manifest struct {
	ContractVersion string   `json:"contract_version"`
	Kind            string   `json:"kind"`
	Name            string   `json:"name"`
	Version         string   `json:"version"`
	Capabilities    []string `json:"capabilities"`
}

type Health struct {
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type Run struct {
	RunID       string         `json:"run_id"`
	Trigger     string         `json:"trigger"`
	Status      string         `json:"status"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Discovered  int            `json:"discovered"`
	Created     int            `json:"created"`
	Updated     int            `json:"updated"`
	Skipped     int            `json:"skipped"`
	Failed      int            `json:"failed"`
	Error       string         `json:"error,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type Record struct {
	RecordID     string         `json:"record_id"`
	SourceType   string         `json:"source_type"`
	SourceID     string         `json:"source_id"`
	SourceTitle  string         `json:"source_title,omitempty"`
	TargetType   string         `json:"target_type,omitempty"`
	TargetID     string         `json:"target_id,omitempty"`
	TargetURL    string         `json:"target_url,omitempty"`
	Status       string         `json:"status"`
	LastSyncedAt *time.Time     `json:"last_synced_at,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type Metrics struct {
	GeneratedAt *time.Time         `json:"generated_at,omitempty"`
	Values      map[string]float64 `json:"values"`
	Metadata    map[string]any     `json:"metadata,omitempty"`
}

// Settings 为连接器插件自定义的不透明配置。coordinator 只做 JSON 透传,
// 字段校验与密钥脱敏由插件自身负责(远端回包应已脱敏)。
type Configuration struct {
	Mode            string         `json:"mode"`
	Version         int64          `json:"version"`
	Source          string         `json:"source,omitempty"`
	RestartRequired bool           `json:"restart_required"`
	Settings        map[string]any `json:"settings"`
	SecretFields    []string       `json:"secret_fields,omitempty"`
}

type SaveConfigurationRequest struct {
	Settings        map[string]any `json:"settings"`
	PreserveSecrets []string       `json:"preserve_secrets,omitempty"`
}

type RemotePage[T any] struct {
	Items    []T   `json:"items"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
}

// SaveRepositoryRequest 的 Settings 为插件自定义的每仓库配置投影,由
// coordinator 组装时透传,不做字段级理解。
type SaveRepositoryRequest struct {
	RepositoryID   string         `json:"repository_id"`
	ConfigRevision int64          `json:"config_revision"`
	Enabled        bool           `json:"enabled"`
	Settings       map[string]any `json:"settings"`
}

type RemoteRepository struct {
	RepositoryID      string         `json:"repository_id"`
	ConfigRevision    int64          `json:"config_revision"`
	Enabled           bool           `json:"enabled"`
	RestartRequired   bool           `json:"restart_required,omitempty"`
	Settings          map[string]any `json:"settings,omitempty"`
}

type RemoteClient interface {
	Manifest(ctx context.Context, baseURL string) (Manifest, error)
	Health(ctx context.Context, baseURL string) (Health, error)
	TriggerRun(ctx context.Context, baseURL, requestID, repositoryID string) (Run, error)
	ListRuns(ctx context.Context, baseURL string, page PageRequest) (RemotePage[Run], error)
	GetRun(ctx context.Context, baseURL, runID string) (Run, error)
	ListRecords(ctx context.Context, baseURL string, page PageRequest) (RemotePage[Record], error)
	GetRecord(ctx context.Context, baseURL, recordID string) (Record, error)
	Metrics(ctx context.Context, baseURL, repositoryID string) (Metrics, error)
	Configuration(ctx context.Context, baseURL string) (Configuration, error)
	SaveConfiguration(ctx context.Context, baseURL string, request SaveConfigurationRequest) (Configuration, error)
	PutRepository(ctx context.Context, baseURL, repositoryID string, request SaveRepositoryRequest) (RemoteRepository, error)
	GetRepository(ctx context.Context, baseURL, repositoryID string) (RemoteRepository, error)
	DeleteRepository(ctx context.Context, baseURL, repositoryID string) error
}
