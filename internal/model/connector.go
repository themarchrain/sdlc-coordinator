package model

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

type ConnectorHealthStatus string

const (
	ConnectorHealthUnknown     ConnectorHealthStatus = "unknown"
	ConnectorHealthHealthy     ConnectorHealthStatus = "healthy"
	ConnectorHealthDegraded    ConnectorHealthStatus = "degraded"
	ConnectorHealthUnavailable ConnectorHealthStatus = "unavailable"
)

type ConnectorInstance struct {
	ID                uint                  `gorm:"primaryKey"`
	ConnectorID       string                `gorm:"uniqueIndex;not null"`
	Kind              string                `gorm:"not null;uniqueIndex:idx_connector_kind_url"`
	DisplayName       string                `gorm:"not null"`
	BaseURL           string                `gorm:"type:text;not null;uniqueIndex:idx_connector_kind_url"`
	Enabled           bool                  `gorm:"not null"`
	TargetProjectPath string                `gorm:"index"`
	CapabilitiesJSON  string                `gorm:"type:text;not null"`
	LastHealthStatus  ConnectorHealthStatus `gorm:"index;not null"`
	LastHealthMessage string                `gorm:"type:text"`
	LastHealthAt      *time.Time            `gorm:"index"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (connector *ConnectorInstance) BeforeCreate(tx *gorm.DB) error {
	if strings.TrimSpace(connector.ConnectorID) == "" {
		connector.ConnectorID = NewID("connector")
	}
	if strings.TrimSpace(connector.CapabilitiesJSON) == "" {
		connector.CapabilitiesJSON = "[]"
	}
	if connector.LastHealthStatus == "" {
		connector.LastHealthStatus = ConnectorHealthUnknown
	}
	return nil
}
