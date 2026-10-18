package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/runner"
)

func testerVerificationTickTask(taskID string, status model.TaskStatus) *model.Task {
	repositoryID := fixtureRepositoryID
	return &model.Task{
		TaskID: taskID, DedupeKey: "verify:group/project:21:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Type: "verify_merge_request", Role: "TesterWorker",
		SourceRepo: "group/project", TargetRepo: "group/project", RepositoryID: &repositoryID, Status: status,
		Objective: "Verify MR !21 at head aaaa", AcceptanceCriteria: "report real evidence",
		ContextJSON: `{"project":{"path":"group/project"},"merge_request":{"iid":21,"url":"https://gitlab.example/group/project/-/merge_requests/21","source_branch":"feature/change","target_branch":"master","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"feature/change","access":"readable"}]}`,
	}
}

// verifyCommandRepository 是带验证命令下限的注册仓行桩。
func verifyCommandRepository(command string) func(context.Context, *model.Task) (*registeredrepo.RegisteredRepository, error) {
	return func(context.Context, *model.Task) (*registeredrepo.RegisteredRepository, error) {
		return &registeredrepo.RegisteredRepository{
			RepositoryID: fixtureRepositoryID, Provider: "gitlab",
			BaseURL: "https://gitlab.example", PathWithNamespace: "group/project", Token: "git-token",
			VerifyCommand: command,
		}, nil
	}
}

func TestTickDispatchesTesterTaskToRunner(t *testing.T) {
	ctx := context.Background()
	tasks, runs := newTickRepositories(t)
	tester := &fakeTesterRunnerCoordinator{}
	orch := New(tasks, runs, WithTesterRunner(tester, ReviewerRunnerSettings{
		Provider:          runner.ProviderConfig{Name: "claude-cli", Revision: 4, Model: "model-a", APIKey: "secret"},
		ResolveRepository: verifyCommandRepository("go test ./..."),
	}))
	task := testerVerificationTickTask("task-tick-runner-tester", model.TaskStatusPending)
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create tester task: %v", err)
	}

	result, err := orch.Tick(ctx, TickRequest{DispatchLimit: 10})
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if tester.startCalls != 1 || len(result.DispatchedTasks) != 1 {
		t.Fatalf("tester task was not dispatched: starts=%d result=%#v", tester.startCalls, result.DispatchedTasks)
	}
	if tester.startRequest.Task.TaskID != task.TaskID || tester.startRequest.Task.Role != "TesterWorker" {
		t.Fatalf("unexpected tester start request: %#v", tester.startRequest)
	}
	if tester.startRequest.Workspace.WorkspaceRoot != "" || tester.startRequest.Workspace.HeadSHA != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("workspace must be pinned to the frozen head sha: %#v", tester.startRequest.Workspace)
	}
	if tester.startRequest.Workspace.WorkerRole != "TesterWorker" || tester.startRequest.Workspace.Credential.Token != "git-token" {
		t.Fatalf("workspace role or credentials missing: %#v", tester.startRequest.Workspace)
	}
	if tester.startRequest.Provider.Model != "model-a" || tester.startRequest.Provider.APIKey != "secret" {
		t.Fatalf("tester fixed configuration was not propagated: %#v", tester.startRequest)
	}
	run, err := runs.GetByRunID(ctx, tester.startRequest.Run.RunID)
	if err != nil || run.ExternalSystem != agentRunnerExternalSystem || run.ExternalID != run.RunID {
		t.Fatalf("unexpected tester run identity: run=%#v err=%v", run, err)
	}
	if !strings.Contains(run.InputJSON, `"verification"`) || !strings.Contains(run.InputJSON, `"floor_command":"go test ./..."`) {
		t.Fatalf("run input must carry the verification prescription: %s", run.InputJSON)
	}
}

func TestTickDispatchesTesterWithoutPrescriptionWhenRepoDeclaresNone(t *testing.T) {
	ctx := context.Background()
	tasks, runs := newTickRepositories(t)
	tester := &fakeTesterRunnerCoordinator{}
	orch := New(tasks, runs, WithTesterRunner(tester, ReviewerRunnerSettings{
		ResolveRepository: stubRunnerRepository(),
	}))
	if err := tasks.Create(ctx, testerVerificationTickTask("task-tick-tester-nofloor", model.TaskStatusPending)); err != nil {
		t.Fatalf("create tester task: %v", err)
	}
	if _, err := orch.Tick(ctx, TickRequest{DispatchLimit: 10}); err != nil {
		t.Fatalf("tick: %v", err)
	}
	run, err := runs.GetByRunID(ctx, tester.startRequest.Run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(run.InputJSON, `"verification"`) {
		t.Fatalf("repo without a floor must not carry a verification section: %s", run.InputJSON)
	}
}

func TestTickParksTesterTaskWhenTesterRunnerUnwired(t *testing.T) {
	ctx := context.Background()
	tasks, runs := newTickRepositories(t)
	orch := New(tasks, runs)
	if err := tasks.Create(ctx, testerVerificationTickTask("task-tick-tester-unwired", model.TaskStatusPending)); err != nil {
		t.Fatalf("create tester task: %v", err)
	}
	result, err := orch.Tick(ctx, TickRequest{DispatchLimit: 10})
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	stored, err := tasks.GetByTaskID(ctx, "task-tick-tester-unwired")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.TaskStatusHumanRequired || stored.CurrentRunID != nil || stored.LastError != testerRunnerNotConfiguredReason {
		t.Fatalf("unwired tester must fall back to human handling: %#v", stored)
	}
	if len(result.DispatchedTasks) != 0 || len(result.DispatchFailures) != 1 {
		t.Fatalf("parked tester must be recorded once: %#v", result)
	}
	repeated, err := orch.Tick(ctx, TickRequest{DispatchLimit: 10})
	if err != nil {
		t.Fatalf("repeat tick: %v", err)
	}
	if len(repeated.DispatchedTasks) != 0 || len(repeated.DispatchFailures) != 0 {
		t.Fatalf("parked tester must not re-enter dispatch: %#v", repeated)
	}
}

func TestTickReconcilesTesterRunThroughTesterCoordinator(t *testing.T) {
	ctx := context.Background()
	tasks, runs := newTickRepositories(t)
	tester := &fakeTesterRunnerCoordinator{reconcileResult: &ReviewerRunnerReconcileResult{
		Execution: &model.AgentExecution{RunID: "run-tick-tester-active", ExecutionID: "run-tick-tester-active", Status: model.AgentExecutionStatusRunning},
	}}
	orch := New(tasks, runs, WithTesterRunner(tester, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}))
	runID := "run-tick-tester-active"
	task := testerVerificationTickTask("task-tick-tester-active", model.TaskStatusRunning)
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
	if tester.reconcileCalls != 1 || len(result.PolledRuns) != 1 || result.PolledRuns[0].ExecutionStatus != string(model.AgentExecutionStatusRunning) {
		t.Fatalf("tester run must reconcile through the tester coordinator: calls=%d result=%#v", tester.reconcileCalls, result.PolledRuns)
	}
	stored, err := tasks.GetByTaskID(ctx, task.TaskID)
	if err != nil || stored.LeaseUntil == nil {
		t.Fatalf("active tester task lease was not renewed: task=%#v err=%v", stored, err)
	}
}

func TestTickBlocksSuccessfulTesterRunWithInvalidResult(t *testing.T) {
	ctx := context.Background()
	tasks, runs := newTickRepositories(t)
	tester := &fakeTesterRunnerCoordinator{
		reconcileResult: &ReviewerRunnerReconcileResult{
			Execution: &model.AgentExecution{RunID: "run-tick-tester-invalid", ExecutionID: "run-tick-tester-invalid", Status: model.AgentExecutionStatusSucceeded},
		},
		reconcileErrors: []error{newInvalidRunnerResultError(errors.New("validate tester runner result: tests evidence is required"))},
	}
	orch := New(tasks, runs, WithTesterRunner(tester, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}))
	runID := "run-tick-tester-invalid"
	task := testerVerificationTickTask("task-tick-tester-invalid", model.TaskStatusRunning)
	task.CurrentRunID = &runID
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
		t.Fatalf("expected invalid tester result to be reported: %#v", result.PolledRuns)
	}
	storedTask, _ := tasks.GetByTaskID(ctx, task.TaskID)
	storedRun, _ := runs.GetByRunID(ctx, runID)
	if storedTask.Status != model.TaskStatusHumanRequired || storedRun.Status != model.RunStatusBlocked || storedRun.ErrorMessage == "" {
		t.Fatalf("successful tester execution with invalid result did not converge: task=%#v run=%#v", storedTask, storedRun)
	}
}

func TestTickRetriesTesterWorkspacePreparationFailureWithNewRun(t *testing.T) {
	ctx := context.Background()
	tasks, runs := newTickRepositories(t)
	tester := &fakeTesterRunnerCoordinator{startErrors: []error{errors.New("git fetch failed"), nil}}
	orch := New(tasks, runs, WithTesterRunner(tester, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}))
	if err := tasks.Create(ctx, testerVerificationTickTask("task-tester-workspace-retry", model.TaskStatusPending)); err != nil {
		t.Fatalf("create tester task: %v", err)
	}

	first, err := orch.Tick(ctx, TickRequest{DispatchLimit: 1})
	if err != nil {
		t.Fatalf("first tick must isolate workspace preparation failure: %v", err)
	}
	if len(first.DispatchFailures) != 1 || tester.startCalls != 1 {
		t.Fatalf("expected one failed tester dispatch: result=%#v starts=%d", first.DispatchFailures, tester.startCalls)
	}
	storedTask, err := tasks.GetByTaskID(ctx, "task-tester-workspace-retry")
	if err != nil {
		t.Fatal(err)
	}
	if storedTask.Status != model.TaskStatusRetryPending || storedTask.CurrentRunID == nil {
		t.Fatalf("tester workspace failure did not enter retry state: %#v", storedTask)
	}
	failedRunID := *storedTask.CurrentRunID

	second, err := orch.Tick(ctx, TickRequest{DispatchLimit: 1})
	if err != nil {
		t.Fatalf("retry tick: %v", err)
	}
	if len(second.DispatchFailures) != 0 || len(second.DispatchedTasks) != 1 || tester.startCalls != 2 {
		t.Fatalf("retry did not create a new tester run: result=%#v starts=%d", second, tester.startCalls)
	}
	if second.DispatchedTasks[0].RunID == failedRunID {
		t.Fatalf("retry reused failed run %s", failedRunID)
	}
}

type fakeTesterRunnerCoordinator struct {
	startCalls      int
	startRequest    TesterRunnerStartRequest
	startErrors     []error
	reconcileCalls  int
	reconcileErrors []error
	reconcileResult *ReviewerRunnerReconcileResult
}

func (f *fakeTesterRunnerCoordinator) Start(_ context.Context, request TesterRunnerStartRequest) (*model.AgentExecution, error) {
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

func (f *fakeTesterRunnerCoordinator) Reconcile(context.Context, string) (*ReviewerRunnerReconcileResult, error) {
	f.reconcileCalls++
	if len(f.reconcileErrors) >= f.reconcileCalls && f.reconcileErrors[f.reconcileCalls-1] != nil {
		return f.reconcileResult, f.reconcileErrors[f.reconcileCalls-1]
	}
	return f.reconcileResult, nil
}
