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

func TestStoppedGitAuthorizationDenied(t *testing.T) {
	db, err := store.Open("file:" + model.NewID("stop_auth") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.StopRequest{}, &model.ActionAdmission{}, &model.ResultSubmission{}, &model.WorkItemLease{}, &model.MergeGateAudit{}); err != nil {
		t.Fatal(err)
	}
	runID := "run"
	task := &model.Task{TaskID: "task", DedupeKey: "task", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusRunning, CurrentRunID: &runID}
	for _, row := range []any{task, &model.Run{RunID: runID, TaskID: task.TaskID, Status: model.RunStatusRunning}} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	lease, won, err := repository.NewWorkItemLeaseRepository(db).Acquire(ctx, "developer-delivery:"+runID, "owner", "test", time.Minute)
	if err != nil || !won {
		t.Fatalf("lease: %v", err)
	}
	if _, err := repository.NewStopRepository(db).Stop(ctx, repository.StopTarget{TaskID: task.TaskID}, "admin", "", 3); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{workspaceproto.KindDeveloperCommit, workspaceproto.KindDeveloperPush} {
		decision, err := authorizeRunnerOperation(ctx, db, workspaceproto.AuthorizationRequest{TaskID: task.TaskID, RunID: runID, Kind: kind, Owner: lease.Owner, Generation: lease.Generation})
		if err != nil || decision.Authorized {
			t.Fatalf("stopped Git authorized: %+v %v", decision, err)
		}
	}
}

func TestStoppedCleanupAuthorizationRequiresProof(t *testing.T) {
	db, err := store.Open("file:" + model.NewID("cleanup_auth") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.StopRequest{}, &model.ActionAdmission{}, &model.WorkItemLease{}, &model.Effect{}, &model.Writeback{}, &model.MergeGateAudit{}); err != nil {
		t.Fatal(err)
	}
	runID := "run"
	task := &model.Task{TaskID: "task", DedupeKey: "task", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusInterrupted, CurrentRunID: &runID}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, Status: model.RunStatusInterrupted}
	stop := &model.StopRequest{StopID: "stop", DedupeKey: "task:task", TaskID: task.TaskID, RunID: runID, Scope: "task", RequestedBy: "admin", DeliveryStatus: model.StopDeliveryWaitingExit}
	for _, row := range []any{task, run, stop} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	check := func(want bool) {
		t.Helper()
		decision, err := authorizeRunnerOperation(context.Background(), db, workspaceproto.AuthorizationRequest{TaskID: task.TaskID, RunID: runID, Kind: workspaceproto.KindCleanup})
		if err != nil || decision.Authorized != want {
			t.Fatalf("cleanup=%+v want=%v err=%v", decision, want, err)
		}
	}
	check(false)
	if err := db.Model(stop).Updates(map[string]any{"delivery_status": model.StopDeliveryConfirmed, "confirmation_kind": model.StopConfirmationExecutionExited}).Error; err != nil {
		t.Fatal(err)
	}
	check(true)
	if err := db.Model(run).Update("status", model.RunStatusSucceeded).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(stop).Update("confirmation_kind", "").Error; err != nil {
		t.Fatal(err)
	}
	check(false)
}
