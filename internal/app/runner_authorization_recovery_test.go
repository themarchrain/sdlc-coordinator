package app

import (
	"context"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/workspaceproto"
)

func TestRunnerAuthorizationRejectsSupersededRecoveryGeneration(t *testing.T) {
	db, err := store.Open("file:authz_recovery_" + model.NewID("t") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.WorkItemLease{}, &model.StopRequest{}); err != nil {
		t.Fatal(err)
	}
	current := &model.Run{RunID: "run-current-gen1", TaskID: "task-gen1", WorkerRole: "DeveloperWorker", Status: model.RunStatusRunning, RecoveryGeneration: 1}
	stale := &model.Run{RunID: "run-stale-gen0", TaskID: "task-gen1", WorkerRole: "DeveloperWorker", Status: model.RunStatusRunning, RecoveryGeneration: 0}
	if err := db.Create(current).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(stale).Error; err != nil {
		t.Fatal(err)
	}
	task := &model.Task{TaskID: "task-gen1", DedupeKey: "authz-gen1", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusRunning, CurrentRunID: &current.RunID, RecoveryGeneration: 1}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	lease, won, err := repository.NewWorkItemLeaseRepository(db).Acquire(context.Background(), "developer-delivery:"+stale.RunID, "owner", "developer-delivery", time.Minute)
	if err != nil || !won {
		t.Fatalf("acquire stale lease: won=%v err=%v", won, err)
	}
	decision, err := authorizeRunnerOperation(context.Background(), db, workspaceproto.AuthorizationRequest{
		OperationID: "op", Kind: workspaceproto.KindDeveloperCommit, TaskID: task.TaskID, RunID: stale.RunID,
		Owner: lease.Owner, Generation: lease.Generation,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Authorized {
		t.Fatalf("stale recovery generation was authorized: %+v", decision)
	}
}
