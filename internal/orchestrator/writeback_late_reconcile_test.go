package orchestrator

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/model"
)

// TestLateReconcileCannotRegressSucceededWriteback 审查 #5 的时序回归：对账
// 读到 unknown 后（其 absent 结论迟到），发送方已把同一写回推进到 succeeded；
// 迟到的对账写回必须被条件写拒绝，succeeded 保持不动，且不会再触发一次
// 远端创建。对账与发送共享同一执行权，本测试直接构造对账观察滞后的
// 时序（发送方在两个编排器之外完成，等价于竞争已发生）。
func TestLateReconcileCannotRegressSucceededWriteback(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, artifactRepo, writebackRepo := newGitLabWritebackRepositories(t)
	// 远端回查给出 absent（旧观察）：该结论不得覆盖并发完成的 succeeded。
	backend := &fakeGitLabWritebackBackend{reconcileResult: codehost.ReconcileResult{State: codehost.ReconcileStateAbsent}}
	resolver := &staticCodeHostResolver{backends: map[string]codehost.Backend{"repo-late": backend}}
	orch := New(
		taskRepo,
		runRepo,
		WithArtifactRepository(artifactRepo),
		WithWritebackRepository(writebackRepo),
		WithCodeHostBackendResolver(resolver),
	)

	repositoryID := "repo-late"
	task := &model.Task{
		TaskID: "task-late-reconcile", DedupeKey: "dedupe-late-reconcile", Type: "bugfix", Role: "DeveloperWorker",
		RepositoryID: &repositoryID, Status: model.TaskStatusRunning,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	run := &model.Run{RunID: "run-late-reconcile", TaskID: task.TaskID, WorkerRole: "DeveloperWorker", Status: model.RunStatusRunning}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	dedupeKey := "codehost:mr:late:1"
	if err := writebackRepo.Create(ctx, &model.Writeback{
		TaskID: task.TaskID, RunID: run.RunID, DedupeKey: dedupeKey,
		Kind: CodeHostWritebackCreateMergeRequest, ProjectID: "group/late",
		Status: model.WritebackStatusUnknown, OperationMarker: CodeHostOperationMarker(dedupeKey),
		RequestJSON: `{"kind":"create_merge_request","project_id":"group/late","source_branch":"agent/x","target_branch":"main"}`,
	}); err != nil {
		t.Fatalf("create unknown writeback: %v", err)
	}

	staleView, err := writebackRepo.GetByDedupeKey(ctx, dedupeKey)
	if err != nil {
		t.Fatalf("load stale reconcile view: %v", err)
	}
	senderView := *staleView
	senderView.Status = model.WritebackStatusSucceeded
	senderView.ExternalID = "55"
	senderView.ExternalURL = "https://codehost.example/mr/55"
	if _, err := writebackRepo.UpdateGuardedIfStatus(ctx, &senderView, model.WritebackStatusUnknown); err != nil {
		t.Fatalf("simulate concurrent sender success: %v", err)
	}

	result, err := orch.reconcileOneWriteback(ctx, staleView)
	if err != nil {
		t.Fatalf("reconcile stale writeback: %v", err)
	}
	if backend.reconcileCalls != 1 || result.State != codehost.ReconcileStateAbsent {
		t.Fatalf("stale absent observation did not run: calls=%d result=%+v", backend.reconcileCalls, result)
	}
	stored, err := writebackRepo.GetByDedupeKey(ctx, dedupeKey)
	if err != nil {
		t.Fatalf("reload writeback: %v", err)
	}
	if stored.Status != model.WritebackStatusSucceeded {
		t.Fatalf("late reconcile regressed the writeback: status=%s", stored.Status)
	}
	if stored.ExternalID != "55" {
		t.Fatalf("late reconcile clobbered the external id: %q", stored.ExternalID)
	}
}
