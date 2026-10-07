package controlplane_test

import (
	"testing"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/store/storetest"
)

func TestControlPlaneMigrateIdempotentOnMySQL(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	db := storetest.OpenDatabase(t, admin, "cp_migrate")
	repository := controlplane.NewRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := repository.Migrate(); err != nil {
		t.Fatalf("second migrate must be idempotent on mysql: %v", err)
	}
}
