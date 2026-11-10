package app

import (
	"context"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
)

func TestRecoveryMigrationPreservesLegacyRows(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := store.Migrate(db, &store.SchemaMigration{}, &model.Task{}, &model.Run{}, &model.AgentExecution{}); err != nil {
		t.Fatal(err)
	}
	legacyTask := &model.Task{TaskID: "legacy-recovery-task", DedupeKey: "legacy-recovery", Type: "bugfix", Role: "DeveloperWorker", Status: model.TaskStatusBlocked, AttemptCount: 5}
	if err := db.Create(legacyTask).Error; err != nil {
		t.Fatal(err)
	}
	legacyRun := &model.Run{RunID: "legacy-recovery-run", TaskID: legacyTask.TaskID, WorkerRole: legacyTask.Role, Status: model.RunStatusBlocked}
	if err := db.Create(legacyRun).Error; err != nil {
		t.Fatal(err)
	}
	for _, drop := range []struct {
		model any
		field string
	}{
		{&model.Task{}, "recovery_generation"},
		{&model.Task{}, "retry_cycle_base_count"},
		{&model.Task{}, "retry_cycle_max_attempts"},
		{&model.Run{}, "recovery_generation"},
		{&model.AgentExecution{}, "failure_json"},
	} {
		if err := db.Migrator().DropColumn(drop.model, drop.field); err != nil {
			t.Fatalf("drop legacy column %s: %v", drop.field, err)
		}
	}
	if err := db.Create(&store.SchemaMigration{Component: "business", Version: 12}).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateBusinessStore(db); err != nil {
		t.Fatalf("migrate recovery schema: %v", err)
	}
	if version, err := store.CurrentSchemaVersion(context.Background(), db, "business"); err != nil || version != store.SchemaVersion {
		t.Fatalf("schema version = %d, err=%v, want %d", version, err, store.SchemaVersion)
	}
	if !db.Migrator().HasTable(&model.TaskRecovery{}) {
		t.Fatal("task recovery table was not created")
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
			t.Fatalf("migration omitted %s", check.field)
		}
	}
	var task model.Task
	if err := db.Where("task_id = ?", legacyTask.TaskID).First(&task).Error; err != nil {
		t.Fatal(err)
	}
	if task.Status != model.TaskStatusBlocked || task.AttemptCount != 5 || task.RecoveryGeneration != 0 || task.RetryCycleBaseCount != 0 || task.RetryCycleMaxAttempts != 0 {
		t.Fatalf("migration changed legacy task history: %+v", task)
	}
	var run model.Run
	if err := db.Where("run_id = ?", legacyRun.RunID).First(&run).Error; err != nil {
		t.Fatal(err)
	}
	if run.Status != model.RunStatusBlocked || run.RecoveryGeneration != 0 {
		t.Fatalf("migration changed legacy run history: %+v", run)
	}
	if err := migrateBusinessStore(db); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}

	first := model.TaskRecovery{
		TaskID: legacyTask.TaskID, RequestID: "legacy-request", RequestHash: "hash", SourceGeneration: 1,
		TargetGeneration: 2, BlockRevision: "revision", RequestedBy: "member-1", RequestedAt: time.Now().UTC(),
		Classification: "unknown", AttemptCountBefore: 5, CycleMaxAttempts: 3,
	}
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	duplicateRequest := first
	duplicateRequest.ID, duplicateRequest.RecoveryID = 0, ""
	duplicateRequest.SourceGeneration = 2
	if err := db.Create(&duplicateRequest).Error; err == nil {
		t.Fatal("duplicate task/request recovery accepted")
	}
	duplicateGeneration := first
	duplicateGeneration.ID, duplicateGeneration.RecoveryID = 0, ""
	duplicateGeneration.RequestID = "other-request"
	if err := db.Create(&duplicateGeneration).Error; err == nil {
		t.Fatal("duplicate task/source generation recovery accepted")
	}
}
