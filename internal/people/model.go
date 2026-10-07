package people

import (
	"strings"
	"time"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

type Person struct {
	ID          uint               `gorm:"primaryKey"`
	PersonID    string             `gorm:"uniqueIndex;not null;size:128"`
	DisplayName string             `gorm:"not null"`
	Email     string             `gorm:"not null;collate:nocase;size:191"`
	Enabled   bool               `gorm:"not null"`
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
	ID uint `gorm:"primaryKey"`
	// IdentityID 唯一性同样由 Migrate 的方言化部分唯一索引承担（理由同 Email）。
	IdentityID      string         `gorm:"not null;size:128"`
	PersonID        string         `gorm:"not null;index;size:128"`
	Provider        string         `gorm:"not null;size:64"`
	CodeHostBaseURL string         `gorm:"not null;size:255"`
	ExternalUserID  string         `gorm:"not null;size:128"`
	Username        string         `gorm:"not null;collate:nocase;size:191"`
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
