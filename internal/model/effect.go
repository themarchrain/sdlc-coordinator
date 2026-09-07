package model

import (
	"time"

	"gorm.io/gorm"
)

type EffectStatus string

const (
	EffectStatusPending   EffectStatus = "pending"
	EffectStatusRunning   EffectStatus = "running"
	EffectStatusSucceeded EffectStatus = "succeeded"
	EffectStatusFailed    EffectStatus = "failed"
	EffectStatusSkipped   EffectStatus = "skipped"
)

type Effect struct {
	ID           uint         `gorm:"primaryKey"`
	EffectID     string       `gorm:"uniqueIndex;not null"`
	TaskID       string       `gorm:"index;not null"`
	RunID        string       `gorm:"index;not null"`
	DedupeKey    string       `gorm:"uniqueIndex;not null"`
	Kind         string       `gorm:"index;not null"`
	Target       string       `gorm:"index;not null"`
	Critical     bool         `gorm:"not null;default:false"`
	Status       EffectStatus `gorm:"index;not null"`
	AttemptCount int          `gorm:"not null;default:0"`
	RequestJSON  string       `gorm:"type:text"`
	ResponseJSON string       `gorm:"type:text"`
	ErrorMessage string       `gorm:"type:text"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (e *Effect) BeforeCreate(tx *gorm.DB) error {
	if e.EffectID == "" {
		e.EffectID = NewID("effect")
	}
	if e.Status == "" {
		e.Status = EffectStatusPending
	}
	return nil
}
