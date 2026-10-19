package controlplane

import (
	"path/filepath"
	"testing"

	"sdlc-coordinator/internal/store"
)

func openTestControlPlane(t *testing.T) *Repository {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "control-plane.db"))
	if err != nil {
		t.Fatalf("open control plane database: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return NewRepository(db)
}

func TestMigrateCreatesControlPlaneTables(t *testing.T) {
	repository := openTestControlPlane(t)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate control plane: %v", err)
	}
	for _, table := range []string{"users", "tenants", "memberships", "invite_codes", "runner_assignments", "runner_pool_cursors", "audit_log", "sessions"} {
		if !repository.db.Migrator().HasTable(table) {
			t.Fatalf("control plane table %q was not created", table)
		}
	}
}
