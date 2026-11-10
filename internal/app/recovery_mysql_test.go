package app

import (
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/storetest"
)

func TestRecoveryMigrationOnMySQL(t *testing.T) {
	db := storetest.OpenDatabase(t, storetest.RequireMySQL(t), "recovery_schema")
	if err := migrateBusinessStore(db); err != nil {
		t.Fatalf("migrate recovery schema: %v", err)
	}
	if err := migrateBusinessStore(db); err != nil {
		t.Fatalf("repeat recovery migration: %v", err)
	}
	if !db.Migrator().HasTable(&model.TaskRecovery{}) {
		t.Fatal("task recovery table missing on mysql")
	}
	for _, check := range []struct {
		model any
		field string
	}{
		{&model.Task{}, "recovery_generation"},
		{&model.Task{}, "retry_cycle_base_count"},
		{&model.Task{}, "retry_cycle_max_attempts"},
		{&model.Run{}, "recovery_generation"},
		{&model.AgentExecution{}, "failure_json"},
	} {
		if !db.Migrator().HasColumn(check.model, check.field) {
			t.Fatalf("mysql migration omitted %s", check.field)
		}
	}
	first := model.TaskRecovery{
		TaskID: "mysql-recovery-task", RequestID: "request-1", RequestHash: "hash", SourceGeneration: 1,
		TargetGeneration: 2, BlockRevision: "revision", RequestedBy: "member", RequestedAt: time.Now().UTC(),
		Classification: "unknown", AttemptCountBefore: 0, CycleMaxAttempts: 3,
	}
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	duplicate := first
	duplicate.ID, duplicate.RecoveryID = 0, ""
	duplicate.RequestID = "request-2"
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("mysql duplicate task/source generation accepted")
	}
	_ = store.DriverMySQL
}
