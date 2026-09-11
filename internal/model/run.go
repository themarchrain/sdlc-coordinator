package model

import (
	"time"

	"gorm.io/gorm"
)

type RunStatus string

const (
	RunStatusPending    RunStatus = "pending"
	RunStatusRunning    RunStatus = "running"
	RunStatusFinalizing RunStatus = "finalizing"
	RunStatusSucceeded  RunStatus = "succeeded"
	RunStatusFailed     RunStatus = "failed"
	RunStatusBlocked    RunStatus = "blocked"
)

type Run struct {
	ID             uint      `gorm:"primaryKey"`
	RunID          string    `gorm:"uniqueIndex;not null"`
	TaskID         string    `gorm:"index;not null"`
	WorkerRole     string    `gorm:"index;not null"`
	Status         RunStatus `gorm:"index;not null"`
	ExternalSystem string    `gorm:"index"`
	ExternalID     string    `gorm:"index"`
	ExternalURL    string    `gorm:"type:text"`
	InputJSON      string    `gorm:"type:text"`
	OutputJSON     string    `gorm:"type:text"`
	ErrorMessage   string    `gorm:"type:text"`
	// Provider 钉住本 run 派发时实际使用的执行 CLI：组织段配置热切换后，
	// 恢复/投影重建路径按钉住值重放请求，请求哈希与远端既有执行保持一致。
	// 空串表示派发于钉住机制之前（或系统默认），恢复路径回落现取值。
	Provider          string     `gorm:"index"`
	StartedAt         *time.Time `gorm:"index"`
	CompletedAt       *time.Time `gorm:"index"`
	ArtifactsPurgedAt *time.Time `gorm:"index"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (r *Run) BeforeCreate(tx *gorm.DB) error {
	if r.RunID == "" {
		r.RunID = NewID("run")
	}
	if r.Status == "" {
		r.Status = RunStatusPending
	}
	return nil
}
