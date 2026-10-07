package model

import (
	"time"

	"gorm.io/gorm"
)

type ResultSubmissionStatus string

const (
	ResultSubmissionStatusProcessing ResultSubmissionStatus = "processing"
	ResultSubmissionStatusSucceeded  ResultSubmissionStatus = "succeeded"
	ResultSubmissionStatusFailed     ResultSubmissionStatus = "failed"
	ResultSubmissionStatusUnknown    ResultSubmissionStatus = "unknown"
)

type ResultSubmission struct {
	ID             uint   `gorm:"primaryKey"`
	SubmissionID   string `gorm:"uniqueIndex;not null;size:128"`
	TaskID         string `gorm:"index;not null;size:128"`
	RunID          string `gorm:"uniqueIndex;not null;size:128"`
	Source         string `gorm:"index;not null;size:32"`
	ToolName       string `gorm:"index;size:64"`
	Role           string `gorm:"index;not null;size:32"`
	PayloadHash    string `gorm:"index;not null;size:64"`
	RawPayloadJSON string `gorm:"type:text;not null"`
	CanonicalJSON  string `gorm:"type:text;not null"`
	// NormalizationJSON records the thin-normalizer repair trail; empty means
	// the payload passed through byte identical (status clean).
	NormalizationJSON string                 `gorm:"type:text"`
	Status            ResultSubmissionStatus `gorm:"index;not null;size:32"`
	AttemptCount      int                    `gorm:"not null;default:0"`
	LeaseUntil        *time.Time             `gorm:"index"`
	ErrorMessage      string                 `gorm:"type:text"`
	CompletedAt       *time.Time             `gorm:"index"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (s *ResultSubmission) BeforeCreate(tx *gorm.DB) error {
	if s.SubmissionID == "" {
		s.SubmissionID = NewID("submission")
	}
	if s.Status == "" {
		s.Status = ResultSubmissionStatusProcessing
	}
	return nil
}
