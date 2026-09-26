package orchestrator

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

func TestApplyCanonicalResultExpandsNextTasksAndWaitsUpstream(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, dependencyRepo := newCallbackNextTaskRepositories(t)
	orch := New(taskRepo, runRepo, WithDependencyRepository(dependencyRepo))
	createCallbackNextTaskFixture(t, ctx, taskRepo, runRepo, "task-next-parent", "run-next-parent")

	task, run, err := orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID:  "task-next-parent",
		RunID:   "run-next-parent",
		Status:  results.StatusSucceeded,
		Summary: "found upstream issue",
		NextTasks: []results.NextTaskIntent{
			{
				Role:               "DeveloperWorker",
				Type:               "upstream_fix",
				TargetRepo:         "framework",
				Objective:          "fix framework validator",
				AcceptanceCriteria: "framework tests pass",
				Context:            map[string]any{"failure_evidence": "downstream test failed"},
			},
		},
	})
	if err != nil {
		t.Fatalf("apply callback: %v", err)
	}

	if task.Status != model.TaskStatusWaitingUpstream {
		t.Fatalf("expected parent waiting_upstream, got %s", task.Status)
	}
	if run.Status != model.RunStatusSucceeded {
		t.Fatalf("expected run succeeded, got %s", run.Status)
	}

	child, err := taskRepo.GetByDedupeKey(ctx, "next:task-next-parent:0")
	if err != nil {
		t.Fatalf("get child by dedupe: %v", err)
	}
	if child.Status != model.TaskStatusPending {
		t.Fatalf("expected child pending, got %s", child.Status)
	}
	if child.Role != "DeveloperWorker" {
		t.Fatalf("expected DeveloperWorker child, got %s", child.Role)
	}
	if child.TargetRepo != "framework" {
		t.Fatalf("expected framework target, got %s", child.TargetRepo)
	}

	dependencies, err := dependencyRepo.ListByParentTaskID(ctx, "task-next-parent")
	if err != nil {
		t.Fatalf("list dependencies: %v", err)
	}
	if len(dependencies) != 1 {
		t.Fatalf("expected one dependency, got %d", len(dependencies))
	}
	if dependencies[0].ChildTaskID != child.TaskID {
		t.Fatalf("expected dependency child %s, got %s", child.TaskID, dependencies[0].ChildTaskID)
	}
	if dependencies[0].Relation != "upstream_fix" {
		t.Fatalf("expected upstream_fix relation, got %s", dependencies[0].Relation)
	}
	if dependencies[0].Status != model.DependencyStatusPending {
		t.Fatalf("expected pending dependency, got %s", dependencies[0].Status)
	}
}

func TestApplyCanonicalResultExpandsNextTasksIdempotently(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, dependencyRepo := newCallbackNextTaskRepositories(t)
	orch := New(taskRepo, runRepo, WithDependencyRepository(dependencyRepo))
	createCallbackNextTaskFixture(t, ctx, taskRepo, runRepo, "task-next-idempotent", "run-next-idempotent")

	payload := results.CanonicalResult{
		TaskID: "task-next-idempotent",
		RunID:  "run-next-idempotent",
		Status: results.StatusSucceeded,
		NextTasks: []results.NextTaskIntent{
			{
				DedupeKey: "next:task-next-idempotent:framework",
				Role:      "DeveloperWorker",
				Objective: "fix framework validator",
			},
		},
	}
	if _, _, err := orch.ApplyCanonicalResult(ctx, payload); err != nil {
		t.Fatalf("first callback: %v", err)
	}
	if _, _, err := orch.ApplyCanonicalResult(ctx, payload); err != nil {
		t.Fatalf("second callback: %v", err)
	}

	child, err := taskRepo.GetByDedupeKey(ctx, "next:task-next-idempotent:framework")
	if err != nil {
		t.Fatalf("get child: %v", err)
	}
	dependencies, err := dependencyRepo.ListByParentTaskID(ctx, "task-next-idempotent")
	if err != nil {
		t.Fatalf("list dependencies: %v", err)
	}
	if len(dependencies) != 1 {
		t.Fatalf("expected one dependency after repeated callback, got %d", len(dependencies))
	}
	if dependencies[0].ChildTaskID != child.TaskID {
		t.Fatalf("expected dependency child %s, got %s", child.TaskID, dependencies[0].ChildTaskID)
	}
}

// fire-and-forget 派生即派生但不等待:父任务照常落终态(不进
// waiting_upstream)、不建依赖行,子任务血缘留在其上下文的 parent 字段里。
// 阻塞式派生(upstream_fix)的 waiting_upstream 语义由既有测试钉住,不受影响。
func TestApplyCanonicalResultFireAndForgetNextTaskLeavesParentSucceeded(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, dependencyRepo := newCallbackNextTaskRepositories(t)
	orch := New(taskRepo, runRepo, WithDependencyRepository(dependencyRepo))
	createCallbackNextTaskFixture(t, ctx, taskRepo, runRepo, "task-next-fnf-parent", "run-next-fnf-parent")

	task, _, err := orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID:  "task-next-fnf-parent",
		RunID:   "run-next-fnf-parent",
		Status:  results.StatusSucceeded,
		Summary: "approved",
		NextTasks: []results.NextTaskIntent{
			{
				DedupeKey:     "verify:group/project:21:abc123",
				Role:          "TesterWorker",
				Type:          "verify_merge_request",
				Objective:     "verify the merge request",
				FireAndForget: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("apply callback: %v", err)
	}
	if task.Status != model.TaskStatusSucceeded {
		t.Fatalf("fire-and-forget derivation must leave the parent on its normal terminal status, got %s", task.Status)
	}

	child, err := taskRepo.GetByDedupeKey(ctx, "verify:group/project:21:abc123")
	if err != nil {
		t.Fatalf("get fire-and-forget child: %v", err)
	}
	if child.Status != model.TaskStatusPending || child.Role != "TesterWorker" {
		t.Fatalf("child must be a dispatchable verification task: %#v", child)
	}
	dependencies, err := dependencyRepo.ListByParentTaskID(ctx, "task-next-fnf-parent")
	if err != nil {
		t.Fatalf("list dependencies: %v", err)
	}
	if len(dependencies) != 0 {
		t.Fatalf("fire-and-forget derivation must not create dependency rows, got %d", len(dependencies))
	}
}

func TestApplyCanonicalResultNextTaskValidationFailureDoesNotAdvanceParentOrRun(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, dependencyRepo := newCallbackNextTaskRepositories(t)
	orch := New(taskRepo, runRepo, WithDependencyRepository(dependencyRepo))
	createCallbackNextTaskFixture(t, ctx, taskRepo, runRepo, "task-next-invalid", "run-next-invalid")

	_, _, err := orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID: "task-next-invalid",
		RunID:  "run-next-invalid",
		Status: results.StatusSucceeded,
		NextTasks: []results.NextTaskIntent{
			{
				Objective: "missing role",
			},
		},
	})
	if err == nil {
		t.Fatal("expected invalid next task to fail")
	}

	task, err := taskRepo.GetByTaskID(ctx, "task-next-invalid")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.Status != model.TaskStatusRunning {
		t.Fatalf("expected parent to remain running, got %s", task.Status)
	}

	run, err := runRepo.GetByRunID(ctx, "run-next-invalid")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != model.RunStatusRunning {
		t.Fatalf("expected run to remain running, got %s", run.Status)
	}
}

func TestApplyCanonicalResultResolvesDependencyAndRestoresParentWhenChildSucceeds(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, dependencyRepo := newCallbackNextTaskRepositories(t)
	orch := New(taskRepo, runRepo, WithDependencyRepository(dependencyRepo))
	createWaitingParentAndChildFixture(t, ctx, taskRepo, runRepo, dependencyRepo, "task-parent-resume", "task-child-resume", "run-child-resume")

	_, _, err := orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID: "task-child-resume",
		RunID:  "run-child-resume",
		Status: results.StatusSucceeded,
	})
	if err != nil {
		t.Fatalf("apply child callback: %v", err)
	}

	parent, err := taskRepo.GetByTaskID(ctx, "task-parent-resume")
	if err != nil {
		t.Fatalf("get parent: %v", err)
	}
	if parent.Status != model.TaskStatusPending {
		t.Fatalf("expected parent restored to pending, got %s", parent.Status)
	}

	dependencies, err := dependencyRepo.ListByChildTaskID(ctx, "task-child-resume")
	if err != nil {
		t.Fatalf("list dependencies: %v", err)
	}
	if len(dependencies) != 1 {
		t.Fatalf("expected one dependency, got %d", len(dependencies))
	}
	if dependencies[0].Status != model.DependencyStatusResolved {
		t.Fatalf("expected dependency resolved, got %s", dependencies[0].Status)
	}
}

func TestApplyCanonicalResultDoesNotRestoreParentUntilAllDependenciesResolved(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, dependencyRepo := newCallbackNextTaskRepositories(t)
	orch := New(taskRepo, runRepo, WithDependencyRepository(dependencyRepo))
	createWaitingParentAndChildFixture(t, ctx, taskRepo, runRepo, dependencyRepo, "task-parent-multi", "task-child-first", "run-child-first")
	createRunningChildWithDependencyFixture(t, ctx, taskRepo, runRepo, dependencyRepo, "task-parent-multi", "task-child-second", "run-child-second")

	_, _, err := orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID: "task-child-first",
		RunID:  "run-child-first",
		Status: results.StatusSucceeded,
	})
	if err != nil {
		t.Fatalf("apply first child callback: %v", err)
	}

	parent, err := taskRepo.GetByTaskID(ctx, "task-parent-multi")
	if err != nil {
		t.Fatalf("get parent after first child: %v", err)
	}
	if parent.Status != model.TaskStatusWaitingUpstream {
		t.Fatalf("expected parent to keep waiting_upstream until all dependencies resolve, got %s", parent.Status)
	}

	dependencies, err := dependencyRepo.ListByParentTaskID(ctx, "task-parent-multi")
	if err != nil {
		t.Fatalf("list dependencies after first child: %v", err)
	}
	assertDependencyStatus(t, dependencies, "task-child-first", model.DependencyStatusResolved)
	assertDependencyStatus(t, dependencies, "task-child-second", model.DependencyStatusPending)

	_, _, err = orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID: "task-child-second",
		RunID:  "run-child-second",
		Status: results.StatusSucceeded,
	})
	if err != nil {
		t.Fatalf("apply second child callback: %v", err)
	}

	parent, err = taskRepo.GetByTaskID(ctx, "task-parent-multi")
	if err != nil {
		t.Fatalf("get parent after second child: %v", err)
	}
	if parent.Status != model.TaskStatusPending {
		t.Fatalf("expected parent restored to pending after all dependencies resolve, got %s", parent.Status)
	}
}

func TestApplyCanonicalResultReplayDoesNotDowngradeResolvedDependencyOrRewaitParent(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, dependencyRepo := newCallbackNextTaskRepositories(t)
	orch := New(taskRepo, runRepo, WithDependencyRepository(dependencyRepo))
	createCallbackNextTaskFixture(t, ctx, taskRepo, runRepo, "task-parent-replay", "run-parent-replay")

	payload := results.CanonicalResult{
		TaskID: "task-parent-replay",
		RunID:  "run-parent-replay",
		Status: results.StatusSucceeded,
		NextTasks: []results.NextTaskIntent{
			{
				DedupeKey: "next:task-parent-replay:framework",
				Role:      "DeveloperWorker",
				Type:      "upstream_fix",
				Objective: "fix framework validator",
			},
		},
	}
	if _, _, err := orch.ApplyCanonicalResult(ctx, payload); err != nil {
		t.Fatalf("first parent callback: %v", err)
	}

	child, err := taskRepo.GetByDedupeKey(ctx, "next:task-parent-replay:framework")
	if err != nil {
		t.Fatalf("get child: %v", err)
	}
	attachRunningRun(t, ctx, taskRepo, runRepo, child, "run-child-replay")

	if _, _, err := orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID: child.TaskID,
		RunID:  "run-child-replay",
		Status: results.StatusSucceeded,
	}); err != nil {
		t.Fatalf("child callback: %v", err)
	}

	if _, _, err := orch.ApplyCanonicalResult(ctx, payload); err != nil {
		t.Fatalf("replayed parent callback should be idempotent: %v", err)
	}

	parent, err := taskRepo.GetByTaskID(ctx, "task-parent-replay")
	if err != nil {
		t.Fatalf("get parent: %v", err)
	}
	if parent.Status != model.TaskStatusPending {
		t.Fatalf("expected replay to keep parent pending, got %s", parent.Status)
	}

	dependencies, err := dependencyRepo.ListByParentTaskID(ctx, "task-parent-replay")
	if err != nil {
		t.Fatalf("list dependencies: %v", err)
	}
	if len(dependencies) != 1 {
		t.Fatalf("expected one dependency, got %d", len(dependencies))
	}
	if dependencies[0].Status != model.DependencyStatusResolved {
		t.Fatalf("expected replay to keep dependency resolved, got %s", dependencies[0].Status)
	}
}

func TestApplyCanonicalResultBlocksDependencyAndKeepsParentWaitingWhenChildFails(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, dependencyRepo := newCallbackNextTaskRepositories(t)
	orch := New(taskRepo, runRepo, WithDependencyRepository(dependencyRepo))
	createWaitingParentAndChildFixture(t, ctx, taskRepo, runRepo, dependencyRepo, "task-parent-blocked", "task-child-blocked", "run-child-blocked")

	_, _, err := orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID: "task-child-blocked",
		RunID:  "run-child-blocked",
		Status: results.StatusFailed,
	})
	if err != nil {
		t.Fatalf("apply child callback: %v", err)
	}

	parent, err := taskRepo.GetByTaskID(ctx, "task-parent-blocked")
	if err != nil {
		t.Fatalf("get parent: %v", err)
	}
	if parent.Status != model.TaskStatusWaitingUpstream {
		t.Fatalf("expected parent to keep waiting_upstream, got %s", parent.Status)
	}

	dependencies, err := dependencyRepo.ListByChildTaskID(ctx, "task-child-blocked")
	if err != nil {
		t.Fatalf("list dependencies: %v", err)
	}
	if len(dependencies) != 1 {
		t.Fatalf("expected one dependency, got %d", len(dependencies))
	}
	if dependencies[0].Status != model.DependencyStatusBlocked {
		t.Fatalf("expected dependency blocked, got %s", dependencies[0].Status)
	}
}

func newCallbackNextTaskRepositories(t *testing.T) (*repository.TaskRepository, *repository.RunRepository, *repository.DependencyRepository) {
	t.Helper()

	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Dependency{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return repository.NewTaskRepository(db), repository.NewRunRepository(db), repository.NewDependencyRepository(db)
}

func createCallbackNextTaskFixture(
	t *testing.T,
	ctx context.Context,
	taskRepo *repository.TaskRepository,
	runRepo *repository.RunRepository,
	taskID string,
	runID string,
) {
	t.Helper()

	currentRunID := runID
	task := &model.Task{
		TaskID:       taskID,
		DedupeKey:    "dedupe-" + taskID,
		Type:         "bugfix",
		Role:         "TesterWorker",
		Status:       model.TaskStatusRunning,
		CurrentRunID: &currentRunID,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create parent task: %v", err)
	}
	run := &model.Run{
		RunID:      runID,
		TaskID:     taskID,
		WorkerRole: "TesterWorker",
		Status:     model.RunStatusRunning,
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("create parent run: %v", err)
	}
}

func createWaitingParentAndChildFixture(
	t *testing.T,
	ctx context.Context,
	taskRepo *repository.TaskRepository,
	runRepo *repository.RunRepository,
	dependencyRepo *repository.DependencyRepository,
	parentTaskID string,
	childTaskID string,
	childRunID string,
) {
	t.Helper()

	parent := &model.Task{
		TaskID:    parentTaskID,
		DedupeKey: "dedupe-" + parentTaskID,
		Type:      "test_from_issue",
		Role:      "TesterWorker",
		Status:    model.TaskStatusWaitingUpstream,
	}
	if err := taskRepo.Create(ctx, parent); err != nil {
		t.Fatalf("create waiting parent: %v", err)
	}

	currentRunID := childRunID
	child := &model.Task{
		TaskID:       childTaskID,
		DedupeKey:    "dedupe-" + childTaskID,
		Type:         "upstream_fix",
		Role:         "DeveloperWorker",
		Status:       model.TaskStatusRunning,
		CurrentRunID: &currentRunID,
	}
	if err := taskRepo.Create(ctx, child); err != nil {
		t.Fatalf("create child task: %v", err)
	}

	childRun := &model.Run{
		RunID:      childRunID,
		TaskID:     childTaskID,
		WorkerRole: "DeveloperWorker",
		Status:     model.RunStatusRunning,
	}
	if err := runRepo.Create(ctx, childRun); err != nil {
		t.Fatalf("create child run: %v", err)
	}

	if err := dependencyRepo.Create(ctx, &model.Dependency{
		ParentTaskID: parentTaskID,
		ChildTaskID:  childTaskID,
		Relation:     "upstream_fix",
		Status:       model.DependencyStatusPending,
	}); err != nil {
		t.Fatalf("create dependency: %v", err)
	}
}

func createRunningChildWithDependencyFixture(
	t *testing.T,
	ctx context.Context,
	taskRepo *repository.TaskRepository,
	runRepo *repository.RunRepository,
	dependencyRepo *repository.DependencyRepository,
	parentTaskID string,
	childTaskID string,
	childRunID string,
) {
	t.Helper()

	currentRunID := childRunID
	child := &model.Task{
		TaskID:       childTaskID,
		DedupeKey:    "dedupe-" + childTaskID,
		Type:         "upstream_fix",
		Role:         "DeveloperWorker",
		Status:       model.TaskStatusRunning,
		CurrentRunID: &currentRunID,
	}
	if err := taskRepo.Create(ctx, child); err != nil {
		t.Fatalf("create child task: %v", err)
	}

	childRun := &model.Run{
		RunID:      childRunID,
		TaskID:     childTaskID,
		WorkerRole: "DeveloperWorker",
		Status:     model.RunStatusRunning,
	}
	if err := runRepo.Create(ctx, childRun); err != nil {
		t.Fatalf("create child run: %v", err)
	}

	if err := dependencyRepo.Create(ctx, &model.Dependency{
		ParentTaskID: parentTaskID,
		ChildTaskID:  childTaskID,
		Relation:     "upstream_fix",
		Status:       model.DependencyStatusPending,
	}); err != nil {
		t.Fatalf("create dependency: %v", err)
	}
}

func attachRunningRun(
	t *testing.T,
	ctx context.Context,
	taskRepo *repository.TaskRepository,
	runRepo *repository.RunRepository,
	task *model.Task,
	runID string,
) {
	t.Helper()

	task.Status = model.TaskStatusRunning
	task.CurrentRunID = &runID
	if err := taskRepo.Update(ctx, task); err != nil {
		t.Fatalf("update child task: %v", err)
	}

	run := &model.Run{
		RunID:      runID,
		TaskID:     task.TaskID,
		WorkerRole: task.Role,
		Status:     model.RunStatusRunning,
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("create child run: %v", err)
	}
}

func assertDependencyStatus(t *testing.T, dependencies []model.Dependency, childTaskID string, expected model.DependencyStatus) {
	t.Helper()

	for _, dependency := range dependencies {
		if dependency.ChildTaskID == childTaskID {
			if dependency.Status != expected {
				t.Fatalf("expected dependency for child %s to be %s, got %s", childTaskID, expected, dependency.Status)
			}
			return
		}
	}
	t.Fatalf("dependency for child %s not found", childTaskID)
}
