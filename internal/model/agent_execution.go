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
)

type AgentExecution struct {
	ID                     uint                 `gorm:"primaryKey"`
	RunID                  string               `gorm:"uniqueIndex;not null"`
	ExecutionID            string               `gorm:"uniqueIndex;not null"`
	Provider               string               `gorm:"index;not null"`
	ProviderConfigRevision int64                `gorm:"not null;default:0"`
	ProviderModel          string               `gorm:"index"`
	ProviderEffort         string               `gorm:"index"`
	Status                 AgentExecutionStatus `gorm:"index;not null"`
	RequestHash            string               `gorm:"index;not null"`
	LastEventSeq           int64                `gorm:"not null;default:0"`
	LastObservedAt         *time.Time           `gorm:"index"`
	ResultHash             string               `gorm:"index"`
	ErrorCode              string               `gorm:"index"`
	ErrorMessage           string               `gorm:"type:text"`
	StartedAt              *time.Time           `gorm:"index"`
	FinishedAt             *time.Time           `gorm:"index"`
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type AgentExecutionEvent struct {
	ID          uint      `gorm:"primaryKey"`
	ExecutionID string    `gorm:"uniqueIndex:idx_agent_execution_event_seq;index;not null"`
	RunID       string    `gorm:"index;not null"`
	Provider    string    `gorm:"index;not null"`
	Seq         int64     `gorm:"uniqueIndex:idx_agent_execution_event_seq;not null"`
	Type        string    `gorm:"index;not null"`
	OccurredAt  time.Time `gorm:"index;not null"`
	PayloadJSON string    `gorm:"type:text"`
	CreatedAt   time.Time
}
