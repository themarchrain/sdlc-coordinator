package model

import (
	"time"

	"gorm.io/gorm"
)

type TaskRecovery struct {
	ID                  uint      `gorm:"primaryKey" json:"-"`
	RecoveryID          string    `gorm:"uniqueIndex;not null;size:128" json:"recovery_id"`
	TaskID              string    `gorm:"uniqueIndex:idx_task_recovery_request;uniqueIndex:idx_task_recovery_generation;not null;size:128" json:"task_id"`
	RequestID           string    `gorm:"uniqueIndex:idx_task_recovery_request;not null;size:128" json:"request_id"`
	RequestHash         string    `gorm:"not null;size:64" json:"-"`
	SourceGeneration    uint64    `gorm:"uniqueIndex:idx_task_recovery_generation;not null" json:"source_generation"`
	TargetGeneration    uint64    `gorm:"not null" json:"target_generation"`
	BlockRevision       string    `gorm:"not null;size:64" json:"block_revision"`
	SourceRunID         string    `gorm:"size:128" json:"source_run_id,omitempty"`
	RequestedBy         string    `gorm:"not null;size:128" json:"requested_by"`
	RequestedAt         time.Time `gorm:"not null" json:"requested_at"`
	Reason              string    `gorm:"type:text" json:"reason,omitempty"`
	RiskAcknowledged    bool      `gorm:"not null;default:false" json:"risk_acknowledged"`
	Classification      string    `gorm:"not null;size:32" json:"classification"`
	FailureSnapshotJSON string    `gorm:"type:text" json:"-"`
	AttemptCountBefore  int       `gorm:"not null" json:"attempt_count_before"`
	CycleMaxAttempts    int       `gorm:"not null" json:"cycle_max_attempts"`
	CreatedAt           time.Time `json:"created_at"`
}

func (r *TaskRecovery) BeforeCreate(tx *gorm.DB) error {
	if r.RecoveryID == "" {
		r.RecoveryID = NewID("recovery")
	}
	if r.RequestedAt.IsZero() {
		r.RequestedAt = time.Now().UTC()
	}
	return nil
}
