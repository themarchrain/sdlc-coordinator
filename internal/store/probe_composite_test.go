package store_test

import (
	"testing"

	"sdlc-coordinator/internal/store/storetest"
)

type compositeProbe struct {
	ID uint   `gorm:"primaryKey"`
	A  string `gorm:"uniqueIndex:idx_probe_ab;not null;size:64"`
	B  string `gorm:"uniqueIndex:idx_probe_ab;not null;size:64"`
}

func (compositeProbe) TableName() string { return "composite_probes" }

// TestMySQLCompositeUniqueIndexSecondMigrate 是 GORM MySQL 迁移路径的实证探针：
// 组合唯一索引在第二次 AutoMigrate 是否幂等。
func TestMySQLCompositeUniqueIndexSecondMigrate(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	db := storetest.OpenDatabase(t, admin, "composite_probe")
	if err := db.AutoMigrate(&compositeProbe{}); err != nil {
		t.Fatalf("first auto migrate: %v", err)
	}
	if err := db.AutoMigrate(&compositeProbe{}); err != nil {
		t.Fatalf("second auto migrate: %v", err)
	}
}
