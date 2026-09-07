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
		StructuredOutput: json.RawMessage(`{"task_id":"task-developer-runner","run_id":"run-developer-runner","outcome":"change_ready","summary":"implemented fix","tests":[{"command":"go test ./...","passed":true,"summary":"passed"}],"merge_request":{"title":"Fix issue","description":"Implements fix"}}`),
	}

	observed, err := fixture.coordinator.Reconcile(context.Background(), fixture.run.RunID)
	if err != nil {
		t.Fatalf("reconcile developer result: %v", err)
	}
	if observed.Submission == nil || fixture.finalizer.calls != 1 || fixture.submitter.calls != 1 {
		t.Fatalf("developer result was not finalized and submitted: observed=%#v finalizer=%d submit=%d", observed, fixture.finalizer.calls, fixture.submitter.calls)
	}
	if fixture.finalizer.request.TaskType != fixture.task.Type ||
		fixture.finalizer.request.RepositoryURL != "https://gitlab.example/group/project.git" ||
		fixture.finalizer.request.TargetBranch != "master" ||
		fixture.finalizer.request.DeliveryBranch != fixture.task.DeliveryBranch {
		t.Fatalf("developer finalizer did not receive trusted task delivery metadata: %#v", fixture.finalizer.request)
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
		StructuredOutput: json.RawMessage(`{"task_id":"task-developer-runner","run_id":"run-developer-runner","outcome":"rework_ready","summary":"applied feedback","tests":[{"command":"go test ./...","passed":true,"summary":"passed"}]}`),
	}

	if _, err := fixture.coordinator.Reconcile(context.Background(), fixture.run.RunID); err != nil {
		t.Fatalf("reconcile rework result: %v", err)
	}
	if fixture.finalizer.request.RunID != fixture.run.RunID || fixture.submitter.calls != 1 {
		t.Fatalf("rework delivery was not finalized: request=%#v submits=%d", fixture.finalizer.request, fixture.submitter.calls)
	}
	if len(fixture.submitter.request.Canonical.Writebacks) != 0 {
		t.Fatalf("rework created a new MR: %#v", fixture.submitter.request.Canonical.Writebacks)
	}
}

func TestReconcileDeveloperRunnerConvergesDeliveryFailureToHumanRequired(t *testing.T) {
	fixture := newDeveloperRunnerFixture(t, "develop_from_issue")
	fixture.finalizer.err = workspace.ErrWorkspaceConflict
	if _, err := fixture.coordinator.Start(context.Background(), fixture.startRequest()); err != nil {
		t.Fatalf("start developer execution: %v", err)
	}
	fixture.runner.execution.Status = runner.StatusSucceeded
	fixture.runner.execution.ResultHash = "result-hash"
	fixture.runner.result = &runner.Result{
		ExecutionID: fixture.run.RunID, ResultHash: "result-hash",
		StructuredOutput: json.RawMessage(`{"task_id":"task-developer-runner","run_id":"run-developer-runner","outcome":"change_ready","summary":"implemented fix","tests":[{"command":"go test ./...","passed":true,"summary":"passed"}],"merge_request":{"title":"Fix issue","description":"Implements fix"}}`),
	}
	orch := New(fixture.tasks, fixture.runs, WithDeveloperRunner(fixture.coordinator, ReviewerRunnerSettings{WorkspaceRoot: fixture.root, ResolveCredential: stubRunnerCredential()}))

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
	if storedRun.Status != model.RunStatusBlocked || !strings.Contains(storedRun.ErrorMessage, "finalize developer git delivery") {
		t.Fatalf("delivery failure did not block the run: %#v", storedRun)
	}
	storedTask, err := fixture.tasks.GetByTaskID(context.Background(), fixture.task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if storedTask.Status != model.TaskStatusHumanRequired || !strings.Contains(storedTask.LastError, "finalize developer git delivery") {
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
	runnerClient := &fakeReviewerRunnerClient{}
	submitter := &recordingResultSubmitter{}
	return developerRunnerFixture{
		root: t.TempDir(), tasks: tasks, runs: runs, task: task, run: run, preparer: preparer, finalizer: finalizer, runner: runnerClient, submitter: submitter,
		coordinator: NewDeveloperRunnerCoordinator(tasks, runs, executions, preparer, finalizer, runnerClient, submitter, ReviewerRunnerSettings{WorkspaceRoot: "", ResolveCredential: stubRunnerCredential()}),
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
	return &workspace.Manifest{Status: workspace.ManifestStatusReady, TaskID: request.TaskID, RunID: request.RunID, Role: "DeveloperWorker", TaskType: request.TaskType}, nil
}

type fakeDeveloperFinalizer struct {
	calls    int
	request  workspace.DeveloperFinalizeRequest
	delivery *workspace.DeveloperDelivery
	err      error
}

func (f *fakeDeveloperFinalizer) Finalize(_ context.Context, request workspace.DeveloperFinalizeRequest) (*workspace.DeveloperDelivery, error) {
	f.calls++
	f.request = request
	if f.err != nil {
		return nil, f.err
	}
	return f.delivery, nil
}
