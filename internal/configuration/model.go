package configuration

import "time"

type SnapshotRecord struct {
	ID          uint   `gorm:"primaryKey"`
	Revision    uint64 `gorm:"uniqueIndex;not null"`
	Status      string `gorm:"not null"`
	Source      string `gorm:"not null"`
	PayloadJSON string `gorm:"type:text;not null"`
	AppliedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (SnapshotRecord) TableName() string {
	return "runtime_config_snapshots"
}

type Snapshot struct {
	Revision  uint64         `json:"revision"`
	Status    SnapshotStatus `json:"status"`
	Source    SnapshotSource `json:"source"`
	Config    RuntimeConfig  `json:"config"`
	AppliedAt *time.Time     `json:"applied_at,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}
