package model

import "time"

type WorkItemLease struct {
	Scope      string `gorm:"column:scope;primaryKey;size:191"`
	Owner      string `gorm:"column:owner;not null;size:64"`
	Generation uint64 `gorm:"column:generation;not null"`
	// OperationID 记录本代次绑定的外部操作 ID（重放/回读对账用，可选）。
	OperationID string    `gorm:"column:operation_id;not null;default:'';size:128"`
	LeaseUntil  time.Time `gorm:"column:lease_until;not null;index"`
	AcquiredAt  time.Time `gorm:"column:acquired_at;not null"`
	UpdatedAt   time.Time `gorm:"column:updated_at;not null"`
}

func (WorkItemLease) TableName() string { return "work_item_leases" }
