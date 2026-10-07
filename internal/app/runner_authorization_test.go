package app

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/tenantdb"
	"sdlc-coordinator/internal/workspaceproto"
)

// TestRunnerAuthorizationDecisions 裁决语义：当前 run 才能 commit/push；
// 任务不在可交付状态拒绝；清理只允许非在途工作区；停用租户一律拒绝。
func TestRunnerAuthorizationDecisions(t *testing.T) {
	ctx := context.Background()
	cfg := withTestControlPlane(t, config.Config{})
	control, err := newControlPlane(cfg.ControlPlane, nil)
	if err != nil {
		t.Fatalf("control plane: %v", err)
	}
	tenant, err := control.CreateTenant(ctx, nil, controlplane.TenantInput{Slug: "acme", DisplayName: "Acme"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	platformDB, err := store.Open("file:authz_platform_" + model.NewID("t") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open platform: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := platformDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	resolver := tenantdb.NewSqliteFileResolver(&tenantdb.Store{DB: platformDB}, t.TempDir(), 0)
	handle, err := resolver.Handle(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("tenant handle: %v", err)
	}
	if err := store.Migrate(handle.DB, &model.Task{}, &model.Run{}); err != nil {
		t.Fatalf("migrate tenant store: %v", err)
	}
	t.Cleanup(resolver.Close)
	authorizer := &tenantRunnerAuthorizer{control: control, resolver: resolver}

	// 写任务与两个 run：run-current（当前 run）与 run-stale（已被取代）。
	tasks := repository.NewTaskRepository(handle.DB)
	runs := repository.NewRunRepository(handle.DB)
	runCurrent := &model.Run{RunID: "run-current", TaskID: "task-authz", WorkerRole: "DeveloperWorker", Status: model.RunStatusRunning}
	runStale := &model.Run{RunID: "run-stale", TaskID: "task-authz", WorkerRole: "DeveloperWorker", Status: model.RunStatusRunning}
	if err := runs.Create(ctx, runCurrent); err != nil {
		t.Fatalf("create current run: %v", err)
	}
	if err := runs.Create(ctx, runStale); err != nil {
		t.Fatalf("create stale run: %v", err)
	}
	task := &model.Task{
		TaskID: "task-authz", DedupeKey: "authz:1", Type: "develop_from_issue", Role: "DeveloperWorker",
		Status: model.TaskStatusRunning, CurrentRunID: &runCurrent.RunID,
	}
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	decide := func(kind, runID string) workspaceproto.AuthorizationDecision {
		t.Helper()
		decision, err := authorizer.AuthorizeRunnerWorkspaceOperation(ctx, workspaceproto.AuthorizationRequest{
			OperationID: "op", Kind: kind, TenantID: tenant.ID, TaskID: task.TaskID, RunID: runID,
		})
		if err != nil {
			t.Fatalf("authorize %s/%s: %v", kind, runID, err)
		}
		return decision
	}

	if decision := decide(workspaceproto.KindDeveloperCommit, runCurrent.RunID); !decision.Authorized {
		t.Fatalf("current run commit must be authorized: %+v", decision)
	}
	if decision := decide(workspaceproto.KindDeveloperPush, runCurrent.RunID); !decision.Authorized {
		t.Fatalf("current run push must be authorized: %+v", decision)
	}
	if decision := decide(workspaceproto.KindDeveloperCommit, runStale.RunID); decision.Authorized {
		t.Fatalf("stale run commit must be denied: %+v", decision)
	}
	if decision := decide(workspaceproto.KindCleanup, runCurrent.RunID); decision.Authorized {
		t.Fatalf("cleanup of an in-flight current run must be denied: %+v", decision)
	}
	if decision := decide(workspaceproto.KindCleanup, runStale.RunID); !decision.Authorized {
		t.Fatalf("cleanup of a superseded run must be authorized: %+v", decision)
	}

	// 任务离开可交付状态（终态）后 commit/push 拒绝。
	if err := handle.DB.Model(&model.Task{}).Where("task_id = ?", task.TaskID).Update("status", model.TaskStatusSucceeded).Error; err != nil {
		t.Fatalf("terminalize task: %v", err)
	}
	if decision := decide(workspaceproto.KindDeveloperCommit, runCurrent.RunID); decision.Authorized {
		t.Fatalf("commit after terminal task must be denied: %+v", decision)
	}
	// 终态 run 的清理放行。
	if err := handle.DB.Model(&model.Run{}).Where("run_id = ?", runCurrent.RunID).Update("status", model.RunStatusSucceeded).Error; err != nil {
		t.Fatalf("terminalize run: %v", err)
	}
	if decision := decide(workspaceproto.KindCleanup, runCurrent.RunID); !decision.Authorized {
		t.Fatalf("cleanup of a terminal run must be authorized: %+v", decision)
	}

	// 停用租户一律拒绝。
	if _, err := control.DisableTenant(ctx, nil, tenant.ID); err != nil {
		t.Fatalf("disable tenant: %v", err)
	}
	if decision := decide(workspaceproto.KindCleanup, runStale.RunID); decision.Authorized {
		t.Fatalf("disabled tenant operations must be denied: %+v", decision)
	}
}
