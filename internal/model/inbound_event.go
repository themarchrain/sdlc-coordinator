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
	EventID        string             `gorm:"uniqueIndex;not null"`
	DedupeKey      string             `gorm:"uniqueIndex;not null"`
	Source         string             `gorm:"index;not null"`
	EventType      string             `gorm:"index;not null"`
	EventAction    string             `gorm:"index"`
	ExternalID     string             `gorm:"index"`
	RepositoryID   *string            `gorm:"column:repository_id;index"`
	Status         InboundEventStatus `gorm:"index;not null"`
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
