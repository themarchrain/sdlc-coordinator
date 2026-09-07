package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

type reviewerTakeoverBackend struct {
	*fakeGitLabWritebackBackend
	addCalls int
	labels   []string
	addErr   error
	calls    *[]string
}

func (f *reviewerTakeoverBackend) AddMergeRequestLabels(_ context.Context, _ string, mrIID int, req codehost.SetLabelsRequest) (*codehost.MergeRequest, error) {
	f.addCalls++
	f.labels = append([]string(nil), req.Labels...)
	if f.calls != nil {
		*f.calls = append(*f.calls, "label")
	}
	if f.addErr != nil {
		return nil, f.addErr
	}
	return &codehost.MergeRequest{IID: mrIID}, nil
}

type orderedReviewerRunner struct{ calls *[]string }

func (f *orderedReviewerRunner) Start(_ context.Context, request ReviewerRunnerStartRequest) (*model.AgentExecution, error) {
	*f.calls = append(*f.calls, "runner_start")
	return &model.AgentExecution{RunID: request.Run.RunID, ExecutionID: request.Run.RunID, Status: model.AgentExecutionStatusAccepted}, nil
}

func (*orderedReviewerRunner) Reconcile(context.Context, string) (*ReviewerRunnerReconcileResult, error) {
	return nil, nil
}

func TestDispatchReviewerPersistsAndAddsTakeoverLabelBeforeRunnerStart(t *testing.T) {
	ctx := context.Background()
	tasks, runs, writebacks := newReviewerTakeoverRepositories(t)
	task := createPendingReviewerTask(t, ctx, tasks, "task-reviewer-takeover")
	calls := []string{}
	codeHost := &reviewerTakeoverBackend{fakeGitLabWritebackBackend: &fakeGitLabWritebackBackend{}, calls: &calls}
	orch := New(tasks, runs, WithWritebackRepository(writebacks), WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, codeHost)), WithReviewerRunner(&orderedReviewerRunner{calls: &calls}, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}))

	result, err := orch.DispatchReviewerTask(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("dispatch reviewer: %v", err)
	}
	if len(calls) != 2 || calls[0] != "label" || calls[1] != "runner_start" {
		t.Fatalf("expected label before Runner start, got %#v", calls)
	}
	if result.Task.Status != model.TaskStatusRunning || result.Run.Status != model.RunStatusRunning {
		t.Fatalf("expected running reviewer task, got task=%s run=%s", result.Task.Status, result.Run.Status)
	}
	if codeHost.addCalls != 1 || len(codeHost.labels) != 1 || codeHost.labels[0] != reviewerTakeoverLabel {
		t.Fatalf("unexpected takeover label calls=%d labels=%#v", codeHost.addCalls, codeHost.labels)
	}
}

func TestDispatchReviewerContinuesWhenTakeoverLabelFails(t *testing.T) {
	ctx := context.Background()
	tasks, runs, writebacks := newReviewerTakeoverRepositories(t)
	task := createPendingReviewerTask(t, ctx, tasks, "task-reviewer-label-failure")
	runner := &fakeReviewerRunnerCoordinator{}
	codeHost := &reviewerTakeoverBackend{fakeGitLabWritebackBackend: &fakeGitLabWritebackBackend{}, addErr: errors.New("gitlab label unavailable")}
	orch := New(tasks, runs, WithWritebackRepository(writebacks), WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, codeHost)), WithReviewerRunner(runner, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}))

	if _, err := orch.DispatchReviewerTask(ctx, task.TaskID); err != nil {
		t.Fatalf("noncritical takeover label must not block dispatch: %v", err)
	}
	if runner.startCalls != 1 {
		t.Fatalf("expected Runner dispatch after label failure, got %d", runner.startCalls)
	}
	writeback, err := writebacks.GetByDedupeKey(ctx, "reviewer-takeover-label:"+task.TaskID)
	if err != nil || writeback.Status != model.WritebackStatusFailed || !strings.Contains(writeback.ErrorMessage, "gitlab label unavailable") {
		t.Fatalf("expected audited failed label writeback, writeback=%#v err=%v", writeback, err)
	}
}

func TestDispatchReviewerDoesNotRepeatSucceededTakeoverLabelOnRetry(t *testing.T) {
	ctx := context.Background()
	tasks, runs, writebacks := newReviewerTakeoverRepositories(t)
	task := createPendingReviewerTask(t, ctx, tasks, "task-reviewer-label-retry")
	runner := &fakeReviewerRunnerCoordinator{startErrors: []error{errors.New("runner unavailable"), nil}}
	codeHost := &reviewerTakeoverBackend{fakeGitLabWritebackBackend: &fakeGitLabWritebackBackend{}}
	orch := New(tasks, runs, WithWritebackRepository(writebacks), WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, codeHost)), WithReviewerRunner(runner, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}), WithPolicy(Policy{MaxAttempts: 3}))

	if _, err := orch.DispatchReviewerTask(ctx, task.TaskID); err == nil {
		t.Fatal("expected first Runner dispatch to fail")
	}
	if _, err := orch.DispatchReviewerTask(ctx, task.TaskID); err != nil {
		t.Fatalf("retry reviewer dispatch: %v", err)
	}
	if codeHost.addCalls != 1 {
		t.Fatalf("succeeded takeover label must be reused, got %d remote calls", codeHost.addCalls)
	}
}

func TestAmbiguousTakeoverLabelReconciliationNeverEscalatesReviewerTask(t *testing.T) {
	ctx := context.Background()
	tasks, runs, writebacks := newReviewerTakeoverRepositories(t)
	task := createPendingReviewerTask(t, ctx, tasks, "task-reviewer-label-ambiguous")
	codeHost := &reviewerTakeoverBackend{fakeGitLabWritebackBackend: &fakeGitLabWritebackBackend{}, addErr: codehost.NewAmbiguousError(errors.New("label response lost"))}
	orch := New(tasks, runs, WithWritebackRepository(writebacks), WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, codeHost)), WithReviewerRunner(&fakeReviewerRunnerCoordinator{}, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()}), WithPolicy(Policy{MaxAttempts: 1}))

	result, err := orch.DispatchReviewerTask(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("ambiguous noncritical label must not block dispatch: %v", err)
	}
	codeHost.reconcileErr = errors.New("gitlab still unavailable")
	if _, err := orch.ReconcileUnknownWritebacks(ctx, 10); err != nil {
		t.Fatalf("reconcile noncritical takeover label: %v", err)
	}
	storedTask, _ := tasks.GetByTaskID(ctx, task.TaskID)
	storedRun, _ := runs.GetByRunID(ctx, result.Run.RunID)
	if storedTask.Status != model.TaskStatusRunning || storedRun.Status != model.RunStatusRunning {
		t.Fatalf("noncritical label reconciliation must not escalate, got task=%s run=%s", storedTask.Status, storedRun.Status)
	}
}

func newReviewerTakeoverRepositories(t *testing.T) (*repository.TaskRepository, *repository.RunRepository, *repository.WritebackRepository) {
	t.Helper()
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Writeback{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return repository.NewTaskRepository(db), repository.NewRunRepository(db), repository.NewWritebackRepository(db)
}

func createPendingReviewerTask(t *testing.T, ctx context.Context, tasks *repository.TaskRepository, taskID string) *model.Task {
	t.Helper()
	repositoryID := fixtureRepositoryID
	task := &model.Task{
		TaskID: taskID, DedupeKey: "gitlab:mr-review:24207:12:" + taskID, Type: "review_mr", Role: "ReviewerWorker",
		SourceRepo: "group/project", TargetRepo: "group/project", RepositoryID: &repositoryID, Status: model.TaskStatusPending,
		ContextJSON: `{"project":{"path":"group/project"},"merge_request":{"iid":12,"url":"https://gitlab.example/group/project/-/merge_requests/12","source_branch":"feature/review","target_branch":"master","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"feature/review","access":"readable"}]}`,
	}
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create reviewer task: %v", err)
	}
	return task
}
