package model

import (
	"time"

	"gorm.io/gorm"
)

type Artifact struct {
	ID           uint   `gorm:"primaryKey"`
	ArtifactID   string `gorm:"uniqueIndex;not null"`
	TaskID       string `gorm:"index;not null;uniqueIndex:idx_artifact_identity"`
	RunID        string `gorm:"index;uniqueIndex:idx_artifact_identity"`
	Kind         string `gorm:"index;not null;uniqueIndex:idx_artifact_identity"`
	Name         string `gorm:"not null;uniqueIndex:idx_artifact_identity"`
	URL          string `gorm:"type:text"`
	MetadataJSON string `gorm:"type:text"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (a *Artifact) BeforeCreate(tx *gorm.DB) error {
	if a.ArtifactID == "" {
		a.ArtifactID = NewID("artifact")
	}
	return nil
}
