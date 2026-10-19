package model

import "time"

type AgentExecutionStatus string

const (
	AgentExecutionStatusAccepted    AgentExecutionStatus = "accepted"
	AgentExecutionStatusStarting    AgentExecutionStatus = "starting"
	AgentExecutionStatusRunning     AgentExecutionStatus = "running"
	AgentExecutionStatusSucceeded   AgentExecutionStatus = "succeeded"
	AgentExecutionStatusFailed      AgentExecutionStatus = "failed"
	AgentExecutionStatusCancelled   AgentExecutionStatus = "cancelled"
	AgentExecutionStatusInterrupted AgentExecutionStatus = "interrupted"
	AgentExecutionStatusDispatching AgentExecutionStatus = "dispatching"
)

type AgentExecution struct {
	ID                     uint   `gorm:"primaryKey"`
	RunID                  string `gorm:"uniqueIndex;not null;size:128"`
	ExecutionID            string `gorm:"uniqueIndex;not null;size:128"`
	Provider               string `gorm:"index;not null;size:64"`
	ProviderConfigRevision int64  `gorm:"not null;default:0"`
	ProviderModel          string `gorm:"index;size:64"`
	ProviderEffort         string `gorm:"index;size:32"`
	// RunnerEndpoint 记录执行所落的 runner 端点（租户池亲和：观测与清理
	// 回到同一端点）；空值 = 单端点默认形态。对外 API 不暴露。
	RunnerEndpoint string               `gorm:"column:runner_endpoint" json:"-"`
	Status         AgentExecutionStatus `gorm:"index;not null;size:32"`
	RequestHash    string               `gorm:"index;not null;size:64"`
	LastEventSeq   int64                `gorm:"not null;default:0"`
	LastObservedAt *time.Time           `gorm:"index"`
	ResultHash     string               `gorm:"index;size:64"`
	ErrorCode      string               `gorm:"index;size:64"`
	ErrorMessage   string               `gorm:"type:text"`
	StartedAt      *time.Time           `gorm:"index"`
	FinishedAt     *time.Time           `gorm:"index"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type AgentExecutionEvent struct {
	ID          uint      `gorm:"primaryKey"`
	ExecutionID string    `gorm:"uniqueIndex:idx_agent_execution_event_seq;index;not null;size:128"`
	RunID       string    `gorm:"index;not null;size:128"`
	Provider    string    `gorm:"index;not null;size:64"`
	Seq         int64     `gorm:"uniqueIndex:idx_agent_execution_event_seq;not null"`
	Type        string    `gorm:"index;not null;size:64"`
	OccurredAt  time.Time `gorm:"index;not null"`
	PayloadJSON string    `gorm:"type:text"`
	CreatedAt   time.Time
}
