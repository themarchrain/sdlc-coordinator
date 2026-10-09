package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

type failOnceArtifactRepository struct {
	delegate ArtifactRepository
	failed   bool
}

func TestSubmitDeveloperReworkResultWaitsForMergeAndRemainsImmutable(t *testing.T) {
	orch, _, tasks, runs := newResultIngressFixture(t)
	ctx := context.Background()
	runID := "run-rework-result"
	task := &model.Task{
		TaskID: "task-rework-result", DedupeKey: "gitlab:mr-rework:1001:21", Type: model.TaskTypeReworkMergeRequest,
		Role: "DeveloperWorker", SourceRepo: "demo-group/demo-project", TargetRepo: "demo-group/demo-project",
		Status: model.TaskStatusRunning, CurrentRunID: &runID, DeliveryBranch: "developer/issue-19-20260806",
		ContextJSON: `{"repos":[{"name":"demo-project","url":"https://gitlab.example/demo-group/demo-project.git","branch":"main","access":"writable"}],"origin_issue":{"iid":19,"assignee_ids":[]},"merge_request":{"iid":21,"url":"https://gitlab.example/demo-group/demo-project/-/merge_requests/21","source_branch":"developer/issue-19-20260806","target_branch":"main","expected_head_sha":"abc123","assignee_ids":[],"reviewer_ids":[]},"rework_request":{"requester":{"id":44,"email":"requester@example.com"}}}`,
	}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create rework task: %v", err)
	}
	if err := runs.Create(ctx, run); err != nil {
		t.Fatalf("create rework run: %v", err)
	}

	contract := results.NewDeveloperContract()
	raw := json.RawMessage(`{"task_id":"task-rework-result","run_id":"run-rework-result","outcome":"rework_ready","summary":"updated original MR branch","delivery":{"commit_sha":"def456","changed_files":["status.go"],"diff_summary":"handle null status"},"tests":[{"command":"go test ./...","passed":true,"summary":"all passed"}]}`)
	canonical, err := orch.NormalizeRoleResult(ctx, contract, raw)
	if err != nil {
		t.Fatalf("normalize rework result: %v", err)
	}
	request := SubmitResultRequest{Source: "mcp_developer", ToolName: contract.ToolName(), RawPayload: raw, Canonical: canonical}
	response, err := orch.SubmitResult(ctx, request)
	if err != nil {
		t.Fatalf("submit rework result: %v", err)
	}
	if response.Task.Status != model.TaskStatusWaitingMerge || response.Run.Status != model.RunStatusSucceeded {
		t.Fatalf("expected waiting_merge task and succeeded run, got %s/%s", response.Task.Status, response.Run.Status)
	}
	if response.Submission.Status != model.ResultSubmissionStatusSucceeded {
		t.Fatalf("expected succeeded submission, got %s", response.Submission.Status)
	}

	replayed, err := orch.SubmitResult(ctx, request)
	if err != nil || replayed.Submission.SubmissionID != response.Submission.SubmissionID {
		t.Fatalf("expected immutable replay, got response=%#v error=%v", replayed, err)
	}
	changed := request
	changed.Canonical.Summary = "different result"
	if _, err := orch.SubmitResult(ctx, changed); !errors.Is(err, ErrResultConflict) {
		t.Fatalf("expected conflicting replay to be rejected, got %v", err)
	}
}

func TestSubmitReworkCanonicalRejectsCoordinatorOwnedIntentsBeforeSubmission(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*results.CanonicalResult)
	}{
		{
			name: "writeback",
			mutate: func(result *results.CanonicalResult) {
				result.Writebacks = []results.CodeHostWritebackIntent{{
					Kind: "create_merge_request", ProjectID: "demo-group/demo-project", Title: "duplicate MR",
					SourceBranch: "developer/issue-19-20260806", TargetBranch: "main",
				}}
			},
		},
		{
			name: "next task",
			mutate: func(result *results.CanonicalResult) {
				result.NextTasks = []results.NextTaskIntent{{Role: "DeveloperWorker", Objective: "unexpected child"}}
			},
		},
		{
			name: "effect",
			mutate: func(result *results.CanonicalResult) {
				result.Effects = []results.EffectIntent{{Kind: "send_im_message", Target: "default"}}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch, submissions, tasks, runs := newResultIngressFixture(t)
			task, run := createReworkIngressFixture(t, tasks, runs, "task-rework-guard-"+strings.ReplaceAll(tt.name, " ", "-"), "run-rework-guard-"+strings.ReplaceAll(tt.name, " ", "-"))
			canonical := validReworkCanonical(task.TaskID, run.RunID)
			tt.mutate(&canonical)

			if _, err := orch.SubmitResult(context.Background(), SubmitResultRequest{Source: "mcp_compat", Canonical: canonical}); err == nil || !strings.Contains(err.Error(), "must not include coordinator-owned intents") {
				t.Fatalf("expected coordinator-owned intent rejection, got %v", err)
			}
			if _, err := submissions.GetByRunID(context.Background(), run.RunID); !errors.Is(err, repository.ErrNotFound) {
				t.Fatalf("rejected result must not create submission, got %v", err)
			}
			storedTask, _ := tasks.GetByTaskID(context.Background(), task.TaskID)
			storedRun, _ := runs.GetByRunID(context.Background(), run.RunID)
			if storedTask.Status != model.TaskStatusRunning || storedRun.Status != model.RunStatusRunning {
				t.Fatalf("rejected result must not change task/run, got %s/%s", storedTask.Status, storedRun.Status)
			}
		})
	}
}

func TestSubmitReworkCanonicalRequiresDeliveryAndPassedTestEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*results.CanonicalResult)
	}{
		{name: "missing commit", mutate: func(result *results.CanonicalResult) { delete(result.Artifacts, "commit_sha") }},
		{name: "missing changed files", mutate: func(result *results.CanonicalResult) { delete(result.Artifacts, "changed_files") }},
		{name: "missing diff", mutate: func(result *results.CanonicalResult) { delete(result.Artifacts, "diff") }},
		{name: "failed test", mutate: func(result *results.CanonicalResult) {
			result.Artifacts["test_results"] = []results.TestEvidence{{Command: "go test ./...", Passed: false, Summary: "failed"}}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch, submissions, tasks, runs := newResultIngressFixture(t)
			task, run := createReworkIngressFixture(t, tasks, runs, "task-rework-evidence-"+strings.ReplaceAll(tt.name, " ", "-"), "run-rework-evidence-"+strings.ReplaceAll(tt.name, " ", "-"))
			canonical := validReworkCanonical(task.TaskID, run.RunID)
			tt.mutate(&canonical)

			if _, err := orch.SubmitResult(context.Background(), SubmitResultRequest{Source: "polling_fallback", Canonical: canonical}); err == nil || !strings.Contains(err.Error(), "rework_merge_request succeeded result") {
				t.Fatalf("expected rework evidence rejection, got %v", err)
			}
			if _, err := submissions.GetByRunID(context.Background(), run.RunID); !errors.Is(err, repository.ErrNotFound) {
				t.Fatalf("rejected result must not create submission, got %v", err)
			}
			storedTask, _ := tasks.GetByTaskID(context.Background(), task.TaskID)
			storedRun, _ := runs.GetByRunID(context.Background(), run.RunID)
			if storedTask.Status != model.TaskStatusRunning || storedRun.Status != model.RunStatusRunning {
				t.Fatalf("rejected result must not change task/run, got %s/%s", storedTask.Status, storedRun.Status)
			}
		})
	}
}

func createReworkIngressFixture(t *testing.T, tasks *repository.TaskRepository, runs *repository.RunRepository, taskID, runID string) (*model.Task, *model.Run) {
	t.Helper()
	task := &model.Task{
		TaskID: taskID, DedupeKey: "gitlab:mr-rework:1001:" + taskID, Type: model.TaskTypeReworkMergeRequest,
		Role: "DeveloperWorker", TargetRepo: "demo-group/demo-project", Status: model.TaskStatusRunning,
		CurrentRunID: &runID, DeliveryBranch: "developer/issue-19-20260806",
		ContextJSON: `{"origin_issue":{"iid":19,"assignee_ids":[]},"merge_request":{"iid":21,"url":"https://gitlab.example/demo-group/demo-project/-/merge_requests/21","source_branch":"developer/issue-19-20260806"},"rework_request":{"requester":{"email":"requester@example.com"}}}`,
	}
	run := &model.Run{RunID: runID, TaskID: taskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := tasks.Create(context.Background(), task); err != nil {
		t.Fatalf("create rework task: %v", err)
	}
	if err := runs.Create(context.Background(), run); err != nil {
		t.Fatalf("create rework run: %v", err)
	}
	return task, run
}

func validReworkCanonical(taskID, runID string) results.CanonicalResult {
	return results.CanonicalResult{
		TaskID: taskID, RunID: runID, Status: results.StatusSucceeded, Summary: "updated original MR branch",
		Artifacts: map[string]any{
			"commit_sha":    "def456",
			"changed_files": []string{"status.go", "status_test.go"},
			"diff":          "handle nil and blank values",
			"test_results":  []results.TestEvidence{{Command: "go test ./...", Passed: true, Summary: "all passed"}},
		},
	}
}

type failAlwaysArtifactRepository struct{}

func (failAlwaysArtifactRepository) UpsertByIdentity(context.Context, *model.Artifact) error {
	return errors.New("deterministic artifact failure")
}

type blockingArtifactRepository struct {
	delegate ArtifactRepository
	entered  chan struct{}
	release  chan struct{}
	once     sync.Once
	mu       sync.Mutex
	calls    int
}

func (r *blockingArtifactRepository) UpsertByIdentity(ctx context.Context, artifact *model.Artifact) error {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	r.once.Do(func() { close(r.entered) })
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.release:
	}
	return r.delegate.UpsertByIdentity(ctx, artifact)
}

func (r *blockingArtifactRepository) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *failOnceArtifactRepository) UpsertByIdentity(ctx context.Context, artifact *model.Artifact) error {
	if !r.failed {
		r.failed = true
		return fmt.Errorf("injected artifact failure")
	}
	return r.delegate.UpsertByIdentity(ctx, artifact)
}

func TestSubmitResultAcceptsAndReplaysSameCanonicalPayload(t *testing.T) {
	ctx := context.Background()
	orch, submissions, taskRepo, runRepo := newResultIngressFixture(t)
	createCallbackArtifactFixture(t, ctx, taskRepo, runRepo, "task-ingress", "run-ingress")
	request := SubmitResultRequest{
		Source: "mcp_tester", ToolName: "submit_tester_result", RawPayload: []byte(`{"outcome":"environment_blocker"}`),
		Canonical: results.CanonicalResult{TaskID: "task-ingress", RunID: "run-ingress", Status: results.StatusBlocked, Summary: "network blocked"},
	}

	first, err := orch.SubmitResult(ctx, request)
	if err != nil {
		t.Fatalf("submit first result: %v", err)
	}
	second, err := orch.SubmitResult(ctx, request)
	if err != nil {
		t.Fatalf("replay same result: %v", err)
	}
	if first.Submission.SubmissionID != second.Submission.SubmissionID {
		t.Fatalf("expected cached submission, got %s and %s", first.Submission.SubmissionID, second.Submission.SubmissionID)
	}
	stored, err := submissions.GetByRunID(ctx, "run-ingress")
	if err != nil {
		t.Fatalf("get submission: %v", err)
	}
	if stored.Status != model.ResultSubmissionStatusSucceeded || second.Task.Status != model.TaskStatusBlocked || second.Run.Status != model.RunStatusBlocked {
		t.Fatalf("unexpected replay state submission=%s task=%s run=%s", stored.Status, second.Task.Status, second.Run.Status)
	}
}

func TestSubmitResultRejectsDifferentPayloadForAcceptedRun(t *testing.T) {
	ctx := context.Background()
	orch, _, taskRepo, runRepo := newResultIngressFixture(t)
	createCallbackArtifactFixture(t, ctx, taskRepo, runRepo, "task-conflict", "run-conflict")
	base := SubmitResultRequest{Source: "mcp", Canonical: results.CanonicalResult{TaskID: "task-conflict", RunID: "run-conflict", Status: results.StatusSucceeded, Summary: "first"}}
	if _, err := orch.SubmitResult(ctx, base); err != nil {
		t.Fatalf("submit first result: %v", err)
	}
	base.Canonical.Summary = "different"
	if _, err := orch.SubmitResult(ctx, base); !errors.Is(err, ErrResultConflict) {
		t.Fatalf("expected result conflict, got %v", err)
	}
}

func TestSubmitResultValidationFailureDoesNotCreateSubmission(t *testing.T) {
	ctx := context.Background()
	orch, submissions, taskRepo, runRepo := newResultIngressFixture(t)
	createCallbackArtifactFixture(t, ctx, taskRepo, runRepo, "task-invalid", "run-invalid")
	_, err := orch.SubmitResult(ctx, SubmitResultRequest{Canonical: results.CanonicalResult{TaskID: "task-invalid", RunID: "run-invalid", Status: "invented"}})
	if err == nil {
		t.Fatal("expected canonical validation error")
	}
	if _, err := submissions.GetByRunID(ctx, "run-invalid"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected no durable submission, got %v", err)
	}
}

func TestSubmitResultConcurrentSamePayloadAppliesOnce(t *testing.T) {
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
	artifactRepo := repository.NewArtifactRepository(db)
	blocking := &blockingArtifactRepository{
		delegate: artifactRepo,
		entered:  make(chan struct{}),
		release:  make(chan struct{}),
	}
	orch := New(tasks, runs, WithArtifactRepository(blocking), WithResultSubmissionRepository(submissions))
	createCallbackArtifactFixture(t, ctx, tasks, runs, "task-concurrent", "run-concurrent")
	request := SubmitResultRequest{
		Source: "mcp_tester",
		Canonical: results.CanonicalResult{
			TaskID: "task-concurrent", RunID: "run-concurrent", Status: results.StatusSucceeded, Summary: "tests passed",
			Artifacts: map[string]any{"test_results": "all passed"},
		},
	}

	errs := make(chan error, 20)
	var first sync.WaitGroup
	first.Add(1)
	go func() {
		defer first.Done()
		_, err := orch.SubmitResult(ctx, request)
		errs <- err
	}()
	<-blocking.entered

	var followers sync.WaitGroup
	for i := 0; i < 19; i++ {
		followers.Add(1)
		go func() {
			defer followers.Done()
			_, err := orch.SubmitResult(ctx, request)
			errs <- err
		}()
	}
	followers.Wait()
	close(blocking.release)
	first.Wait()
	close(errs)

	succeeded := 0
	processing := 0
	for err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrResultProcessing):
			processing++
		default:
			t.Fatalf("unexpected concurrent submission error: %v", err)
		}
	}
	if succeeded != 1 || processing != 19 {
		t.Fatalf("expected one finalizer and 19 processing replays, got succeeded=%d processing=%d", succeeded, processing)
	}
	if blocking.callCount() != 1 {
		t.Fatalf("expected artifact application once, got %d", blocking.callCount())
	}
	stored, err := submissions.GetByRunID(ctx, "run-concurrent")
	if err != nil {
		t.Fatalf("get submission: %v", err)
	}
	if stored.Status != model.ResultSubmissionStatusSucceeded {
		t.Fatalf("expected succeeded submission, got %s", stored.Status)
	}
	artifacts, err := artifactRepo.ListByRunID(ctx, "run-concurrent")
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected one artifact, got %d", len(artifacts))
	}
}

func TestSubmitResultReplaysFailedFinalizationFromAcceptedCanonicalPayload(t *testing.T) {
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
	orch := New(tasks, runs, WithArtifactRepository(artifacts), WithResultSubmissionRepository(submissions))
	createCallbackArtifactFixture(t, ctx, tasks, runs, "task-resume", "run-resume")
	request := SubmitResultRequest{
		Source: "mcp_tester",
		Canonical: results.CanonicalResult{
			TaskID: "task-resume", RunID: "run-resume", Status: results.StatusSucceeded, Summary: "tests passed",
			Artifacts: map[string]any{"test_results": "all passed"},
		},
	}

	if _, err := orch.SubmitResult(ctx, request); err == nil {
		t.Fatal("expected injected finalization failure")
	}
	response, err := orch.SubmitResult(ctx, request)
	if err != nil {
		t.Fatalf("resume accepted result: %v", err)
	}
	if response.Submission.Status != model.ResultSubmissionStatusSucceeded {
		t.Fatalf("expected succeeded submission, got %s", response.Submission.Status)
	}
	if response.Task.Status != model.TaskStatusSucceeded || response.Run.Status != model.RunStatusSucceeded {
		t.Fatalf("expected terminal task/run, got task=%s run=%s", response.Task.Status, response.Run.Status)
	}

	storedArtifacts, err := repository.NewArtifactRepository(db).ListByRunID(ctx, "run-resume")
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(storedArtifacts) != 1 {
		t.Fatalf("expected one artifact after replay, got %d", len(storedArtifacts))
	}
}

func TestFinalizeRecoverableSubmissionsUsesStoredCanonicalPayload(t *testing.T) {
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
	orch := New(tasks, runs, WithArtifactRepository(artifacts), WithResultSubmissionRepository(submissions))
	createCallbackArtifactFixture(t, ctx, tasks, runs, "task-sweep", "run-sweep")
	request := SubmitResultRequest{
		Source: "mcp_tester",
		Canonical: results.CanonicalResult{
			TaskID: "task-sweep", RunID: "run-sweep", Status: results.StatusSucceeded, Summary: "tests passed",
			Artifacts: map[string]any{"test_results": "all passed"},
		},
	}
	if _, err := orch.SubmitResult(ctx, request); err == nil {
		t.Fatal("expected injected finalization failure")
	}

	restarted := New(
		tasks,
		runs,
		WithArtifactRepository(repository.NewArtifactRepository(db)),
		WithResultSubmissionRepository(submissions),
	)
	finalized, err := restarted.FinalizeRecoverableSubmissions(ctx, 10)
	if err != nil {
		t.Fatalf("finalize recoverable submissions: %v", err)
	}
	if len(finalized) != 1 {
		t.Fatalf("expected one finalization result, got %d", len(finalized))
	}
	if !finalized[0].Finalized || finalized[0].Status != model.ResultSubmissionStatusSucceeded {
		t.Fatalf("unexpected finalization result: %#v", finalized[0])
	}

	stored, err := submissions.GetByRunID(ctx, "run-sweep")
	if err != nil {
		t.Fatalf("get submission: %v", err)
	}
	if stored.Status != model.ResultSubmissionStatusSucceeded {
		t.Fatalf("expected succeeded submission, got %s", stored.Status)
	}
	if task, _ := tasks.GetByTaskID(ctx, "task-sweep"); task.Status != model.TaskStatusSucceeded {
		t.Fatalf("expected succeeded task, got %s", task.Status)
	}
}

func TestFinalizeRecoverableSubmissionsStopsAfterThreeFailures(t *testing.T) {
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
	orch := New(tasks, runs, WithArtifactRepository(failAlwaysArtifactRepository{}), WithResultSubmissionRepository(submissions))
	createCallbackArtifactFixture(t, ctx, tasks, runs, "task-finalize-stop", "run-finalize-stop")
	request := SubmitResultRequest{
		Source: "mcp_tester",
		Canonical: results.CanonicalResult{
			TaskID: "task-finalize-stop", RunID: "run-finalize-stop", Status: results.StatusSucceeded, Summary: "tests passed",
			Artifacts: map[string]any{"test_results": "all passed"},
		},
	}

	if _, err := orch.SubmitResult(ctx, request); err == nil {
		t.Fatal("expected first finalization failure")
	}
	for attempt := 2; attempt <= 3; attempt++ {
		results, err := orch.FinalizeRecoverableSubmissions(ctx, 10)
		if err != nil {
			t.Fatalf("finalization attempt %d: %v", attempt, err)
		}
		if len(results) != 1 || results[0].Error == "" {
			t.Fatalf("expected failed finalization attempt %d, got %#v", attempt, results)
		}
	}

	task, err := tasks.GetByTaskID(ctx, "task-finalize-stop")
	if err != nil {
		t.Fatalf("get stopped task: %v", err)
	}
	if task.Status != model.TaskStatusHumanRequired {
		t.Fatalf("expected human_required task, got %s", task.Status)
	}
	run, err := runs.GetByRunID(ctx, "run-finalize-stop")
	if err != nil {
		t.Fatalf("get stopped run: %v", err)
	}
	if run.Status != model.RunStatusBlocked {
		t.Fatalf("expected blocked run, got %s", run.Status)
	}
	submission, err := submissions.GetByRunID(ctx, "run-finalize-stop")
	if err != nil {
		t.Fatalf("get stopped submission: %v", err)
	}
	if submission.Status != model.ResultSubmissionStatusFailed || submission.AttemptCount != 3 {
		t.Fatalf("expected exhausted failed submission, got status=%s attempts=%d", submission.Status, submission.AttemptCount)
	}

	again, err := orch.FinalizeRecoverableSubmissions(ctx, 10)
	if err != nil {
		t.Fatalf("list after exhaustion: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("exhausted submission must leave automatic queue, got %#v", again)
	}
}

func TestFinalizeRecoverableSubmissionsStopsInvalidCanonicalAfterThreeFailures(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.ResultSubmission{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	tasks := repository.NewTaskRepository(db)
	runs := repository.NewRunRepository(db)
	submissions := repository.NewResultSubmissionRepository(db)
	createCallbackArtifactFixture(t, ctx, tasks, runs, "task-invalid-canonical", "run-invalid-canonical")
	submission := &model.ResultSubmission{
		TaskID: "task-invalid-canonical", RunID: "run-invalid-canonical",
		Source: "mcp", Role: "TesterWorker", PayloadHash: "hash", RawPayloadJSON: `{}`, CanonicalJSON: `{invalid`,
	}
	if err := submissions.Accept(ctx, submission, time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("accept invalid canonical fixture: %v", err)
	}
	submission.Status = model.ResultSubmissionStatusFailed
	submission.AttemptCount = 2
	if err := submissions.Update(ctx, submission); err != nil {
		t.Fatalf("create invalid submission: %v", err)
	}

	results, err := New(tasks, runs, WithResultSubmissionRepository(submissions)).FinalizeRecoverableSubmissions(ctx, 10)
	if err != nil {
		t.Fatalf("finalize invalid canonical: %v", err)
	}
	if len(results) != 1 || results[0].Error == "" {
		t.Fatalf("expected invalid canonical failure, got %#v", results)
	}
	if task, _ := tasks.GetByTaskID(ctx, "task-invalid-canonical"); task.Status != model.TaskStatusHumanRequired {
		t.Fatalf("expected human_required task, got %s", task.Status)
	}
	if run, _ := runs.GetByRunID(ctx, "run-invalid-canonical"); run.Status != model.RunStatusBlocked {
		t.Fatalf("expected blocked run, got %s", run.Status)
	}
}

func newResultIngressFixture(t *testing.T) (*Orchestrator, *repository.ResultSubmissionRepository, *repository.TaskRepository, *repository.RunRepository) {
	t.Helper()
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
	orch := New(tasks, runs, WithArtifactRepository(repository.NewArtifactRepository(db)), WithResultSubmissionRepository(submissions))
	return orch, submissions, tasks, runs
}

// TestDeveloperReworkFollowupFailureDoesNotBlockResultApplication 锁定"告知性
// followup"与"结果落账"的依赖方向:任务包缺 merge_request 时通知组装必然报错,
// 但交付结果仍必须落账到终态,而不是被整条否掉。
func TestDeveloperReworkFollowupFailureDoesNotBlockResultApplication(t *testing.T) {
	orch, _, tasks, runs := newResultIngressFixture(t)
	ctx := context.Background()
	runID := "run-rework-followup-failure"
	task := &model.Task{
		TaskID: "task-rework-followup-failure", DedupeKey: "github:mr-rework:1352631679:21",
		Type: model.TaskTypeReworkMergeRequest, Role: "DeveloperWorker",
		SourceRepo: "themarchrain/sdlc-coordinator", TargetRepo: "themarchrain/sdlc-coordinator",
		Status: model.TaskStatusRunning, CurrentRunID: &runID, DeliveryBranch: "feat/account-center",
		ContextJSON: `{"repos":[{"name":"sdlc-coordinator","url":"https://github.com/themarchrain/sdlc-coordinator.git","branch":"main","access":"writable"}]}`,
	}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create rework task: %v", err)
	}
	if err := runs.Create(ctx, run); err != nil {
		t.Fatalf("create rework run: %v", err)
	}

	contract := results.NewDeveloperContract()
	raw := json.RawMessage(`{"task_id":"task-rework-followup-failure","run_id":"run-rework-followup-failure","outcome":"rework_ready","summary":"updated the original MR branch","delivery":{"commit_sha":"a78d201361f203f19a94d0c9f1f7588ace8782c7","changed_files":["web/src/app/SidebarAccountMenu.tsx"],"diff_summary":"1 file changed"},"tests":[{"command":"git diff --check","passed":true,"summary":"clean"}]}`)
	canonical, err := orch.NormalizeRoleResult(ctx, contract, raw)
	if err != nil {
		t.Fatalf("normalize rework result: %v", err)
	}
	response, err := orch.SubmitResult(ctx, SubmitResultRequest{Source: "mcp_developer", ToolName: contract.ToolName(), RawPayload: raw, Canonical: canonical})
	if err != nil {
		t.Fatalf("followup failure must not fail result application: %v", err)
	}
	if response.Task.Status != model.TaskStatusWaitingMerge || response.Run.Status != model.RunStatusSucceeded {
		t.Fatalf("expected waiting_merge task and succeeded run, got %s/%s", response.Task.Status, response.Run.Status)
	}
	if response.Submission.Status != model.ResultSubmissionStatusSucceeded {
		t.Fatalf("expected succeeded submission, got %s", response.Submission.Status)
	}
}
