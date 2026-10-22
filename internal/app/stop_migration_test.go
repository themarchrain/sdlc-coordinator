package app

import (
	"context"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
)

func TestCancellationMigrationUpgradesSchema7(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := store.Migrate(db, &store.SchemaMigration{}, &model.Task{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&store.SchemaMigration{Component: "business", Version: 7}).Error; err != nil {
		t.Fatal(err)
	}
	task := &model.Task{TaskID: "old-success", DedupeKey: "old", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusSucceeded, AttemptCount: 2}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateBusinessStore(db); err != nil {
		t.Fatal(err)
	}
	version, err := store.CurrentSchemaVersion(context.Background(), db, "business")
	if err != nil {
		t.Fatal(err)
	}
	if version != store.SchemaVersion {
		t.Fatalf("schema = %d, want %d", version, store.SchemaVersion)
	}
	if !db.Migrator().HasTable(&model.StopRequest{}) || !db.Migrator().HasTable(&model.ActionAdmission{}) {
		t.Fatal("business migration omitted cancellation tables")
	}
	var current model.Task
	if err := db.Where("task_id = ?", task.TaskID).First(&current).Error; err != nil {
		t.Fatal(err)
	}
	if current.Status != model.TaskStatusSucceeded || current.AttemptCount != 2 || current.DedupeKey != "old" {
		t.Fatalf("migration changed history: %+v", current)
	}
	if err := migrateBusinessStore(db); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
	first := model.ActionAdmission{AdmissionID: "a1", TaskID: task.TaskID, Kind: model.ActionWriteback, ActionKey: "stable-action", Attempt: 1, LeaseOwner: "owner", LeaseGeneration: 1}
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	duplicate := first
	duplicate.ID, duplicate.AdmissionID = 0, "a2"
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("same action attempt admitted twice")
	}
}

func TestCancellationMigrationAddsNonDeliveryProof(t *testing.T) {
	db, err := store.Open("file:" + model.NewID("admission-upgrade") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := store.Migrate(db, &store.SchemaMigration{}, &model.ActionAdmission{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropColumn(&model.ActionAdmission{}, "not_sent_at"); err != nil {
		t.Fatal(err)
	}
	if err := db.Table("action_admissions").Create(map[string]any{"admission_id": "old-permit", "task_id": "old-task", "kind": "writeback", "action_key": "old-action", "attempt": 1, "lease_owner": "old-owner", "lease_generation": 7, "admitted_at": time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&store.SchemaMigration{Component: "business", Version: 8}).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateBusinessStore(db); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasColumn(&model.ActionAdmission{}, "not_sent_at") {
		t.Fatal("admission migration omitted durable non-delivery proof")
	}
	var previous model.ActionAdmission
	if err := db.Where("admission_id = ?", "old-permit").First(&previous).Error; err != nil || previous.NotSentAt != nil || previous.LeaseOwner != "old-owner" || previous.LeaseGeneration != 7 {
		t.Fatalf("migration invented non-delivery or changed history: %+v err=%v", previous, err)
	}
}

func TestCancellationMigrationPreservesUnknownConfirmation(t *testing.T) {
	db, err := store.Open("file:" + model.NewID("confirmation_upgrade") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := store.Migrate(db, &store.SchemaMigration{}, &model.StopRequest{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropColumn(&model.StopRequest{}, "confirmation_kind"); err != nil {
		t.Fatal(err)
	}
	if err := db.Table("stop_requests").Create(map[string]any{"stop_id": "old-stop", "dedupe_key": "task:old", "scope": "task", "task_id": "old", "requested_by": "admin", "requested_at": time.Now().UTC(), "reason_code": "manual_stop", "delivery_status": "confirmed", "attempt_count": 0}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&store.SchemaMigration{Component: "business", Version: 10}).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateBusinessStore(db); err != nil {
		t.Fatal(err)
	}
	var previous model.StopRequest
	if err := db.Where("stop_id = ?", "old-stop").First(&previous).Error; err != nil {
		t.Fatal(err)
	}
	if previous.ConfirmationKind != "" || previous.DeliveryStatus != model.StopDeliveryConfirmed {
		t.Fatalf("migration fabricated exit: %+v", previous)
	}
	if err := migrateBusinessStore(db); err != nil {
		t.Fatal(err)
	}
}
