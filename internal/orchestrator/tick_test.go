package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

type fakeTickBackend struct {
	conversationStatuses map[string]string
	conversationErrors   map[string]error
	finalResponses       map[string]string
	createErrors         map[int]error
	nextConversationID   int
	calls                []string
}

func TestTickSkipsNonAgentRunnerRuns(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo := newTickRepositories(t)
	developer := &fakeDeveloperRunnerCoordinator{}
	orch := New(taskRepo, runRepo, WithDeveloperRunner(developer, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}))

	for _, fixture := range []struct {
		taskID         string
		runID          string
		conversationID string
	}{
		{taskID: "task-poll-bad", runID: "run-poll-bad", conversationID: "conversation-bad"},
		{taskID: "task-poll-good", runID: "run-poll-good", conversationID: "conversation-good"},
	} {
		currentRunID := fixture.runID
		if err := taskRepo.Create(ctx, &model.Task{
			TaskID: fixture.taskID, DedupeKey: "dedupe-" + fixture.taskID, Type: "bugfix",
			Role: "DeveloperWorker", Status: model.TaskStatusRunning, CurrentRunID: &currentRunID,
		}); err != nil {
			t.Fatalf("create task %s: %v", fixture.taskID, err)
		}
		if err := runRepo.Create(ctx, &model.Run{
			RunID: fixture.runID, TaskID: fixture.taskID, WorkerRole: "DeveloperWorker",
			Status: model.RunStatusRunning, ExternalSystem: "legacy-agent-server", ExternalID: fixture.conversationID,
		}); err != nil {
			t.Fatalf("create run %s: %v", fixture.runID, err)
		}
	}

	result, err := orch.Tick(ctx, TickRequest{PollLimit: 10})
	if err != nil {
		t.Fatalf("tick must isolate one poll failure: %v", err)
	}
	if len(result.PolledRuns) != 0 || developer.reconcileCalls != 0 {
		t.Fatalf("non-agent-runner runs must not be polled: results=%#v reconciles=%d", result.PolledRuns, developer.reconcileCalls)
	}
}

func TestTickMovesUnsupportedTasksToHumanRequiredAndStillExecutesEffects(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, effectRepo := newTickEffectRepositories(t)
	executor := &fakeEffectExecutor{responses: map[string]string{"effect-after-dispatch-failure": `{"message_id":"msg-1"}`}}
	orch := New(
		taskRepo,
		runRepo,
		WithEffectRepository(effectRepo),
		WithEffectExecutor(executor),
	)

	repositoryID := fixtureRepositoryID
	for _, taskID := range []string{"task-dispatch-bad", "task-dispatch-good"} {
		if err := taskRepo.Create(ctx, &model.Task{
			TaskID: taskID, DedupeKey: "dedupe-" + taskID, Type: "bugfix", RepositoryID: &repositoryID,
			Role: "TesterWorker", Status: model.TaskStatusPending,
		}); err != nil {
			t.Fatalf("create task %s: %v", taskID, err)
		}
	}
	if err := effectRepo.Create(ctx, &model.Effect{
		TaskID: "task-effect", RunID: "run-effect", DedupeKey: "effect-after-dispatch-failure",
		Kind: "send_im_message", Target: "team-review-group", Status: model.EffectStatusPending,
		RequestJSON: `{"body":"dispatch failure isolated"}`,
	}); err != nil {
		t.Fatalf("create effect: %v", err)
	}

	result, err := orch.Tick(ctx, TickRequest{
		DispatchLimit: 10,
		EffectLimit:   10,
	})
	if err != nil {
		t.Fatalf("tick must isolate one dispatch failure: %v", err)
	}
	if len(result.DispatchFailures) != 2 {
		t.Fatalf("expected both unsupported tasks to require human handling: %#v", result.DispatchFailures)
	}
	if len(result.DispatchedTasks) != 0 {
		t.Fatalf("unsupported tasks reached an execution backend: dispatched=%#v", result.DispatchedTasks)
	}
	// 预置 effect 1 条 + 两个 unsupported 任务各物化一条 human_required 通知,
	// 同轮全部执行成功——停车不停通知链。
	if len(result.ExecutedEffects) != 3 {
		t.Fatalf("expected three executed effects: %#v", result.ExecutedEffects)
	}
	for _, executed := range result.ExecutedEffects {
		if executed.Status != model.EffectStatusSucceeded {
			t.Fatalf("effect execution was skipped: %#v", result.ExecutedEffects)
		}
	}
}

func TestTickPollsRecoversThenDispatches(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo := newTickRepositories(t)
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	backend := &fakeTickBackend{
		conversationStatuses: map[string]string{
			"conversation-poll": "finished",
		},
		finalResponses: map[string]string{
			"conversation-poll": `{"task_id":"task-tick-poll","run_id":"run-tick-poll","status":"succeeded","summary":"poll done"}`,
		},
	}
	orch := New(
		taskRepo,
		runRepo,
		WithClock(func() time.Time { return now }),
		WithPolicy(Policy{LeaseDuration: time.Minute, MaxAttempts: 2}),
	)

	currentRunID := "run-tick-poll"
	if err := taskRepo.Create(ctx, &model.Task{
		TaskID:       "task-tick-poll",
		DedupeKey:    "dedupe-tick-poll",
		Type:         "bugfix",
		Role:         "DeveloperWorker",
		Status:       model.TaskStatusRunning,
		CurrentRunID: &currentRunID,
	}); err != nil {
		t.Fatalf("create poll task: %v", err)
	}
	if err := runRepo.Create(ctx, &model.Run{
		RunID:          "run-tick-poll",
		TaskID:         "task-tick-poll",
		WorkerRole:     "DeveloperWorker",
		Status:         model.RunStatusRunning,
		ExternalSystem: "legacy-agent-server",
		ExternalID:     "conversation-poll",
	}); err != nil {
		t.Fatalf("create poll run: %v", err)
	}

	repositoryID := fixtureRepositoryID
	if err := taskRepo.Create(ctx, &model.Task{
		TaskID:       "task-tick-recover",
		DedupeKey:    "dedupe-tick-recover",
		Type:         "bugfix",
		Role:         "TesterWorker",
		RepositoryID: &repositoryID,
		Status:       model.TaskStatusClaimed,
		LeaseUntil:   &expired,
		AttemptCount: 0,
	}); err != nil {
		t.Fatalf("create recover task: %v", err)
	}
	if err := taskRepo.Create(ctx, &model.Task{
		TaskID:       "task-tick-dispatch",
		DedupeKey:    "dedupe-tick-dispatch",
		Type:         "bugfix",
		Role:         "TesterWorker",
		RepositoryID: &repositoryID,
		Status:       model.TaskStatusPending,
	}); err != nil {
		t.Fatalf("create dispatch task: %v", err)
	}

	result, err := orch.Tick(ctx, TickRequest{
		PollLimit:     10,
		RecoveryLimit: 10,
		DispatchLimit: 10,
	})
	if err != nil {
		t.Fatalf("tick: %v", err)
	}

	if len(result.PolledRuns) != 0 || len(backend.calls) != 0 {
		t.Fatalf("legacy agent-server run was still active: results=%#v calls=%#v", result.PolledRuns, backend.calls)
	}
	if len(result.Recoveries) != 1 {
		t.Fatalf("expected one recovery, got %d", len(result.Recoveries))
	}
	if result.Recoveries[0].TaskID != "task-tick-recover" {
		t.Fatalf("expected task-tick-recover recovery, got %s", result.Recoveries[0].TaskID)
	}
	if len(result.DispatchedTasks) != 0 || len(result.DispatchFailures) != 2 {
		t.Fatalf("unsupported testers were not moved to human handling: dispatched=%#v failures=%#v", result.DispatchedTasks, result.DispatchFailures)
	}

	assertTaskStatus(t, ctx, taskRepo, "task-tick-poll", model.TaskStatusRunning)
	assertTaskStatus(t, ctx, taskRepo, "task-tick-recover", model.TaskStatusHumanRequired)
	assertTaskStatus(t, ctx, taskRepo, "task-tick-dispatch", model.TaskStatusHumanRequired)
}

func TestTickExecutesPendingEffectsWhenEffectLimitSet(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, effectRepo := newTickEffectRepositories(t)
	if err := effectRepo.Create(ctx, &model.Effect{
		TaskID:      "task-effect",
		RunID:       "run-effect",
		DedupeKey:   "effect-tick-1",
		Kind:        "send_im_message",
		Target:      "team-review-group",
		Status:      model.EffectStatusPending,
		RequestJSON: `{"body":"review completed"}`,
	}); err != nil {
		t.Fatalf("create effect: %v", err)
	}

	executor := &fakeEffectExecutor{responses: map[string]string{
		"effect-tick-1": `{"message_id":"msg-1"}`,
	}}
	orch := New(
		taskRepo,
		runRepo,
		WithEffectRepository(effectRepo),
		WithEffectExecutor(executor),
	)

	result, err := orch.Tick(ctx, TickRequest{
		EffectLimit: 10,
	})
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(result.ExecutedEffects) != 1 {
		t.Fatalf("expected one executed effect, got %d", len(result.ExecutedEffects))
	}
	if result.ExecutedEffects[0].DedupeKey != "effect-tick-1" {
		t.Fatalf("unexpected effect dedupe key: %s", result.ExecutedEffects[0].DedupeKey)
	}
	if result.ExecutedEffects[0].Status != model.EffectStatusSucceeded {
		t.Fatalf("expected succeeded effect result, got %s", result.ExecutedEffects[0].Status)
	}
}

func TestTickFinalizesAcceptedResultBeforeTaskRecoveryAndDispatch(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Artifact{}, &model.ResultSubmission{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	tasks := repository.NewTaskRepository(db)
	runs := repository.NewRunRepository(db)
	submissions := repository.NewResultSubmissionRepository(db)
	artifacts := &failOnceArtifactRepository{delegate: repository.NewArtifactRepository(db)}
	backend := &fakeTickBackend{}
	orch := New(
		tasks,
		runs,
		WithArtifactRepository(artifacts),
		WithResultSubmissionRepository(submissions),
	)
	createCallbackArtifactFixture(t, ctx, tasks, runs, "task-tick-finalize", "run-tick-finalize")
	request := SubmitResultRequest{
		Source: "mcp_tester",
		Canonical: results.CanonicalResult{
			TaskID: "task-tick-finalize", RunID: "run-tick-finalize", Status: results.StatusSucceeded,
			Summary: "tests passed", Artifacts: map[string]any{"test_results": "all passed"},
		},
	}
	if _, err := orch.SubmitResult(ctx, request); err == nil {
		t.Fatal("expected injected finalization failure")
	}

	result, err := orch.Tick(ctx, TickRequest{
		FinalizationLimit: 10,
		PollLimit:         10,
		RecoveryLimit:     10,
		DispatchLimit:     10,
	})
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(result.FinalizedResults) != 1 || !result.FinalizedResults[0].Finalized {
		t.Fatalf("expected one completed finalization, got %#v", result.FinalizedResults)
	}
	if len(result.Recoveries) != 0 || len(result.DispatchedTasks) != 0 {
		t.Fatalf("finalizing task must not recover or dispatch: recoveries=%#v dispatched=%#v", result.Recoveries, result.DispatchedTasks)
	}
	if len(backend.calls) != 0 {
		t.Fatalf("accepted result must not call the backend again, got %#v", backend.calls)
	}
	assertTaskStatus(t, ctx, tasks, "task-tick-finalize", model.TaskStatusSucceeded)
}

func TestTickReconcilesUnknownWritebackBeforeFinalizingSubmission(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Artifact{}, &model.Writeback{}, &model.ResultSubmission{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	tasks := repository.NewTaskRepository(db)
	runs := repository.NewRunRepository(db)
	artifacts := repository.NewArtifactRepository(db)
	writebacks := repository.NewWritebackRepository(db)
	submissions := repository.NewResultSubmissionRepository(db)
	backend := &fakeGitLabWritebackBackend{
		err: codehost.NewAmbiguousError(errors.New("response ended prematurely")),
		reconcileResult: codehost.ReconcileResult{
			State:       codehost.ReconcileStateFound,
			ExternalID:  "7",
			ExternalURL: "https://gitlab.example/group/framework/-/merge_requests/7",
			Response:    map[string]any{"iid": 7},
		},
	}
	orch := New(
		tasks,
		runs,
		WithArtifactRepository(artifacts),
		WithWritebackRepository(writebacks),
		WithResultSubmissionRepository(submissions),
		WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, backend)),
	)
	createCallbackArtifactFixture(t, ctx, tasks, runs, "task-tick-reconcile", "run-tick-reconcile")
	task, err := tasks.GetByTaskID(ctx, "task-tick-reconcile")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	task.TargetRepo = "group/framework"
	if err := tasks.Update(ctx, task); err != nil {
		t.Fatalf("update task target repo: %v", err)
	}
	request := SubmitResultRequest{
		Source: "mcp_developer",
		Canonical: results.CanonicalResult{
			TaskID: "task-tick-reconcile", RunID: "run-tick-reconcile", Status: results.StatusSucceeded,
			Writebacks: []results.CodeHostWritebackIntent{{
				DedupeKey:    "codehost:mr:framework:tick-reconcile",
				Kind:         CodeHostWritebackCreateMergeRequest,
				ProjectID:    "group/framework",
				Title:        "fix framework",
				SourceBranch: model.DeliveryBranchForTask("task-tick-reconcile"),
				TargetBranch: "main",
			}},
		},
	}
	if _, err := orch.SubmitResult(ctx, request); err == nil {
		t.Fatal("expected ambiguous writeback to interrupt synchronous finalization")
	}

	result, err := orch.Tick(ctx, TickRequest{
		ReconciliationLimit: 10,
		FinalizationLimit:   10,
		RecoveryLimit:       10,
		DispatchLimit:       10,
	})
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(result.ReconciledWritebacks) != 1 || result.ReconciledWritebacks[0].State != codehost.ReconcileStateFound {
		t.Fatalf("expected one found reconciliation, got %#v", result.ReconciledWritebacks)
	}
	if len(result.FinalizedResults) != 1 || !result.FinalizedResults[0].Finalized {
		t.Fatalf("expected finalization after reconciliation, got %#v", result.FinalizedResults)
	}
	if backend.createMergeRequestCalls != 1 {
		t.Fatalf("expected exactly one remote create attempt, got %d", backend.createMergeRequestCalls)
	}
	assertTaskStatus(t, ctx, tasks, "task-tick-reconcile", model.TaskStatusSucceeded)
}

func newTickRepositories(t *testing.T) (*repository.TaskRepository, *repository.RunRepository) {
	t.Helper()

	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return repository.NewTaskRepository(db), repository.NewRunRepository(db)
}

func newTickEffectRepositories(t *testing.T) (*repository.TaskRepository, *repository.RunRepository, *repository.EffectRepository) {
	t.Helper()

	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Effect{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return repository.NewTaskRepository(db), repository.NewRunRepository(db), repository.NewEffectRepository(db)
}

type fakeRepositoryLookup struct{}

func (fakeRepositoryLookup) Get(_ context.Context, repositoryID string) (*registeredrepo.RegisteredRepository, error) {
	if repositoryID == "repo-bound" {
		return &registeredrepo.RegisteredRepository{RepositoryID: "repo-bound", Status: registeredrepo.StatusEnabled}, nil
	}
	if repositoryID == "repo-disabled" {
		return &registeredrepo.RegisteredRepository{RepositoryID: "repo-disabled", Status: registeredrepo.StatusDisabled}, nil
	}
	return nil, repository.ErrNotFound
}

func TestTickParksUnboundTaskAndDispatchesBoundTask(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo := newTickRepositories(t)
	orch := New(taskRepo, runRepo,
		WithDeveloperRunner(&fakeDeveloperRunnerCoordinator{}, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}),

		WithRepositoryLookup(fakeRepositoryLookup{}),
	)

	unbound := &model.Task{
		TaskID: "task-unbound", DedupeKey: "dedupe-unbound", Type: "bugfix", Role: "DeveloperWorker",
		Status: model.TaskStatusPending,
	}
	if err := taskRepo.Create(ctx, unbound); err != nil {
		t.Fatalf("create unbound task: %v", err)
	}
	repositoryID := "repo-bound"
	bound := &model.Task{
		TaskID: "task-bound", DedupeKey: "dedupe-bound", Type: "bugfix", Role: "DeveloperWorker",
		SourceRepo: "group/project", TargetRepo: "group/project",
		RepositoryID: &repositoryID, Status: model.TaskStatusPending,
		ContextJSON: `{"project":{"path":"group/project"},"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"master","access":"writable"}]}`,
	}
	if err := taskRepo.Create(ctx, bound); err != nil {
		t.Fatalf("create bound task: %v", err)
	}

	if _, err := orch.Tick(ctx, TickRequest{CallbackBaseURL: "http://coordinator.local", DispatchLimit: 1}); err != nil {
		t.Fatalf("first tick: %v", err)
	}
	unbound, err := taskRepo.GetByTaskID(ctx, "task-unbound")
	if err != nil {
		t.Fatalf("get unbound task: %v", err)
	}
	if unbound.Status != model.TaskStatusBlocked || unbound.LastError == "" {
		t.Fatalf("expected unbound task parked as blocked with last_error, got status=%s last_error=%q", unbound.Status, unbound.LastError)
	}

	result, err := orch.Tick(ctx, TickRequest{CallbackBaseURL: "http://coordinator.local", DispatchLimit: 1})
	if err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if len(result.DispatchedTasks) != 1 || result.DispatchedTasks[0].TaskID != "task-bound" {
		t.Fatalf("expected bound task dispatched after unbound parked, got %#v", result.DispatchedTasks)
	}
}

// 禁用仓库的任务不得派发也不得 park：任务保持 Pending，等仓库重新启用后自然恢复。
func TestTickSkipsDispatchForDisabledRepository(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo := newTickRepositories(t)
	orch := New(taskRepo, runRepo,
		WithDeveloperRunner(&fakeDeveloperRunnerCoordinator{}, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}),

		WithRepositoryLookup(fakeRepositoryLookup{}),
	)

	repositoryID := "repo-disabled"
	disabled := &model.Task{
		TaskID: "task-disabled-repo", DedupeKey: "dedupe-disabled", Type: "bugfix", Role: "DeveloperWorker",
		SourceRepo: "group/project", TargetRepo: "group/project",
		RepositoryID: &repositoryID, Status: model.TaskStatusPending,
		ContextJSON: `{"project":{"path":"group/project"},"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"master","access":"writable"}]}`,
	}
	if err := taskRepo.Create(ctx, disabled); err != nil {
		t.Fatalf("create disabled-repo task: %v", err)
	}

	result, err := orch.Tick(ctx, TickRequest{CallbackBaseURL: "http://coordinator.local", DispatchLimit: 1})
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(result.DispatchedTasks) != 0 {
		t.Fatalf("expected no dispatch for disabled repository, got %#v", result.DispatchedTasks)
	}
	stored, err := taskRepo.GetByTaskID(ctx, "task-disabled-repo")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if stored.Status != model.TaskStatusPending {
		t.Fatalf("expected task to stay pending when repository disabled, got %s", stored.Status)
	}
}
