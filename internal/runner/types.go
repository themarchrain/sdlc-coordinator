package runner

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrNotFound     = errors.New("execution not found")
	ErrConflict     = errors.New("execution request conflict")
	ErrNotReady     = errors.New("execution result is not ready")
	ErrNotPurgeable = errors.New("execution artifacts are not purgeable")
)

type Status string

const (
	StatusAccepted    Status = "accepted"
	StatusStarting    Status = "starting"
	StatusRunning     Status = "running"
	StatusSucceeded   Status = "succeeded"
	StatusFailed      Status = "failed"
	StatusCancelled   Status = "cancelled"
	StatusInterrupted Status = "interrupted"
)

type ProviderConfig struct {
	Name     string            `json:"name"`
	Revision int64             `json:"revision"`
	BaseURL  string            `json:"base_url,omitempty"`
	APIKey   string            `json:"api_key,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Model    string            `json:"model,omitempty"`
	Effort   string            `json:"effort,omitempty"`
}

type CreateExecutionRequest struct {
	ExecutionID    string         `json:"execution_id"`
	TaskID         string         `json:"task_id"`
	Role           string         `json:"role"`
	WorkspacePath  string         `json:"workspace_path"`
	PromptFile     string         `json:"prompt_file"`
	SchemaFile     string         `json:"schema_file"`
	TimeoutSeconds int64          `json:"timeout_seconds,omitempty"`
	Provider       ProviderConfig `json:"provider"`
	// ResultSubmission enables the MCP result channel for this run: the
	// coordinator /mcp endpoint URL and the per-run bearer token. It is
	// deliberately excluded from RequestHash (per-run, retry must stay
	// idempotent) and must never be logged.
	ResultSubmission *ResultSubmissionChannel `json:"result_submission,omitempty"`
}

// ResultSubmissionChannel carries the per-run MCP endpoint credentials from
// the coordinator through the runner into the agent CLI process.
type ResultSubmissionChannel struct {
	EndpointURL string `json:"endpoint_url"`
	BearerToken string `json:"bearer_token"`
}

type Execution struct {
	ExecutionID  string     `json:"execution_id"`
	TaskID       string     `json:"task_id"`
	Role         string     `json:"role"`
	Provider     string     `json:"provider"`
	Status       Status     `json:"status"`
	RequestHash  string     `json:"request_hash"`
	ResultHash   string     `json:"result_hash,omitempty"`
	ErrorCode    string     `json:"error_code,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

type Event struct {
	ExecutionID string         `json:"execution_id"`
	Seq         int64          `json:"seq"`
	Type        string         `json:"type"`
	At          time.Time      `json:"at"`
	Payload     map[string]any `json:"payload,omitempty"`
}

type Result struct {
	ExecutionID      string          `json:"execution_id"`
	ResultHash       string          `json:"result_hash"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	CompletedAt      time.Time       `json:"completed_at"`
}

type ProviderRequest struct {
	ExecutionID      string
	TaskID           string
	Role             string
	WorkspacePath    string
	PromptFile       string
	SchemaFile       string
	Provider         ProviderConfig
	ResultSubmission *ResultSubmissionChannel
}

type EmitFunc func(eventType string, payload map[string]any)

type Provider interface {
	Execute(context.Context, ProviderRequest, EmitFunc) (json.RawMessage, error)
}
