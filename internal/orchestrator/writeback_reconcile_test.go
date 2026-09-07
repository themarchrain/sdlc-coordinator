package orchestrator

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/model"
)

func TestReconcileUnknownWritebackUsesRepositoryResolver(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, artifactRepo, writebackRepo := newGitLabWritebackRepositories(t)
	perRepoBackend := &fakeGitLabWritebackBackend{reconcileResult: codehost.ReconcileResult{State: codehost.ReconcileStateFound, ExternalID: "7"}}
	resolver := &staticCodeHostResolver{backends: map[string]codehost.Backend{"repo-framework": perRepoBackend}}
	orch := New(
		taskRepo,
		runRepo,
		WithArtifactRepository(artifactRepo),
		WithWritebackRepository(writebackRepo),
		WithCodeHostBackendResolver(resolver),
	)

	repositoryID := "repo-framework"
	task := &model.Task{
		TaskID: "task-reconcile-resolver", DedupeKey: "dedupe-reconcile-resolver", Type: "bugfix", Role: "DeveloperWorker",
		RepositoryID: &repositoryID, Status: model.TaskStatusRunning,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	run := &model.Run{RunID: "run-reconcile-resolver", TaskID: task.TaskID, WorkerRole: "DeveloperWorker", Status: model.RunStatusRunning}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	marker := CodeHostOperationMarker("codehost:mr:framework:resolver")
	if err := writebackRepo.Create(ctx, &model.Writeback{
		TaskID: task.TaskID, RunID: run.RunID, DedupeKey: "codehost:mr:framework:resolver",
		Kind: CodeHostWritebackCreateMergeRequest, ProjectID: "group/framework",
		Status: model.WritebackStatusUnknown, OperationMarker: marker,
		RequestJSON: `{"kind":"create_merge_request","project_id":"group/framework","source_branch":"agent/x","target_branch":"main"}`,
	}); err != nil {
		t.Fatalf("create unknown writeback: %v", err)
	}

	results, err := orch.ReconcileUnknownWritebacks(ctx, 10)
	if err != nil {
		t.Fatalf("reconcile unknown writebacks: %v", err)
	}
	if len(results) != 1 || results[0].State != codehost.ReconcileStateFound {
		t.Fatalf("unexpected reconcile results: %#v", results)
	}
	if perRepoBackend.reconcileCalls != 1 {
		t.Fatalf("expected reconcile through per-repo backend, got %d calls", perRepoBackend.reconcileCalls)
	}
}
