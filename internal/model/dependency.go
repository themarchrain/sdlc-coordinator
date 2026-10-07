package model

import (
	"time"

	"gorm.io/gorm"
)

type DependencyStatus string

const (
	DependencyStatusPending  DependencyStatus = "pending"
	DependencyStatusReady    DependencyStatus = "ready"
	DependencyStatusBlocked  DependencyStatus = "blocked"
	DependencyStatusResolved DependencyStatus = "resolved"
)

type Dependency struct {
	ID           uint             `gorm:"primaryKey"`
	DependencyID string           `gorm:"uniqueIndex;not null;size:128"`
	ParentTaskID string           `gorm:"index;not null;uniqueIndex:idx_dependency_identity;size:128"`
	ChildTaskID  string           `gorm:"index;not null;uniqueIndex:idx_dependency_identity;size:128"`
	Relation     string           `gorm:"index;not null;uniqueIndex:idx_dependency_identity;size:32"`
	Status       DependencyStatus `gorm:"index;not null;size:32"`
	MetadataJSON       string           `gorm:"type:text"`
	ParentRepositoryID *string          `gorm:"-"`
	ChildRepositoryID  *string          `gorm:"-"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (d *Dependency) BeforeCreate(tx *gorm.DB) error {
	if d.DependencyID == "" {
		d.DependencyID = NewID("dependency")
	}
	if d.Status == "" {
		d.Status = DependencyStatusPending
	}
	return nil
}
