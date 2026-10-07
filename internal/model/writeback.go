package model

import (
	"time"

	"gorm.io/gorm"
)

type WritebackStatus string

const (
	WritebackStatusPending   WritebackStatus = "pending"
	WritebackStatusRunning   WritebackStatus = "running"
	WritebackStatusSucceeded WritebackStatus = "succeeded"
	WritebackStatusFailed    WritebackStatus = "failed"
	WritebackStatusUnknown   WritebackStatus = "unknown"
)

type Writeback struct {
	ID                    uint            `gorm:"primaryKey"`
	WritebackID           string          `gorm:"uniqueIndex;not null;size:128"`
	TaskID                string          `gorm:"index;not null;size:128"`
	RunID                 string          `gorm:"index;not null;size:128"`
	DedupeKey             string          `gorm:"uniqueIndex;not null;size:255"`
	Kind                  string          `gorm:"index;not null;size:64"`
	ProjectID             string          `gorm:"index;not null;size:128"`
	TargetType            string          `gorm:"index;size:64"`
	TargetIID             int             `gorm:"index"`
	Status                WritebackStatus `gorm:"index;not null;size:32"`
	AttemptCount          int             `gorm:"not null;default:0"`
	OperationMarker       string          `gorm:"type:text"`
	ReconcileAttemptCount int             `gorm:"not null;default:0"`
	LastReconciledAt      *time.Time      `gorm:"index"`
	RequestJSON           string          `gorm:"type:text"`
	ResponseJSON          string          `gorm:"type:text"`
	ExternalID            string          `gorm:"index;size:255"`
	ExternalURL           string          `gorm:"type:text"`
	ErrorMessage          string          `gorm:"type:text"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (w *Writeback) BeforeCreate(tx *gorm.DB) error {
	if w.WritebackID == "" {
		w.WritebackID = NewID("writeback")
	}
	if w.Status == "" {
		w.Status = WritebackStatusPending
	}
	return nil
}
