package people

import (
	"strings"
	"time"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

type Person struct {
	ID          uint               `gorm:"primaryKey"`
	PersonID    string             `gorm:"uniqueIndex;not null"`
	DisplayName string             `gorm:"not null"`
	Email       string             `gorm:"not null;collate:nocase;uniqueIndex:uidx_people_email,where:deleted_at IS NULL"`
	Enabled     bool               `gorm:"not null"`
	Identities  []CodeHostIdentity `gorm:"foreignKey:PersonID;references:PersonID"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (Person) TableName() string { return "people" }

func (person *Person) BeforeCreate(_ *gorm.DB) error {
	if strings.TrimSpace(person.PersonID) == "" {
		person.PersonID = model.NewID("person")
	}
	return nil
}

type CodeHostIdentity struct {
	ID              uint   `gorm:"primaryKey"`
	IdentityID      string `gorm:"not null;uniqueIndex:uidx_identity_id,where:deleted_at IS NULL"`
	PersonID        string `gorm:"not null;index"`
	Provider        string `gorm:"not null"`
	CodeHostBaseURL string `gorm:"not null"`
	ExternalUserID  string `gorm:"not null"`
	Username        string `gorm:"not null;collate:nocase"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt `gorm:"index"`
}

func (CodeHostIdentity) TableName() string { return "codehost_identities" }

func (identity *CodeHostIdentity) BeforeCreate(_ *gorm.DB) error {
	if strings.TrimSpace(identity.IdentityID) == "" {
		identity.IdentityID = model.NewID("codehost_identity")
	}
	return nil
}
