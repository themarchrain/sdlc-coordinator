package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/workspace"
)

func TestDeveloperRunnerFinalizesGitBeforeSubmittingChangeReady(t *testing.T) {
	fixture := newDeveloperRunnerFixture(t, "develop_from_issue")
	if _, err := fixture.coordinator.Start(context.Background(), fixture.startRequest()); err != nil {
		t.Fatalf("start developer execution: %v", err)
	}
	if fixture.runner.createRequest.Provider.Name != "codex-cli" {
		t.Fatalf("developer execution did not preserve the selected Codex provider: %#v", fixture.runner.createRequest.Provider)
	}
	fixture.runner.execution.Status = runner.StatusSucceeded
	fixture.runner.execution.ResultHash = "result-hash"
	fixture.runner.result = &runner.Result{
		ExecutionID: fixture.run.RunID, ResultHash: "result-hash",
		StructuredOutput: json.RawMessage(`{"task_id":"task-developer-runner","run_id":"run-developer-runner","outcome":"change_ready","summary":"implemented fix","tests":[{"command":"go test ./...","passed":true,"summary":"passed"}],"merge_request":{"title":"Fix issue","description":"Implements fix"},"commit_message":"fix: implement the requested change"}`),
	}

	observed, err := fixture.coordinator.Reconcile(context.Background(), fixture.run.RunID)
	if err != nil {
		t.Fatalf("reconcile developer result: %v", err)
	}
	if observed.Submission == nil || fixture.finalizer.commits != 1 || fixture.finalizer.pushes != 1 || fixture.submitter.calls != 1 {
		t.Fatalf("developer result was not committed, pushed and submitted: observed=%#v commits=%d pushes=%d submit=%d", observed, fixture.finalizer.commits, fixture.finalizer.pushes, fixture.submitter.calls)
	}
	if fixture.finalizer.commitRequest.TaskType != fixture.task.Type ||
		fixture.finalizer.commitRequest.RepositoryURL != "https://gitlab.example/group/project.git" ||
		fixture.finalizer.commitRequest.TargetBranch != "master" ||
		fixture.finalizer.commitRequest.DeliveryBranch != fixture.task.DeliveryBranch ||
		fixture.finalizer.commitRequest.BaselineSHA == "" {
		t.Fatalf("developer finalizer did not receive trusted task delivery metadata: %#v", fixture.finalizer.commitRequest)
	}
	if fixture.checkpoints.record == nil || fixture.checkpoints.record.CommitSHA != "commit-123" {
		t.Fatalf("delivery checkpoint must hold the pushed commit: %#v", fixture.checkpoints.record)
	}
	canonical := fixture.submitter.request.Canonical
	if canonical.Status != results.StatusSucceeded || canonical.Artifacts["commit_sha"] != "commit-123" {
		t.Fatalf("unexpected developer canonical result: %#v", canonical)
	}
	if len(canonical.Writebacks) != 1 || canonical.Writebacks[0].Kind != "create_merge_request" || canonical.Writebacks[0].SourceBranch != fixture.task.DeliveryBranch {
		t.Fatalf("developer MR semantics were not preserved: %#v", canonical.Writebacks)
	}
}

func TestDeveloperRunnerReworkPushesWithoutCreatingMergeRequest(t *testing.T) {
	fixture := newDeveloperRunnerFixture(t, model.TaskTypeReworkMergeRequest)
	fixture.task.DeliveryBranch = "feature/existing"
	fixture.task.ContextJSON = `{"project":{"path":"group/project"},"merge_request":{"source_branch":"feature/existing","target_branch":"master","expected_head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"rework_request":{"context":"补充边界测试"},"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"master","access":"writable"}]}`
	if err := fixture.tasks.Update(context.Background(), fixture.task); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.coordinator.Start(context.Background(), fixture.startRequest()); err != nil {
		t.Fatalf("start rework execution: %v", err)
	}
	if fixture.runner.createRequest.Provider.Name != "codex-cli" {
		t.Fatalf("rework execution did not preserve the selected Codex provider: %#v", fixture.runner.createRequest.Provider)
	}
	fixture.runner.execution.Status = runner.StatusSucceeded
	fixture.runner.execution.ResultHash = "result-hash"
	fixture.runner.result = &runner.Result{
		ExecutionID: fixture.run.RunID, ResultHash: "result-hash",
		StructuredOutput: json.RawMessage(`{"task_id":"task-developer-runner","run_id":"run-developer-runner","outcome":"rework_ready","summary":"applied feedback","tests":[{"command":"go test ./...","passed":true,"summary":"passed"}],"commit_message":"fix: apply the rework request"}`),
	}

	if _, err := fixture.coordinator.Reconcile(context.Background(), fixture.run.RunID); err != nil {
		t.Fatalf("reconcile rework result: %v", err)
	}
	if fixture.finalizer.commitRequest.RunID != fixture.run.RunID || fixture.submitter.calls != 1 {
		t.Fatalf("rework delivery was not finalized: request=%#v submits=%d", fixture.finalizer.commitRequest, fixture.submitter.calls)
	}
	if len(fixture.submitter.request.Canonical.Writebacks) != 0 {
		t.Fatalf("rework created a new MR: %#v", fixture.submitter.request.Canonical.Writebacks)
	}
}

func TestReconcileDeveloperRunnerConvergesDeliveryFailureToHumanRequired(t *testing.T) {
	fixture := newDeveloperRunnerFixture(t, "develop_from_issue")
	fixture.finalizer.commitErr = workspace.ErrWorkspaceConflict
	if _, err := fixture.coordinator.Start(context.Background(), fixture.startRequest()); err != nil {
		t.Fatalf("start developer execution: %v", err)
	}
	fixture.runner.execution.Status = runner.StatusSucceeded
	fixture.runner.execution.ResultHash = "result-hash"
	fixture.runner.result = &runner.Result{
		ExecutionID: fixture.run.RunID, ResultHash: "result-hash",
		StructuredOutput: json.RawMessage(`{"task_id":"task-developer-runner","run_id":"run-developer-runner","outcome":"change_ready","summary":"implemented fix","tests":[{"command":"go test ./...","passed":true,"summary":"passed"}],"merge_request":{"title":"Fix issue","description":"Implements fix"},"commit_message":"fix: implement the requested change"}`),
	}
	orch := New(fixture.tasks, fixture.runs, WithDeveloperRunner(fixture.coordinator, ReviewerRunnerSettings{WorkspaceRoot: fixture.root, ResolveRepository: stubRunnerRepository()}))

	observed, err := orch.ReconcileDeveloperRunner(context.Background(), fixture.run.RunID)
	if err != nil {
		t.Fatalf("delivery failure should converge instead of surfacing error: %v", err)
	}
	if observed == nil {
		t.Fatal("reconcile returned no observation")
	}
	storedRun, err := fixture.runs.GetByRunID(context.Background(), fixture.run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if storedRun.Status != model.RunStatusBlocked || !strings.Contains(storedRun.ErrorMessage, "create developer git delivery") {
		t.Fatalf("delivery failure did not block the run: %#v", storedRun)
	}
	storedTask, err := fixture.tasks.GetByTaskID(context.Background(), fixture.task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if storedTask.Status != model.TaskStatusHumanRequired || !strings.Contains(storedTask.LastError, "create developer git delivery") {
		t.Fatalf("delivery failure did not move the task to human required: %#v", storedTask)
	}

	running, err := fixture.runs.ListRunning(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range running {
		if item.RunID == fixture.run.RunID {
			t.Fatalf("converged run is still polled as running: %#v", item)
		}
	}

	again, err := orch.ReconcileDeveloperRunner(context.Background(), fixture.run.RunID)
	if err != nil {
		t.Fatalf("second reconcile after convergence should be a no-op: %v", err)
	}
	if again == nil {
		t.Fatal("second reconcile returned no observation")
	}
}

func TestRecordInvalidRunnerResultDoesNotDisturbStaleRun(t *testing.T) {
	fixture := newDeveloperRunnerFixture(t, "develop_from_issue")
	newRunID := "run-developer-runner-new"
	newRun := &model.Run{RunID: newRunID, TaskID: fixture.task.TaskID, WorkerRole: "DeveloperWorker", Status: model.RunStatusRunning}
	if err := fixture.runs.Create(context.Background(), newRun); err != nil {
		t.Fatal(err)
	}
	fixture.task.CurrentRunID = &newRunID
	if err := fixture.tasks.Update(context.Background(), fixture.task); err != nil {
		t.Fatal(err)
	}
	orch := New(fixture.tasks, fixture.runs)

	if err := orch.recordInvalidRunnerResult(context.Background(), fixture.run.RunID, errors.New("invalid result")); err != nil {
		t.Fatalf("record invalid result: %v", err)
	}
	storedRun, err := fixture.runs.GetByRunID(context.Background(), fixture.run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if storedRun.Status != model.RunStatusBlocked || !strings.Contains(storedRun.ErrorMessage, "invalid result") {
		t.Fatalf("stale run was not terminalized: %#v", storedRun)
	}
	storedTask, err := fixture.tasks.GetByTaskID(context.Background(), fixture.task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if storedTask.Status != model.TaskStatusRunning || storedTask.CurrentRunID == nil || *storedTask.CurrentRunID != newRunID {
		t.Fatalf("stale result disturbed the active run: %#v", storedTask)
	}
}

type developerRunnerFixture struct {
	root        string
	tasks       *repository.TaskRepository
	runs        *repository.RunRepository
	task        *model.Task
	run         *model.Run
	preparer    *fakeDeveloperWorkspacePreparer
	finalizer   *fakeDeveloperFinalizer
	checkpoints *fakeDeliveryCheckpoints
	runner      *fakeReviewerRunnerClient
	submitter   *recordingResultSubmitter
	coordinator *DeveloperRunnerCoordinator
}

func newDeveloperRunnerFixture(t *testing.T, taskType string) developerRunnerFixture {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.AgentExecutionEvent{}); err != nil {
		t.Fatal(err)
	}
	tasks := repository.NewTaskRepository(db)
	runs := repository.NewRunRepository(db)
	executions := repository.NewAgentExecutionRepository(db)
	runID := "run-developer-runner"
	task := &model.Task{
		TaskID: "task-developer-runner", DedupeKey: "developer-runner", Type: taskType, Role: "DeveloperWorker",
		TargetRepo: "group/project", SourceRepo: "group/project", Objective: "implement fix", AcceptanceCriteria: "tests pass",
		Status: model.TaskStatusRunning, CurrentRunID: &runID, DeliveryBranch: "agent/task-developer-runner",
		ContextJSON: `{"project":{"path":"group/project"},"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"master","access":"writable"}]}`,
	}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := tasks.Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if err := runs.Create(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	preparer := &fakeDeveloperWorkspacePreparer{}
	finalizer := &fakeDeveloperFinalizer{delivery: &workspace.DeveloperDelivery{CommitSHA: "commit-123", ChangedFiles: []string{"main.go"}, DiffSummary: "1 file changed"}}
	checkpoints := &fakeDeliveryCheckpoints{}
	runnerClient := &fakeReviewerRunnerClient{}
	submitter := &recordingResultSubmitter{}
	coordinator := NewDeveloperRunnerCoordinator(tasks, runs, executions, preparer, finalizer, runnerClient, submitter, ReviewerRunnerSettings{WorkspaceRoot: "", ResolveRepository: stubRunnerRepository()})
	coordinator.SetDeveloperDeliveryCheckpoints(checkpoints)
	return developerRunnerFixture{
		root: t.TempDir(), tasks: tasks, runs: runs, task: task, run: run, preparer: preparer, finalizer: finalizer, checkpoints: checkpoints, runner: runnerClient, submitter: submitter,
		coordinator: coordinator,
	}
}

func (f developerRunnerFixture) startRequest() DeveloperRunnerStartRequest {
	f.coordinator.settings.WorkspaceRoot = f.root
	return DeveloperRunnerStartRequest{
		Task: f.task, Run: f.run,
		Workspace: workspace.DeveloperRequest{
			WorkspaceRoot: f.root, Project: f.task.TargetRepo, RepositoryURL: "https://gitlab.example/group/project.git",
			TaskType: f.task.Type, TargetBranch: "master", DeliveryBranch: f.task.DeliveryBranch,
		},
		Provider: runner.ProviderConfig{Name: "codex-cli", Revision: 8}, Timeout: 30 * time.Minute,
	}
}

type fakeDeveloperWorkspacePreparer struct{ calls int }

func (f *fakeDeveloperWorkspacePreparer) Prepare(_ context.Context, request workspace.DeveloperRequest) (*workspace.Manifest, error) {
	f.calls++
	root := filepath.Join(request.WorkspaceRoot, "runs", request.RunID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &workspace.Manifest{Status: workspace.ManifestStatusReady, TaskID: request.TaskID, RunID: request.RunID, Role: "DeveloperWorker", TaskType: request.TaskType,
		Repository: workspace.RepositoryManifest{InitialHeadSHA: "baseline-sha", TargetSHA: "baseline-sha"}}, nil
}

type fakeDeveloperFinalizer struct {
	commits  int
	pushes   int
	// onCreate 在 CommitDelivery 计数后、返回前调用（故障注入用）。
	onCreate func()
	commitRequest workspace.DeveloperCommitRequest
	pushRequest   workspace.DeveloperPushRequest
	commitErr     error
	pushErr       error
	commit        *workspace.DeveloperCommitResult
	delivery      *workspace.DeveloperDelivery
}

func (f *fakeDeveloperFinalizer) CommitDelivery(_ context.Context, request workspace.DeveloperCommitRequest) (*workspace.DeveloperCommitResult, error) {
	f.commits++
	f.commitRequest = request
	if f.onCreate != nil {
		f.onCreate()
	}
	if f.commitErr != nil {
		return nil, f.commitErr
	}
	if f.commit != nil {
		return f.commit, nil
	}
	return &workspace.DeveloperCommitResult{CommitSHA: "commit-123"}, nil
}

func (f *fakeDeveloperFinalizer) PushDelivery(_ context.Context, request workspace.DeveloperPushRequest) (*workspace.DeveloperDelivery, error) {
	f.pushes++
	f.pushRequest = request
	if f.pushErr != nil {
		return nil, f.pushErr
	}
	if f.delivery != nil {
		return f.delivery, nil
	}
	return &workspace.DeveloperDelivery{CommitSHA: request.CommitSHA, ChangedFiles: []string{"main.go"}, DiffSummary: "1 file changed"}, nil
}

// fakeDeliveryCheckpoints 以内存态模拟租户库检查点:RecordBaseline 惰性钉
// 基线(Start 路径调用),Load 返回当前记录,RecordCommit 记录翻转。
type fakeDeliveryCheckpoints struct {
	record      *model.DeveloperDeliveryCheckpoint
	baselineErr error
	loadErr     error
	recordErr   error
	commits     []string
}

func (f *fakeDeliveryCheckpoints) RecordBaseline(_ context.Context, record *model.DeveloperDeliveryCheckpoint) error {
	if f.baselineErr != nil {
		return f.baselineErr
	}
	if f.record == nil {
		f.record = record
	}
	return nil
}

func (f *fakeDeliveryCheckpoints) Load(_ context.Context, _ string) (*model.DeveloperDeliveryCheckpoint, error) {
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	return f.record, nil
}

func (f *fakeDeliveryCheckpoints) RecordCommit(_ context.Context, _ string, commitSHA string) error {
	if f.recordErr != nil {
		return f.recordErr
	}
	f.commits = append(f.commits, commitSHA)
	if f.record != nil {
		f.record.CommitSHA = commitSHA
	}
	return nil
}

func TestDeveloperRunnerStartHonorsPinnedProviderAndPinsOnDispatch(t *testing.T) {
	fixture := newDeveloperRunnerFixture(t, "implement_issue")
	fixture.coordinator.SetRunnerProviderLookup(func(context.Context) RunnerSelection { return RunnerSelection{Provider: "codex-cli"} })

	if _, err := fixture.coordinator.Start(context.Background(), fixture.startRequest()); err != nil {
		t.Fatalf("start with tenant lookup: %v", err)
	}
	if provider := fixture.runner.createRequest.Provider; provider.Name != "codex-cli" {
		t.Fatalf("developer dispatch must honor the tenant lookup, got %#v", provider)
	}
	persisted, err := fixture.runs.GetByRunID(context.Background(), fixture.run.RunID)
	if err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if persisted.Provider != "codex-cli" {
		t.Fatalf("run must be pinned to the dispatched cli, got %q", persisted.Provider)
	}

	// 配置热切换后重放（恢复/投影重建路径）：钉住值优先于现取值。
	fixture.coordinator.SetRunnerProviderLookup(func(context.Context) RunnerSelection { return RunnerSelection{Provider: "claude-cli"} })
	if _, err := fixture.coordinator.Start(context.Background(), fixture.startRequest()); err != nil {
		t.Fatalf("replay after config switch: %v", err)
	}
	if provider := fixture.runner.createRequest.Provider; provider.Name != "codex-cli" {
		t.Fatalf("pinned provider must survive a config switch, got %#v", provider)
	}
}
