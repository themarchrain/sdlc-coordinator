package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/resulttoken"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

type staticRunTokenIssuer struct {
	secret []byte
	calls  int
}

func newStaticRunTokenIssuer(t *testing.T) *staticRunTokenIssuer {
	t.Helper()
	secret, err := resulttoken.NewSecret()
	if err != nil {
		t.Fatalf("new secret: %v", err)
	}
	return &staticRunTokenIssuer{secret: secret}
}

func (s *staticRunTokenIssuer) IssueRunToken(taskID, runID, role string) (string, error) {
	s.calls++
	return resulttoken.Issue(s.secret, resulttoken.Claims{TaskID: taskID, RunID: runID, Role: role, TTL: time.Hour}, time.Now())
}

func newDispatchIngressFixture(t *testing.T, issuer RunTokenIssuer, endpointURL string) (*AgentRunnerCoordinator, *fakeReviewerRunnerClient, *model.Task, *model.Run) {
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
	runID := "run-result-ingress"
	task := &model.Task{TaskID: "task-result-ingress", DedupeKey: "result-ingress", Type: "review_mr", Role: "ReviewerWorker", Status: model.TaskStatusRunning, CurrentRunID: &runID}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := tasks.Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if err := runs.Create(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	client := &fakeReviewerRunnerClient{}
	coordinator := NewAgentRunnerCoordinator(tasks, runs, executions, client)
	coordinator.setResultIngress(issuer, endpointURL)
	return coordinator, client, task, run
}

func ingressRequest(task *model.Task, run *model.Run) runner.CreateExecutionRequest {
	return runner.CreateExecutionRequest{
		ExecutionID: run.RunID, TaskID: task.TaskID, Role: task.Role,
		WorkspacePath: "D:/tmp/ws/runs/" + run.RunID, PromptFile: "prompt.md", SchemaFile: "result-schema.json", TimeoutSeconds: 1800,
		Provider: runner.ProviderConfig{Name: "fake", Revision: 3},
	}
}

func TestAgentRunnerStartSignsPerRunTokenIntoDispatchRequest(t *testing.T) {
	issuer := newStaticRunTokenIssuer(t)
	coordinator, client, task, run := newDispatchIngressFixture(t, issuer, "http://coordinator.example:8080/mcp")

	if _, err := coordinator.Start(context.Background(), task, run, ingressRequest(task, run)); err != nil {
		t.Fatalf("start execution: %v", err)
	}
	channel := client.createRequest.ResultSubmission
	if channel == nil {
		t.Fatal("dispatch request carries no submission channel")
	}
	if channel.EndpointURL != "http://coordinator.example:8080/mcp" {
		t.Fatalf("unexpected endpoint: %q", channel.EndpointURL)
	}
	claims, err := resulttoken.Verify(issuer.secret, channel.BearerToken, time.Now())
	if err != nil {
		t.Fatalf("verify dispatched token: %v", err)
	}
	if claims.TaskID != task.TaskID || claims.RunID != run.RunID || claims.Role != task.Role {
		t.Fatalf("token claims do not bind the run: %#v", claims)
	}
}

func TestAgentRunnerStartSkipsTokenOnIdempotentReplay(t *testing.T) {
	issuer := newStaticRunTokenIssuer(t)
	coordinator, _, task, run := newDispatchIngressFixture(t, issuer, "http://coordinator.example:8080/mcp")
	request := ingressRequest(task, run)
	if _, err := coordinator.Start(context.Background(), task, run, request); err != nil {
		t.Fatalf("start execution: %v", err)
	}
	firstCalls := issuer.calls
	if _, err := coordinator.Start(context.Background(), task, run, request); err != nil {
		t.Fatalf("repeat start: %v", err)
	}
	if issuer.calls != firstCalls {
		t.Fatalf("idempotent replay must not re-sign tokens: %d -> %d", firstCalls, issuer.calls)
	}
}

func TestAgentRunnerStartOmitsChannelWithoutIssuer(t *testing.T) {
	coordinator, client, task, run := newDispatchIngressFixture(t, nil, "")
	if _, err := coordinator.Start(context.Background(), task, run, ingressRequest(task, run)); err != nil {
		t.Fatalf("start execution: %v", err)
	}
	if client.createRequest.ResultSubmission != nil {
		t.Fatalf("channel must stay closed without issuer: %#v", client.createRequest.ResultSubmission)
	}
}

func TestAgentRunnerStartFailsClosedOnIssuerError(t *testing.T) {
	issuer := &failingRunTokenIssuer{}
	coordinator, _, task, run := newDispatchIngressFixture(t, issuer, "http://coordinator.example:8080/mcp")
	if _, err := coordinator.Start(context.Background(), task, run, ingressRequest(task, run)); err == nil {
		t.Fatal("issuer failure must fail the dispatch")
	}
}

type failingRunTokenIssuer struct{}

func (f *failingRunTokenIssuer) IssueRunToken(string, string, string) (string, error) {
	return "", errors.New("issuer unavailable")
}
