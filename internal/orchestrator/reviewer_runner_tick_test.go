package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store/repository"
)

func TestTickDispatchesReviewerAndDeveloperToRunnerAndMovesTesterToHumanRequired(t *testing.T) {
	ctx := context.Background()
	tasks, runs := newTickRepositories(t)
	reviewer := &fakeReviewerRunnerCoordinator{}
	developer := &fakeDeveloperRunnerCoordinator{}
	orch := New(
		tasks,
		runs,
		WithReviewerRunner(reviewer, ReviewerRunnerSettings{
			WorkspaceRoot:     "/app/workspaces",
			Provider:          runner.ProviderConfig{Name: "claude-cli", Revision: 4, Model: "model-a", APIKey: "secret"},
			ResolveRepository: stubRunnerRepository(),
		}),
		WithDeveloperRunner(developer, ReviewerRunnerSettings{
			WorkspaceRoot:     "/app/workspaces",
			Provider:          runner.ProviderConfig{Name: "claude-cli", Revision: 4, Model: "model-a", APIKey: "secret"},
			ResolveRepository: stubRunnerRepository(),
		}),
	)
	reviewerTask := reviewerRunnerTickTask("task-tick-runner-reviewer", model.TaskStatusPending)
	if err := tasks.Create(ctx, reviewerTask); err != nil {
		t.Fatalf("create reviewer task: %v", err)
	}
	if err := tasks.Create(ctx, &model.Task{
		TaskID: "task-tick-runner-developer", DedupeKey: "tick-runner-developer", Type: "develop_from_issue", Role: "DeveloperWorker",
		SourceRepo: "group/project", TargetRepo: "group/project", RepositoryID: reviewerTask.RepositoryID, Status: model.TaskStatusPending, Objective: "implement change", AcceptanceCriteria: "tests pass",
		DeliveryBranch: "agent/task-tick-runner-developer",
		ContextJSON:    `{"project":{"path":"group/project"},"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"master","access":"writable"}]}`,
	}); err != nil {
		t.Fatalf("create developer task: %v", err)
	}
	if err := tasks.Create(ctx, &model.Task{
		TaskID: "task-tick-legacy-tester", DedupeKey: "tick-legacy-tester", Type: "test", Role: "TesterWorker",
		RepositoryID: reviewerTask.RepositoryID,
		Status:       model.TaskStatusPending, Objective: "test change", AcceptanceCriteria: "tests pass",
	}); err != nil {
		t.Fatalf("create tester task: %v", err)
	}

	result, err := orch.Tick(ctx, TickRequest{DispatchLimit: 10})
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(result.DispatchedTasks) != 2 || reviewer.startCalls != 1 || developer.startCalls != 1 {
		t.Fatalf("unexpected mixed dispatch: result=%#v reviewer_starts=%d developer_starts=%d", result.DispatchedTasks, reviewer.startCalls, developer.startCalls)
	}
	if reviewer.startRequest.Task.TaskID != reviewerTask.TaskID || reviewer.startRequest.Workspace.WorkspaceRoot != "/app/workspaces" {
		t.Fatalf("unexpected reviewer runner request: %#v", reviewer.startRequest)
	}
	if reviewer.startRequest.Provider.Model != "model-a" || reviewer.startRequest.Provider.APIKey != "secret" || reviewer.startRequest.Workspace.Credential.Token != "git-token" {
		t.Fatalf("reviewer fixed configuration was not propagated: %#v", reviewer.startRequest)
	}
	if developer.startRequest.Task.Role != "DeveloperWorker" || developer.startRequest.Workspace.WorkspaceRoot != "/app/workspaces" || developer.startRequest.Workspace.DeliveryBranch != "agent/task-tick-runner-developer" {
		t.Fatalf("unexpected developer runner request: %#v", developer.startRequest)
	}
	tester, err := tasks.GetByTaskID(ctx, "task-tick-legacy-tester")
	if err != nil {
		t.Fatalf("get tester task: %v", err)
	}
	if tester.Status != model.TaskStatusHumanRequired || tester.CurrentRunID != nil || tester.LastError != unsupportedTesterWorkerReason {
		t.Fatalf("tester was not moved to stable human handling: %#v", tester)
	}
	repeated, err := orch.Tick(ctx, TickRequest{DispatchLimit: 10})
	if err != nil {
		t.Fatalf("repeat tick: %v", err)
	}
	if len(repeated.DispatchedTasks) != 0 || len(repeated.DispatchFailures) != 0 {
		t.Fatalf("repeat tick retried unsupported tester: %#v", repeated)
	}
	reviewerRun, err := runs.GetByRunID(ctx, reviewer.startRequest.Run.RunID)
	if err != nil || reviewerRun.ExternalSystem != agentRunnerExternalSystem || reviewerRun.ExternalID != reviewerRun.RunID || reviewerRun.InputJSON == "" {
		t.Fatalf("unexpected reviewer run identity: run=%#v err=%v", reviewerRun, err)
	}
}

func TestTickDoesNotPollLegacyAgentServerRuns(t *testing.T) {
	ctx := context.Background()
	tasks, runs := newTickRepositories(t)
	orch := New(tasks, runs)
	runID := "run-legacy-agent"
	leaseUntil := orch.now().Add(time.Hour)
	if err := tasks.Create(ctx, &model.Task{
		TaskID: "task-legacy-agent", DedupeKey: "task-legacy-agent", Type: "test", Role: "TesterWorker",
		Status: model.TaskStatusRunning, CurrentRunID: &runID, LeaseUntil: &leaseUntil,
	}); err != nil {
		t.Fatalf("create legacy task: %v", err)
	}
	if err := runs.Create(ctx, &model.Run{
		RunID: runID, TaskID: "task-legacy-agent", WorkerRole: "TesterWorker", Status: model.RunStatusRunning,
		ExternalSystem: "legacy-agent-server", ExternalID: "conversation-legacy",
	}); err != nil {
		t.Fatalf("create legacy run: %v", err)
	}

	result, err := orch.Tick(ctx, TickRequest{PollLimit: 10})
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(result.PolledRuns) != 0 {
		t.Fatalf("legacy agent-server run was polled: result=%#v", result.PolledRuns)
	}
	stored, err := runs.GetByRunID(ctx, runID)
	if err != nil || stored.Status != model.RunStatusRunning {
		t.Fatalf("legacy run should remain historical running data: run=%#v err=%v", stored, err)
	}
}

func TestTickReconcilesRunnerRunAndRenewsTaskLease(t *testing.T) {
	ctx := context.Background()
	tasks, runs := newTickRepositories(t)
	reviewer := &fakeReviewerRunnerCoordinator{reconcileResult: &ReviewerRunnerReconcileResult{
		Execution: &model.AgentExecution{RunID: "run-tick-runner-active", ExecutionID: "run-tick-runner-active", Status: model.AgentExecutionStatusRunning},
	}}
	orch := New(tasks, runs, WithReviewerRunner(reviewer, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}))
	runID := "run-tick-runner-active"
	task := reviewerRunnerTickTask("task-tick-runner-active", model.TaskStatusRunning)
	task.CurrentRunID = &runID
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := runs.Create(ctx, &model.Run{
		RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning,
		ExternalSystem: agentRunnerExternalSystem, ExternalID: runID,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	result, err := orch.Tick(ctx, TickRequest{PollLimit: 10})
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if reviewer.reconcileCalls != 1 || len(result.PolledRuns) != 1 || result.PolledRuns[0].ExecutionStatus != string(model.AgentExecutionStatusRunning) {
		t.Fatalf("unexpected runner reconciliation: calls=%d result=%#v", reviewer.reconcileCalls, result.PolledRuns)
	}
	stored, err := tasks.GetByTaskID(ctx, task.TaskID)
	if err != nil || stored.LeaseUntil == nil {
		t.Fatalf("active runner task lease was not renewed: task=%#v err=%v", stored, err)
	}
}

func TestTickRebuildsMissingReviewerExecutionProjectionBeforeReconcile(t *testing.T) {
	ctx := context.Background()
	tasks, runs := newTickRepositories(t)
	reviewer := &fakeReviewerRunnerCoordinator{
		reconcileErrors: []error{repository.ErrNotFound, nil},
		reconcileResult: &ReviewerRunnerReconcileResult{
			Execution: &model.AgentExecution{RunID: "run-tick-runner-repair", ExecutionID: "run-tick-runner-repair", Status: model.AgentExecutionStatusRunning},
		},
	}
	orch := New(
		tasks,
		runs,
		WithReviewerRunner(reviewer, ReviewerRunnerSettings{
			WorkspaceRoot:     "/app/workspaces",
			Provider:          runner.ProviderConfig{Name: "claude-cli", Revision: 5, Model: "model-a"},
			ResolveRepository: stubRunnerRepository(),
		}),
	)
	runID := "run-tick-runner-repair"
	task := reviewerRunnerTickTask("task-tick-runner-repair", model.TaskStatusRunning)
	task.CurrentRunID = &runID
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := runs.Create(ctx, &model.Run{
		RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning,
		ExternalSystem: agentRunnerExternalSystem, ExternalID: runID,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	result, err := orch.Tick(ctx, TickRequest{PollLimit: 10})
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if reviewer.startCalls != 1 || reviewer.reconcileCalls != 2 {
		t.Fatalf("missing projection was not rebuilt: start=%d reconcile=%d", reviewer.startCalls, reviewer.reconcileCalls)
	}
	if reviewer.startRequest.Run.RunID != runID || reviewer.startRequest.Task.TaskID != task.TaskID || reviewer.startRequest.Workspace.WorkspaceRoot != "/app/workspaces" {
		t.Fatalf("unexpected repair request: %#v", reviewer.startRequest)
	}
	if len(result.PolledRuns) != 1 || result.PolledRuns[0].Error != "" || result.PolledRuns[0].ExecutionStatus != string(model.AgentExecutionStatusRunning) {
		t.Fatalf("unexpected repaired reconciliation: %#v", result.PolledRuns)
	}
}

func TestTickMovesFailedRunnerExecutionThroughExistingRetryPolicy(t *testing.T) {
	ctx := context.Background()
	tasks, runs := newTickRepositories(t)
	reviewer := &fakeReviewerRunnerCoordinator{reconcileResult: &ReviewerRunnerReconcileResult{
		Execution: &model.AgentExecution{
			RunID: "run-tick-runner-failed", ExecutionID: "run-tick-runner-failed",
			Status: model.AgentExecutionStatusFailed, ErrorCode: "provider_failed",
		},
	}}
	orch := New(tasks, runs, WithReviewerRunner(reviewer, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}))
	runID := "run-tick-runner-failed"
	task := reviewerRunnerTickTask("task-tick-runner-failed", model.TaskStatusRunning)
	task.CurrentRunID = &runID
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := runs.Create(ctx, &model.Run{
		RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning,
		ExternalSystem: agentRunnerExternalSystem, ExternalID: runID,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	result, err := orch.Tick(ctx, TickRequest{PollLimit: 10})
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(result.PolledRuns) != 1 || !result.PolledRuns[0].Terminal || result.PolledRuns[0].Error != "provider_failed" {
		t.Fatalf("unexpected failed runner poll: %#v", result.PolledRuns)
	}
	storedTask, _ := tasks.GetByTaskID(ctx, task.TaskID)
	storedRun, _ := runs.GetByRunID(ctx, runID)
	if storedTask.Status != model.TaskStatusRunning || storedTask.CurrentRunID == nil || *storedTask.CurrentRunID == runID || reviewer.startCalls != 1 || storedRun.Status != model.RunStatusFailed || storedRun.ErrorMessage != "provider_failed" {
		t.Fatalf("runner failure bypassed existing policy: task=%#v run=%#v", storedTask, storedRun)
	}
}

func TestTickRetriesReviewerWorkspacePreparationFailureWithNewRun(t *testing.T) {
	ctx := context.Background()
	tasks, runs := newTickRepositories(t)
	reviewer := &fakeReviewerRunnerCoordinator{startErrors: []error{errors.New("git fetch failed"), nil}}
	orch := New(tasks, runs, WithReviewerRunner(reviewer, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}))
	task := reviewerRunnerTickTask("task-reviewer-workspace-retry", model.TaskStatusPending)
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create reviewer task: %v", err)
	}

	first, err := orch.Tick(ctx, TickRequest{DispatchLimit: 1})
	if err != nil {
		t.Fatalf("first tick must isolate workspace preparation failure: %v", err)
	}
	if len(first.DispatchFailures) != 1 || reviewer.startCalls != 1 {
		t.Fatalf("expected one failed reviewer dispatch: result=%#v starts=%d", first.DispatchFailures, reviewer.startCalls)
	}
	storedTask, err := tasks.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("get task after first failure: %v", err)
	}
	if storedTask.Status != model.TaskStatusRetryPending || storedTask.CurrentRunID == nil {
		t.Fatalf("workspace failure did not enter retry state: %#v", storedTask)
	}
	failedRunID := *storedTask.CurrentRunID
	failedRun, err := runs.GetByRunID(ctx, failedRunID)
	if err != nil || failedRun.Status != model.RunStatusFailed {
		t.Fatalf("failed reviewer run was not recorded: run=%#v err=%v", failedRun, err)
	}

	second, err := orch.Tick(ctx, TickRequest{DispatchLimit: 1})
	if err != nil {
		t.Fatalf("retry tick: %v", err)
	}
	if len(second.DispatchFailures) != 0 || len(second.DispatchedTasks) != 1 || reviewer.startCalls != 2 {
		t.Fatalf("retry did not create a new reviewer run: result=%#v starts=%d", second, reviewer.startCalls)
	}
	if second.DispatchedTasks[0].RunID == failedRunID {
		t.Fatalf("retry reused failed run %s", failedRunID)
	}
}

func TestTickBlocksSuccessfulRunnerWithInvalidBusinessResult(t *testing.T) {
	ctx := context.Background()
	tasks, runs := newTickRepositories(t)
	developer := &fakeDeveloperRunnerCoordinator{
		reconcileResult: &DeveloperRunnerReconcileResult{
			Execution: &model.AgentExecution{RunID: "run-tick-invalid-result", ExecutionID: "run-tick-invalid-result", Status: model.AgentExecutionStatusSucceeded},
		},
		reconcileErrors: []error{newInvalidRunnerResultError(errors.New("validate developer runner result: tests[4] must pass before change delivery"))},
	}
	orch := New(tasks, runs, WithDeveloperRunner(developer, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}))
	runID := "run-tick-invalid-result"
	task := &model.Task{
		TaskID: "task-tick-invalid-result", DedupeKey: "task-tick-invalid-result", Type: "develop_from_issue", Role: "DeveloperWorker",
		SourceRepo: "group/project", TargetRepo: "group/project", Status: model.TaskStatusRunning, CurrentRunID: &runID,
	}
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := runs.Create(ctx, &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning, ExternalSystem: agentRunnerExternalSystem, ExternalID: runID}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	result, err := orch.Tick(ctx, TickRequest{PollLimit: 10})
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(result.PolledRuns) != 1 || result.PolledRuns[0].Error == "" {
		t.Fatalf("expected invalid result to be reported: %#v", result.PolledRuns)
	}
	storedTask, _ := tasks.GetByTaskID(ctx, task.TaskID)
	storedRun, _ := runs.GetByRunID(ctx, runID)
	if storedTask.Status != model.TaskStatusHumanRequired || storedRun.Status != model.RunStatusBlocked || storedRun.ErrorMessage == "" {
		t.Fatalf("successful execution with invalid result did not converge: task=%#v run=%#v", storedTask, storedRun)
	}
}

type fakeReviewerRunnerCoordinator struct {
	startCalls      int
	startRequest    ReviewerRunnerStartRequest
	startErrors     []error
	reconcileCalls  int
	reconcileErrors []error
	reconcileResult *ReviewerRunnerReconcileResult
}

type fakeDeveloperRunnerCoordinator struct {
	startCalls      int
	startRequest    DeveloperRunnerStartRequest
	reconcileCalls  int
	reconcileErrors []error
	reconcileResult *DeveloperRunnerReconcileResult
}

func (f *fakeDeveloperRunnerCoordinator) Start(_ context.Context, request DeveloperRunnerStartRequest) (*model.AgentExecution, error) {
	f.startCalls++
	f.startRequest = request
	return &model.AgentExecution{RunID: request.Run.RunID, ExecutionID: request.Run.RunID, Provider: request.Provider.Name, Status: model.AgentExecutionStatusAccepted, RequestHash: "request-hash"}, nil
}

func (f *fakeDeveloperRunnerCoordinator) Reconcile(context.Context, string) (*DeveloperRunnerReconcileResult, error) {
	f.reconcileCalls++
	if len(f.reconcileErrors) >= f.reconcileCalls && f.reconcileErrors[f.reconcileCalls-1] != nil {
		return f.reconcileResult, f.reconcileErrors[f.reconcileCalls-1]
	}
	return f.reconcileResult, nil
}

func (f *fakeReviewerRunnerCoordinator) Start(_ context.Context, request ReviewerRunnerStartRequest) (*model.AgentExecution, error) {
	f.startCalls++
	f.startRequest = request
	if len(f.startErrors) >= f.startCalls && f.startErrors[f.startCalls-1] != nil {
		return nil, f.startErrors[f.startCalls-1]
	}
	return &model.AgentExecution{
		RunID: request.Run.RunID, ExecutionID: request.Run.RunID, Provider: request.Provider.Name,
		ProviderConfigRevision: request.Provider.Revision, Status: model.AgentExecutionStatusAccepted, RequestHash: "request-hash",
	}, nil
}

func (f *fakeReviewerRunnerCoordinator) Reconcile(context.Context, string) (*ReviewerRunnerReconcileResult, error) {
	f.reconcileCalls++
	if len(f.reconcileErrors) >= f.reconcileCalls && f.reconcileErrors[f.reconcileCalls-1] != nil {
		return nil, f.reconcileErrors[f.reconcileCalls-1]
	}
	return f.reconcileResult, nil
}

func reviewerRunnerTickTask(taskID string, status model.TaskStatus) *model.Task {
	repositoryID := fixtureRepositoryID
	return &model.Task{
		TaskID: taskID, DedupeKey: taskID, Type: "review_mr", Role: "ReviewerWorker",
		SourceRepo: "group/project", TargetRepo: "group/project", RepositoryID: &repositoryID, Status: status,
		Objective: "review MR", AcceptanceCriteria: "return verdict",
		ContextJSON: `{"project":{"path":"group/project"},"merge_request":{"iid":21,"url":"https://gitlab.example/group/project/-/merge_requests/21","review_mode":"full","source_branch":"feature/review","target_branch":"master","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"feature/review","access":"readable"}]}`,
	}
}
