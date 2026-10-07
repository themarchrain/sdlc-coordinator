package model

import (
	"time"

	"gorm.io/gorm"
)

type InboundEventStatus string

const (
	InboundEventStatusReceived    InboundEventStatus = "received"
	InboundEventStatusNormalized  InboundEventStatus = "normalized"
	InboundEventStatusDeferred    InboundEventStatus = "deferred"
	InboundEventStatusIgnored     InboundEventStatus = "ignored"
	InboundEventStatusTaskCreated InboundEventStatus = "task_created"
	InboundEventStatusFailed      InboundEventStatus = "failed"
)

type InboundEvent struct {
	ID             uint               `gorm:"primaryKey"`
	EventID        string             `gorm:"uniqueIndex;not null;size:128"`
	DedupeKey      string             `gorm:"uniqueIndex;not null;size:255"`
	Source         string             `gorm:"index;not null;size:32"`
	EventType      string             `gorm:"index;not null;size:64"`
	EventAction    string             `gorm:"index;size:64"`
	ExternalID     string             `gorm:"index;size:128"`
	RepositoryID   *string            `gorm:"column:repository_id;index;size:128"`
	Status         InboundEventStatus `gorm:"index;not null;size:32"`
	PayloadJSON    string             `gorm:"type:text"`
	NormalizedJSON string             `gorm:"type:text"`
	ErrorMessage   string             `gorm:"type:text"`
	ReceivedAt     time.Time          `gorm:"index"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (e *InboundEvent) BeforeCreate(tx *gorm.DB) error {
	if e.EventID == "" {
		e.EventID = NewID("event")
	}
	if e.Status == "" {
		e.Status = InboundEventStatusReceived
	}
	if e.ReceivedAt.IsZero() {
		e.ReceivedAt = time.Now()
	}
	return nil
}
