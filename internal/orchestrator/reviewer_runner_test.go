package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sdlc-coordinator/internal/agent"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/workspace"
)

func TestReviewerRepositoryRejectsReadableRepositoryThatDoesNotMatchTarget(t *testing.T) {
	_, err := reviewerRepository([]agent.RepoSpec{{
		Name: "other-project", URL: "https://gitlab.example/group/other-project.git", Access: "readable",
	}}, "group/project")
	if err == nil {
		t.Fatal("expected reviewer repository mismatch to be rejected")
	}
}

func TestReviewerRunnerStartPersistsExecutionAndIsIdempotent(t *testing.T) {
	fixture := newReviewerRunnerFixture(t)
	started, err := fixture.coordinator.Start(context.Background(), fixture.startRequest())
	if err != nil {
		t.Fatalf("start reviewer execution: %v", err)
	}
	if started.RunID != fixture.run.RunID || started.ExecutionID != fixture.run.RunID || started.RequestHash == "" {
		t.Fatalf("unexpected execution projection: %#v", started)
	}
	if fixture.preparer.calls != 1 || fixture.runner.createCalls != 1 {
		t.Fatalf("unexpected initial calls: prepare=%d create=%d", fixture.preparer.calls, fixture.runner.createCalls)
	}
	if fixture.runner.createRequest.TimeoutSeconds != int64((45*time.Minute)/time.Second) {
		t.Fatalf("timeout was not propagated: %#v", fixture.runner.createRequest)
	}
	if _, err := os.Stat(filepath.Join(fixture.root, "runs", fixture.run.RunID, "prompt.md")); err != nil {
		t.Fatalf("task bundle was not written: %v", err)
	}

	replayed, err := fixture.coordinator.Start(context.Background(), fixture.startRequest())
	if err != nil {
		t.Fatalf("replay reviewer start: %v", err)
	}
	if replayed.ID != started.ID || fixture.preparer.calls != 1 || fixture.runner.createCalls != 1 {
		t.Fatalf("start was not idempotent: projection=%#v prepare=%d create=%d", replayed, fixture.preparer.calls, fixture.runner.createCalls)
	}
}

func TestReviewerRunnerReconcileAdvancesEventCursor(t *testing.T) {
	fixture := newReviewerRunnerFixture(t)
	if _, err := fixture.coordinator.Start(context.Background(), fixture.startRequest()); err != nil {
		t.Fatalf("start reviewer execution: %v", err)
	}
	fixture.runner.execution.Status = runner.StatusRunning
	fixture.runner.events = []runner.Event{
		{ExecutionID: fixture.run.RunID, Seq: 1, Type: "accepted"},
		{ExecutionID: fixture.run.RunID, Seq: 2, Type: "running"},
	}

	observed, err := fixture.coordinator.Reconcile(context.Background(), fixture.run.RunID)
	if err != nil {
		t.Fatalf("reconcile reviewer execution: %v", err)
	}
	if observed.Execution.Status != model.AgentExecutionStatusRunning || observed.Execution.LastEventSeq != 2 {
		t.Fatalf("unexpected reconciled projection: %#v", observed.Execution)
	}
	if len(observed.Events) != 2 || fixture.runner.eventsAfter != 0 || fixture.submitter.calls != 0 {
		t.Fatalf("unexpected reconcile activity: result=%#v after=%d submissions=%d", observed, fixture.runner.eventsAfter, fixture.submitter.calls)
	}

	fixture.runner.events = append(fixture.runner.events, runner.Event{ExecutionID: fixture.run.RunID, Seq: 3, Type: "provider_progress"})
	if _, err := fixture.coordinator.Reconcile(context.Background(), fixture.run.RunID); err != nil {
		t.Fatalf("reconcile reviewer execution again: %v", err)
	}
	if fixture.runner.eventsAfter != 2 {
		t.Fatalf("expected event polling after seq 2, got %d", fixture.runner.eventsAfter)
	}
}

func TestReviewerRunnerReconcileNormalizesAndSubmitsSuccessfulResult(t *testing.T) {
	fixture := newReviewerRunnerFixture(t)
	if _, err := fixture.coordinator.Start(context.Background(), fixture.startRequest()); err != nil {
		t.Fatalf("start reviewer execution: %v", err)
	}
	fixture.runner.execution.Status = runner.StatusSucceeded
	fixture.runner.execution.ResultHash = "result-hash"
	fixture.runner.result = &runner.Result{
		ExecutionID:      fixture.run.RunID,
		ResultHash:       "result-hash",
		StructuredOutput: json.RawMessage(`{"task_id":"task-reviewer-runner","run_id":"run-reviewer-runner","verdict":"approved","summary":"no blocking issue","findings":[],"test_summary":"not run: review only","comment_body":"AI Review 结论：建议合并。"}`),
	}

	observed, err := fixture.coordinator.Reconcile(context.Background(), fixture.run.RunID)
	if err != nil {
		t.Fatalf("reconcile successful reviewer execution: %v", err)
	}
	if observed.Submission == nil || fixture.submitter.calls != 1 {
		t.Fatalf("expected one result submission: %#v", observed)
	}
	if fixture.submitter.request.Source != "agent_runner" || string(fixture.submitter.request.RawPayload) != string(fixture.runner.result.StructuredOutput) {
		t.Fatalf("unexpected result ingress request: %#v", fixture.submitter.request)
	}
	canonical := fixture.submitter.request.Canonical
	if canonical.TaskID != fixture.task.TaskID || canonical.RunID != fixture.run.RunID || canonical.Status != results.StatusSucceeded {
		t.Fatalf("unexpected canonical result: %#v", canonical)
	}
	if len(canonical.Writebacks) != 1 || canonical.Writebacks[0].MergeRequestIID != 21 {
		t.Fatalf("reviewer writeback semantics were not preserved: %#v", canonical.Writebacks)
	}
}

func TestReviewerRunnerReconcilePersistsTerminalFailureWithoutSubmitting(t *testing.T) {
	fixture := newReviewerRunnerFixture(t)
	if _, err := fixture.coordinator.Start(context.Background(), fixture.startRequest()); err != nil {
		t.Fatalf("start reviewer execution: %v", err)
	}
	fixture.runner.execution.Status = runner.StatusFailed
	fixture.runner.execution.ErrorCode = "provider_failed"

	observed, err := fixture.coordinator.Reconcile(context.Background(), fixture.run.RunID)
	if err != nil {
		t.Fatalf("reconcile failed reviewer execution: %v", err)
	}
	if observed.Execution.Status != model.AgentExecutionStatusFailed || observed.Execution.ErrorCode != "provider_failed" {
		t.Fatalf("unexpected failed projection: %#v", observed.Execution)
	}
	if fixture.runner.resultCalls != 0 || fixture.submitter.calls != 0 {
		t.Fatalf("terminal failure fetched/submitted result: result_calls=%d submissions=%d", fixture.runner.resultCalls, fixture.submitter.calls)
	}
}

type reviewerRunnerFixture struct {
	root        string
	task        *model.Task
	run         *model.Run
	preparer    *fakeReviewerWorkspacePreparer
	runner      *fakeReviewerRunnerClient
	submitter   *recordingResultSubmitter
	coordinator *ReviewerRunnerCoordinator
}

func newReviewerRunnerFixture(t *testing.T) reviewerRunnerFixture {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.AgentExecutionEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	tasks := repository.NewTaskRepository(db)
	runs := repository.NewRunRepository(db)
	executions := repository.NewAgentExecutionRepository(db)
	runID := "run-reviewer-runner"
	task := &model.Task{
		TaskID: "task-reviewer-runner", DedupeKey: "reviewer-runner", Type: "review_mr", Role: "ReviewerWorker",
		TargetRepo: "group/project", SourceRepo: "group/project", Objective: "review MR", AcceptanceCriteria: "return verdict",
		Status: model.TaskStatusRunning, CurrentRunID: &runID,
		ContextJSON: `{"project":{"path":"group/project"},"merge_request":{"iid":21,"url":"https://gitlab.example/group/project/-/merge_requests/21","review_mode":"full","source_branch":"feature/review","target_branch":"master","head_sha":"abc123"},"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"feature/review","access":"readable"}]}`,
	}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := tasks.Create(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := runs.Create(context.Background(), run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	preparer := &fakeReviewerWorkspacePreparer{}
	runnerClient := &fakeReviewerRunnerClient{}
	submitter := &recordingResultSubmitter{}
	return reviewerRunnerFixture{
		root: t.TempDir(), task: task, run: run, preparer: preparer, runner: runnerClient, submitter: submitter,
		coordinator: NewReviewerRunnerCoordinator(tasks, runs, executions, preparer, runnerClient, submitter),
	}
}

func (f reviewerRunnerFixture) startRequest() ReviewerRunnerStartRequest {
	return ReviewerRunnerStartRequest{
		Task: f.task,
		Run:  f.run,
		Workspace: workspace.ReviewerRequest{
			WorkspaceRoot: f.root, Project: f.task.TargetRepo, RepositoryURL: "https://gitlab.example/group/project.git",
			SourceBranch: "feature/review", TargetBranch: "master", HeadSHA: "abc123", RequestedReviewMode: workspace.ReviewModeFull,
		},
		Provider: runner.ProviderConfig{Name: "fake", Revision: 7, APIKey: "secret"},
		Timeout:  45 * time.Minute,
	}
}

type fakeReviewerWorkspacePreparer struct{ calls int }

func (f *fakeReviewerWorkspacePreparer) Prepare(_ context.Context, request workspace.ReviewerRequest) (*workspace.Manifest, error) {
	f.calls++
	root := filepath.Join(request.WorkspaceRoot, "runs", request.RunID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &workspace.Manifest{Status: workspace.ManifestStatusReady, TaskID: request.TaskID, RunID: request.RunID, Role: "ReviewerWorker"}, nil
}

type fakeReviewerRunnerClient struct {
	createCalls   int
	createRequest runner.CreateExecutionRequest
	execution     runner.Execution
	events        []runner.Event
	eventsAfter   int64
	result        *runner.Result
	resultCalls   int
}

func (f *fakeReviewerRunnerClient) Create(_ context.Context, request runner.CreateExecutionRequest) (*runner.Execution, error) {
	f.createCalls++
	f.createRequest = request
	hash, err := runner.RequestHash(request)
	if err != nil {
		return nil, err
	}
	f.execution = runner.Execution{
		ExecutionID: request.ExecutionID, TaskID: request.TaskID, Role: request.Role,
		Provider: request.Provider.Name, Status: runner.StatusAccepted, RequestHash: hash,
	}
	copy := f.execution
	return &copy, nil
}

func (f *fakeReviewerRunnerClient) Get(_ context.Context, _ string) (*runner.Execution, error) {
	copy := f.execution
	return &copy, nil
}

func (f *fakeReviewerRunnerClient) Events(_ context.Context, _ string, after int64) ([]runner.Event, error) {
	f.eventsAfter = after
	items := make([]runner.Event, 0, len(f.events))
	for _, event := range f.events {
		if event.Seq > after {
			items = append(items, event)
		}
	}
	return items, nil
}

func (f *fakeReviewerRunnerClient) Result(_ context.Context, _ string) (*runner.Result, error) {
	f.resultCalls++
	return f.result, nil
}

func (f *fakeReviewerRunnerClient) Cancel(context.Context, string) (*runner.Execution, error) {
	return nil, nil
}

type recordingResultSubmitter struct {
	calls   int
	request SubmitResultRequest
}

func (s *recordingResultSubmitter) SubmitResult(_ context.Context, request SubmitResultRequest) (*SubmitResultResponse, error) {
	s.calls++
	s.request = request
	return &SubmitResultResponse{Submission: &model.ResultSubmission{SubmissionID: "submission-runner", Status: model.ResultSubmissionStatusSucceeded}}, nil
}
