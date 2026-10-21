package app

import (
	"context"
	"testing"

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
	if version != 8 {
		t.Fatalf("schema = %d, want 8", version)
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
