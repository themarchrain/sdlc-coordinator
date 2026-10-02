package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/resulttoken"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/workspace"
)

func TestWithResultIngressWiresTesterRunnerWithRunScopedChannel(t *testing.T) {
	ctx := context.Background()
	fixture := newTesterIngressFixture(t)
	issuer := newStaticRunTokenIssuer(t)
	const endpoint = "http://coordinator.example:8080/mcp"

	orch := New(fixture.tasks, fixture.runs,
		WithReviewerRunner(NewReviewerRunnerCoordinator(fixture.tasks, fixture.runs, fixture.executions, nil, fixture.reviewerClient, &recordingResultSubmitter{}), ReviewerRunnerSettings{}),
		WithDeveloperRunner(NewDeveloperRunnerCoordinator(fixture.tasks, fixture.runs, fixture.executions, nil, nil, fixture.developerClient, &recordingResultSubmitter{}, ReviewerRunnerSettings{}), ReviewerRunnerSettings{}),
		WithTesterRunner(fixture.tester, ReviewerRunnerSettings{}),
		WithResultIngress(issuer, endpoint),
	)
	if !(orch.reviewerRunner.(*ReviewerRunnerCoordinator).core != orch.developerRunner.(*DeveloperRunnerCoordinator).core &&
		orch.developerRunner.(*DeveloperRunnerCoordinator).core != fixture.tester.core &&
		orch.reviewerRunner.(*ReviewerRunnerCoordinator).core != fixture.tester.core) {
		t.Fatal("role coordinators must use three distinct runner cores")
	}

	for _, roleCase := range []struct {
		role string
		core *AgentRunnerCoordinator
	}{
		{role: "ReviewerWorker", core: orch.reviewerRunner.(*ReviewerRunnerCoordinator).core},
		{role: "DeveloperWorker", core: orch.developerRunner.(*DeveloperRunnerCoordinator).core},
	} {
		record := fixture.records[roleCase.role]
		task, run := record.task, record.run
		if _, err := roleCase.core.Start(ctx, task, run, ingressRequest(task, run)); err != nil {
			t.Fatalf("start %s execution: %v", roleCase.role, err)
		}
	}

	record := fixture.records["TesterWorker"]
	task, run := record.task, record.run
	if _, err := fixture.tester.Start(ctx, testerIngressStartRequest(task, run, t.TempDir())); err != nil {
		t.Fatalf("start TesterWorker execution: %v", err)
	}
	channel := fixture.testerClient.createRequest.ResultSubmission
	if channel == nil {
		t.Fatal("TesterWorker dispatch request carries no submission channel")
	}
	if channel.EndpointURL != endpoint {
		t.Fatalf("unexpected tester endpoint: %q", channel.EndpointURL)
	}
	claims, err := resulttoken.Verify(issuer.secret, channel.BearerToken, time.Now())
	if err != nil {
		t.Fatalf("verify tester run token: %v", err)
	}
	if claims.TaskID != task.TaskID || claims.RunID != run.RunID || claims.Role != "TesterWorker" {
		t.Fatalf("tester token does not bind the run role and identity: %#v", claims)
	}
	if issuer.lastTaskTimeout != 45*time.Minute {
		t.Fatalf("issuer must receive effective dispatch timeout, got %s", issuer.lastTaskTimeout)
	}
	if fixture.testerClient.createRequest.TimeoutSeconds != int64((45*time.Minute)/time.Second) {
		t.Fatalf("runner request must carry effective dispatch timeout, got %d seconds", fixture.testerClient.createRequest.TimeoutSeconds)
	}
	for _, client := range []*fakeReviewerRunnerClient{fixture.reviewerClient, fixture.developerClient} {
		if client.createRequest.ResultSubmission == nil || client.createRequest.ResultSubmission.EndpointURL != endpoint {
			t.Fatalf("Reviewer/Developer ingress wiring regressed: %#v", client.createRequest.ResultSubmission)
		}
	}
}

func TestWithResultIngressTesterSignerErrorPreventsCreate(t *testing.T) {
	fixture := newTesterIngressFixture(t)
	newTesterIngressOrchestrator(fixture, &failingRunTokenIssuer{}, "http://coordinator.example:8080/mcp", true)
	record := fixture.records["TesterWorker"]
	task, run := record.task, record.run
	_, err := fixture.tester.Start(context.Background(), testerIngressStartRequest(task, run, t.TempDir()))
	if err == nil {
		t.Fatal("configured token signer failure must fail TesterWorker dispatch")
	}
	if fixture.testerClient.createCalls != 0 {
		t.Fatalf("runner Create must not be called when token signing fails, got %d calls", fixture.testerClient.createCalls)
	}
}

func TestWithResultIngressTesterIdempotentReplayDoesNotResign(t *testing.T) {
	fixture := newTesterIngressFixture(t)
	issuer := newStaticRunTokenIssuer(t)
	newTesterIngressOrchestrator(fixture, issuer, "http://coordinator.example:8080/mcp", true)
	record := fixture.records["TesterWorker"]
	task, run := record.task, record.run
	request := testerIngressStartRequest(task, run, t.TempDir())
	for i := 0; i < 2; i++ {
		if _, err := fixture.tester.Start(context.Background(), request); err != nil {
			t.Fatalf("start TesterWorker attempt %d: %v", i+1, err)
		}
	}
	if issuer.calls != 1 || fixture.testerClient.createCalls != 1 {
		t.Fatalf("same-run replay must reuse execution without re-signing: signs=%d creates=%d", issuer.calls, fixture.testerClient.createCalls)
	}
}

func TestWithResultIngressTesterWithoutIssuerOrEndpointUsesJSONMode(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		issuer RunTokenIssuer
		url    string
	}{
		{name: "no issuer", url: "http://coordinator.example:8080/mcp"},
		{name: "empty endpoint", issuer: newStaticRunTokenIssuer(t)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newTesterIngressFixture(t)
			newTesterIngressOrchestrator(fixture, testCase.issuer, testCase.url, true)
			record := fixture.records["TesterWorker"]
			task, run := record.task, record.run
			if _, err := fixture.tester.Start(context.Background(), testerIngressStartRequest(task, run, t.TempDir())); err != nil {
				t.Fatalf("start TesterWorker execution: %v", err)
			}
			if fixture.testerClient.createRequest.ResultSubmission != nil {
				t.Fatalf("result submission channel must stay closed in JSON mode: %#v", fixture.testerClient.createRequest.ResultSubmission)
			}
		})
	}
}

func TestWithResultIngressAllowsOmittedTesterCoordinator(t *testing.T) {
	fixture := newTesterIngressFixture(t)
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("optional Tester coordinator must remain compatible: %v", recovered)
		}
	}()
	_ = New(fixture.tasks, fixture.runs,
		WithReviewerRunner(NewReviewerRunnerCoordinator(fixture.tasks, fixture.runs, fixture.executions, nil, fixture.reviewerClient, &recordingResultSubmitter{}), ReviewerRunnerSettings{}),
		WithDeveloperRunner(NewDeveloperRunnerCoordinator(fixture.tasks, fixture.runs, fixture.executions, nil, nil, fixture.developerClient, &recordingResultSubmitter{}, ReviewerRunnerSettings{}), ReviewerRunnerSettings{}),
		WithResultIngress(newStaticRunTokenIssuer(t), "http://coordinator.example:8080/mcp"),
	)
}

func TestTesterReconcileAbsorbsFailureAfterMCPFinalizedRun(t *testing.T) {
	fixture := newTesterIngressFixture(t)
	newTesterIngressOrchestrator(fixture, newStaticRunTokenIssuer(t), "http://coordinator.example:8080/mcp", true)
	record := fixture.records["TesterWorker"]
	if _, err := fixture.tester.Start(context.Background(), testerIngressStartRequest(record.task, record.run, t.TempDir())); err != nil {
		t.Fatalf("start TesterWorker execution: %v", err)
	}

	storedRun, err := fixture.runs.GetByRunID(context.Background(), record.run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	storedRun.Status = model.RunStatusSucceeded
	if err := fixture.runs.Update(context.Background(), storedRun); err != nil {
		t.Fatalf("finalize run after MCP acceptance: %v", err)
	}
	fixture.testerClient.execution.Status = runner.StatusFailed
	fixture.testerClient.execution.ErrorCode = "provider_failed"

	observed, err := fixture.tester.Reconcile(context.Background(), record.run.RunID)
	if err != nil {
		t.Fatalf("reconcile failed TesterWorker execution: %v", err)
	}
	if !observed.Absorbed {
		t.Fatalf("execution failure after MCP finalization must be absorbed: %#v", observed)
	}
	storedRun, err = fixture.runs.GetByRunID(context.Background(), record.run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if storedRun.Status != model.RunStatusSucceeded {
		t.Fatalf("MCP-finalized run status must stay succeeded, got %s", storedRun.Status)
	}
	if fixture.submitter.calls != 0 {
		t.Fatalf("absorbed TesterWorker failure must not resubmit: %d calls", fixture.submitter.calls)
	}
}

type testerIngressFixture struct {
	tasks      *repository.TaskRepository
	runs       *repository.RunRepository
	executions *repository.AgentExecutionRepository
	records    map[string]testerIngressRecord
	// The concrete pointers are retained to prove role cores remain independent.
	reviewerClient  *fakeReviewerRunnerClient
	developerClient *fakeReviewerRunnerClient
	testerClient    *fakeReviewerRunnerClient
	submitter       *recordingResultSubmitter
	tester          *TesterRunnerCoordinator
}

type testerIngressRecord struct {
	task *model.Task
	run  *model.Run
}

func newTesterIngressFixture(t *testing.T) testerIngressFixture {
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
	fixture := testerIngressFixture{
		tasks: tasks, runs: runs, executions: executions,
		records:        make(map[string]testerIngressRecord),
		reviewerClient: &fakeReviewerRunnerClient{}, developerClient: &fakeReviewerRunnerClient{}, testerClient: &fakeReviewerRunnerClient{}, submitter: &recordingResultSubmitter{},
	}
	for _, role := range []string{"ReviewerWorker", "DeveloperWorker", "TesterWorker"} {
		taskID, runID := "task-"+role, "run-"+role
		task := &model.Task{
			TaskID: taskID, DedupeKey: "dedupe-" + taskID, Type: "verify_merge_request", Role: role,
			TargetRepo: "group/project", SourceRepo: "group/project", Objective: "verify the change", AcceptanceCriteria: "report evidence",
			Status: model.TaskStatusRunning, CurrentRunID: &runID,
			ContextJSON: `{"project":{"path":"group/project"},"merge_request":{"iid":21,"url":"https://gitlab.example/group/project/-/merge_requests/21","source_branch":"feature/change","target_branch":"master","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"feature/change","access":"readable"}]}`,
		}
		run := &model.Run{RunID: runID, TaskID: taskID, WorkerRole: role, Status: model.RunStatusRunning}
		if err := tasks.Create(context.Background(), task); err != nil {
			t.Fatalf("create %s task: %v", role, err)
		}
		if err := runs.Create(context.Background(), run); err != nil {
			t.Fatalf("create %s run: %v", role, err)
		}
		fixture.records[role] = testerIngressRecord{task: task, run: run}
	}
	fixture.tester = NewTesterRunnerCoordinator(tasks, runs, executions, &testerIngressWorkspacePreparer{}, fixture.testerClient, fixture.submitter)
	return fixture
}

func newTesterIngressOrchestrator(fixture testerIngressFixture, issuer RunTokenIssuer, endpoint string, includeTester bool) *Orchestrator {
	options := []Option{
		WithReviewerRunner(NewReviewerRunnerCoordinator(fixture.tasks, fixture.runs, fixture.executions, nil, fixture.reviewerClient, &recordingResultSubmitter{}), ReviewerRunnerSettings{}),
		WithDeveloperRunner(NewDeveloperRunnerCoordinator(fixture.tasks, fixture.runs, fixture.executions, nil, nil, fixture.developerClient, &recordingResultSubmitter{}, ReviewerRunnerSettings{}), ReviewerRunnerSettings{}),
	}
	if includeTester {
		options = append(options, WithTesterRunner(fixture.tester, ReviewerRunnerSettings{}))
	}
	options = append(options, WithResultIngress(issuer, endpoint))
	return New(fixture.tasks, fixture.runs, options...)
}

func testerIngressStartRequest(task *model.Task, run *model.Run, root string) TesterRunnerStartRequest {
	return TesterRunnerStartRequest{
		Task: task, Run: run,
		Workspace: workspace.ReviewerRequest{
			WorkspaceRoot: root, Project: task.TargetRepo, RepositoryURL: "https://gitlab.example/group/project.git",
			SourceBranch: "feature/change", TargetBranch: "master", HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			RequestedReviewMode: workspace.ReviewModeFull,
		},
		Provider: runner.ProviderConfig{Name: "fake", Revision: 3}, Timeout: 45 * time.Minute,
	}
}

type testerIngressWorkspacePreparer struct{}

func (*testerIngressWorkspacePreparer) Prepare(_ context.Context, request workspace.ReviewerRequest) (*workspace.Manifest, error) {
	root := filepath.Join(request.WorkspaceRoot, "runs", request.RunID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &workspace.Manifest{Status: workspace.ManifestStatusReady, TaskID: request.TaskID, RunID: request.RunID, Role: "TesterWorker"}, nil
}
