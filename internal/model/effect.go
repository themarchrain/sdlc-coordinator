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
	// delivery_unverified:HTTP 2xx 但回执超限/读取中断/无法解析——消息
	// 可能已送达但无法核实,独立终态,不进入常规重试(避免重复发送)。
	EffectStatusDeliveryUnverified EffectStatus = "delivery_unverified"
)

type Effect struct {
	ID           uint         `gorm:"primaryKey"`
	EffectID     string       `gorm:"uniqueIndex;not null;size:128"`
	TaskID       string       `gorm:"index;not null;size:128"`
	RunID        string       `gorm:"index;not null;size:128"`
	DedupeKey    string       `gorm:"uniqueIndex;not null;size:255"`
	Kind         string       `gorm:"index;not null;size:64"`
	Target       string       `gorm:"index;not null;size:255"`
	Critical     bool         `gorm:"not null;default:false"`
	Status       EffectStatus `gorm:"index;not null;size:32"`
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
