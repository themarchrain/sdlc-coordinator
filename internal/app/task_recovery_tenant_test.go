package app

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"

	"gorm.io/gorm"
)

func TestTenantRecoveryStoresAreIsolated(t *testing.T) {
	first := openRecoveryTenantDB(t)
	second := openRecoveryTenantDB(t)
	shared := engineShared{cfg: config.Config{}, logger: zap.NewNop()}
	firstEngine, err := buildTenantEngine(first, 101, shared)
	if err != nil {
		t.Fatal(err)
	}
	secondEngine, err := buildTenantEngine(second, 202, shared)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Create(&model.Task{TaskID: "same-task-id", DedupeKey: "first", Type: "bugfix", Role: "DeveloperWorker", Status: model.TaskStatusBlocked, Objective: "first tenant"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := second.Create(&model.Task{TaskID: "same-task-id", DedupeKey: "second", Type: "bugfix", Role: "DeveloperWorker", Status: model.TaskStatusBlocked, Objective: "second tenant"}).Error; err != nil {
		t.Fatal(err)
	}
	firstTask, err := firstEngine.orch.GetTask(context.Background(), "same-task-id")
	if err != nil {
		t.Fatal(err)
	}
	secondTask, err := secondEngine.orch.GetTask(context.Background(), "same-task-id")
	if err != nil {
		t.Fatal(err)
	}
	if firstTask.Objective != "first tenant" || secondTask.Objective != "second tenant" {
		t.Fatalf("tenant recovery stores crossed: first=%+v second=%+v", firstTask, secondTask)
	}
}

func openRecoveryTenantDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := store.Open("file:" + model.NewID("recovery_tenant") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := migrateBusinessStore(db); err != nil {
		t.Fatal(err)
	}
	return db
}
