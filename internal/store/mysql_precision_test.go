package store_test

import (
	"context"
	"testing"
	"time"

	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/storetest"
)

func TestMySQLMigrationKeepsMicrosecondPrecision(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	for _, budgeted := range []bool{false, true} {
		name := "standard"
		if budgeted {
			name = "budgeted"
		}
		t.Run(name, func(t *testing.T) {
			for _, existing := range []bool{false, true} {
				name := "fresh"
				if existing {
					name = "existing"
				}
				t.Run(name, func(t *testing.T) {
					dsn := storetest.NewDatabase(t, admin, "precision")
					spec, err := store.ParseDSN(dsn)
					if err != nil {
						t.Fatal(err)
					}
					if budgeted {
						budget, err := store.NewConnectionBudget(4)
						if err != nil {
							t.Fatal(err)
						}
						spec = spec.WithConnectionBudget(budget)
					}
					db, err := store.OpenSpec(spec)
					if err != nil {
						t.Fatal(err)
					}
					sqlDB, err := db.DB()
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = sqlDB.Close() })
					type probe struct {
						ID        uint64    `gorm:"primaryKey"`
						CreatedAt time.Time `gorm:"not null"`
					}
					written := time.Date(2026, 1, 2, 3, 4, 5, 123456000, time.UTC)
					if existing {
						if err := db.Exec("CREATE TABLE precision_probe(id bigint unsigned AUTO_INCREMENT PRIMARY KEY, created_at datetime(6) NOT NULL)").Error; err != nil {
							t.Fatal(err)
						}
						if err := db.Exec("INSERT INTO precision_probe(id,created_at) VALUES (1,?)", written).Error; err != nil {
							t.Fatal(err)
						}
					}
					migrate := func() error { return db.Table("precision_probe").AutoMigrate(&probe{}) }
					if err := store.MigrateGuarded(context.Background(), db, "precision", migrate); err != nil {
						t.Fatal(err)
					}
					if !existing {
						if err := db.Table("precision_probe").Create(&probe{ID: 1, CreatedAt: written}).Error; err != nil {
							t.Fatal(err)
						}
					}
					if err := store.MigrateGuarded(context.Background(), db, "precision", migrate, store.Force()); err != nil {
						t.Fatal(err)
					}
					var precision int
					if err := db.Raw("SELECT DATETIME_PRECISION FROM information_schema.columns WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='precision_probe' AND COLUMN_NAME='created_at'").Scan(&precision).Error; err != nil {
						t.Fatal(err)
					}
					if precision != 6 {
						t.Errorf("datetime precision = %d, want 6", precision)
					}
					var actual time.Time
					if err := db.Raw("SELECT created_at FROM precision_probe WHERE id=1").Scan(&actual).Error; err != nil {
						t.Fatal(err)
					}
					if !actual.Equal(written) {
						t.Fatalf("migration changed stored timestamp: got %s, want %s", actual.Format(time.RFC3339Nano), written.Format(time.RFC3339Nano))
					}
				})
			}
		})
	}
}
