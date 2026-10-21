package model

import "time"

type StopScope string

const (
	StopScopeTask             StopScope = "task"
	StopScopeExecutionCleanup StopScope = "execution_cleanup"
)

type StopDeliveryStatus string

const (
	StopDeliveryPending     StopDeliveryStatus = "pending"
	StopDeliveryDelivering  StopDeliveryStatus = "delivering"
	StopDeliveryWaitingExit StopDeliveryStatus = "waiting_exit"
	StopDeliveryConfirmed   StopDeliveryStatus = "confirmed"
)

type StopRequest struct {
	ID             uint               `gorm:"primaryKey"`
	StopID         string             `gorm:"uniqueIndex;not null;size:128"`
	DedupeKey      string             `gorm:"uniqueIndex;not null;size:255"`
	Scope          StopScope          `gorm:"not null;size:32"`
	TaskID         string             `gorm:"index;not null;size:128"`
	RunID          string             `gorm:"index;size:128"`
	ExecutionID    string             `gorm:"index;size:128"`
	RunnerEndpoint string             `gorm:"size:512" json:"-"`
	RequestedBy    string             `gorm:"not null;size:128"`
	RequestedAt    time.Time          `gorm:"not null"`
	ReasonCode     string             `gorm:"not null;size:32"`
	Reason         string             `gorm:"type:text"`
	DeliveryStatus StopDeliveryStatus `gorm:"index:idx_stop_delivery_due,priority:1;not null;size:32"`
	AttemptCount   int                `gorm:"not null;default:0"`
	LastAttemptAt  *time.Time
	NextAttemptAt  *time.Time `gorm:"index:idx_stop_delivery_due,priority:2"`
	LastError      string     `gorm:"type:text"`
	ConfirmedAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
