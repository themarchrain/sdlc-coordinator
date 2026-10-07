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
	RunID          string    `gorm:"uniqueIndex;not null;size:128"`
	TaskID         string    `gorm:"index;not null;size:128"`
	WorkerRole     string    `gorm:"index;not null;size:32"`
	Status         RunStatus `gorm:"index;not null;size:32"`
	ExternalSystem string    `gorm:"index;size:64"`
	ExternalID     string    `gorm:"index;size:128"`
	ExternalURL    string    `gorm:"type:text"`
	InputJSON      string    `gorm:"type:text"`
	OutputJSON     string    `gorm:"type:text"`
	ErrorMessage   string    `gorm:"type:text"`
	// Provider 钉住本 run 派发时实际使用的执行 CLI：组织段配置热切换后，
	// 恢复/投影重建路径按钉住值重放请求，请求哈希与远端既有执行保持一致。
	// 空串表示派发于钉住机制之前（或系统默认），恢复路径回落现取值。
	Provider string `gorm:"index;size:64"`
	// Effort 钉住本 run 派发时实际生效的推理强度，语义与 Provider 一致：
	// 请求哈希覆盖推理强度，热切换后按钉住值重放才能与远端既有执行对上；
	// 空串表示派发于钉住机制之前，恢复路径回落现取值。
	Effort string `gorm:"index;size:32"`
	// TaskTimeoutSeconds 钉住本 run 派发时实际生效的单任务超时（秒），语义
	// 与 Provider/Effort 一致：请求哈希覆盖超时秒数，热切换后按钉住值重放
	// 才能与远端既有执行对上；0 表示派发于钉住机制之前，恢复路径回落现取值。
	TaskTimeoutSeconds int64      `gorm:"index"`
	StartedAt          *time.Time `gorm:"index"`
	CompletedAt        *time.Time `gorm:"index"`
	ArtifactsPurgedAt  *time.Time `gorm:"index"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
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
