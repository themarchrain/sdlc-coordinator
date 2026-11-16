package app

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"

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

func TestTenantExecutionExitConfirmationIsIsolated(t *testing.T) {
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
	runID := "run-tenant-exit"
	if err := first.Create(&model.Task{
		TaskID: "task-tenant-exit", DedupeKey: "tenant-exit", Type: "bugfix", Role: "DeveloperWorker",
		Status: model.TaskStatusBlocked, CurrentRunID: &runID, Objective: "first tenant",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := first.Create(&model.Run{RunID: runID, TaskID: "task-tenant-exit", WorkerRole: "DeveloperWorker", Status: model.RunStatusBlocked}).Error; err != nil {
		t.Fatal(err)
	}
	if err := first.Create(&model.AgentExecution{
		RunID: runID, ExecutionID: runID, Provider: "codex", Status: model.AgentExecutionStatusInterrupted,
		RequestHash: "request-hash", ErrorCode: "runner_restarted",
	}).Error; err != nil {
		t.Fatal(err)
	}
	executionID, fingerprint := "run-tenant-exit", ""
	var stored model.AgentExecution
	if err := first.Where("execution_id = ?", executionID).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	fingerprint = model.ExecutionExitFingerprint(&stored)
	request := orchestrator.ConfirmExecutionExitRequest{
		ExecutionID: executionID, ExpectedFingerprint: fingerprint,
		RequestID: "request-tenant-exit", Reason: "checked the runner host",
	}
	if _, err := firstEngine.orch.ConfirmExecutionExit(context.Background(), "task-tenant-exit", "tenant-admin", request); err != nil {
		t.Fatalf("own tenant confirmation: %v", err)
	}
	if _, err := secondEngine.orch.ConfirmExecutionExit(context.Background(), "task-tenant-exit", "tenant-admin", request); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("cross tenant confirmation error = %v", err)
	}
}
