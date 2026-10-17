package orchestrator

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

// 租户 runner 池派发路由的协调器侧行为：
//  1. Create 经路由给出的端点客户端派发，projection 记录端点（观测亲和）；
//  2. Create 失败后同端点 Get 崩溃恢复（既有语义）；Get 404（确定性未建）
//     才轮换下一端点；
//  3. 歧义失败（传输层错误）不轮换，按既有失败语义终止；
//  4. Observe 按 projection 记录的端点钉住客户端。

type scriptedRunnerClient struct {
	endpoint     string
	createErr    error
	getResult    *runner.Execution
	getErr       error
	events       []runner.Event
	resultResult *runner.Result
	createCalls  int
	getCalls     int
	eventsCalls  int
	resultCalls  int
	lastHash     string
}

func (c *scriptedRunnerClient) Create(_ context.Context, request runner.CreateExecutionRequest) (*runner.Execution, error) {
	c.createCalls++
	if c.createErr != nil {
		return nil, c.createErr
	}
	hash, err := runner.RequestHash(request)
	if err != nil {
		return nil, err
	}
	c.lastHash = hash
	return &runner.Execution{ExecutionID: request.ExecutionID, TaskID: request.TaskID, Role: request.Role, Provider: request.Provider.Name, Status: runner.StatusAccepted, RequestHash: hash}, nil
}

func (c *scriptedRunnerClient) Get(_ context.Context, executionID string) (*runner.Execution, error) {
	c.getCalls++
	if c.getErr != nil {
		return nil, c.getErr
	}
	if c.getResult != nil {
		return c.getResult, nil
	}
	return &runner.Execution{ExecutionID: executionID, Status: runner.StatusAccepted, RequestHash: c.lastHash}, nil
}

func (c *scriptedRunnerClient) Events(_ context.Context, executionID string, _ int64) ([]runner.Event, error) {
	c.eventsCalls++
	return c.events, nil
}

func (c *scriptedRunnerClient) Result(_ context.Context, executionID string) (*runner.Result, error) {
	c.resultCalls++
	return c.resultResult, nil
}

func (c *scriptedRunnerClient) Cancel(_ context.Context, executionID string) (*runner.Execution, error) {
	return &runner.Execution{ExecutionID: executionID}, nil
}

type fakeTenantRouter struct {
	attempt    RunnerAttempt
	attemptErr error
	pinned     map[string]ReviewerRunnerClient
	taskCalls  int
}

func (r *fakeTenantRouter) AttemptForTask(_ context.Context, _ *model.Task) (RunnerAttempt, error) {
	r.taskCalls++
	if r.attemptErr != nil {
		return RunnerAttempt{}, r.attemptErr
	}
	return r.attempt, nil
}

func (r *fakeTenantRouter) ClientForEndpoint(endpoint string) (ReviewerRunnerClient, bool) {
	client, ok := r.pinned[endpoint]
	return client, ok
}

type routingWorld struct {
	coordinator *AgentRunnerCoordinator
	router      *fakeTenantRouter
	task        *model.Task
	run         *model.Run
	request     runner.CreateExecutionRequest
}

func newAgentRunnerRoutingWorld(t *testing.T) *routingWorld {
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
	runID := "run-routing-core"
	task := &model.Task{TaskID: "task-routing-core", DedupeKey: "routing-core", Type: "review_mr", Role: "ReviewerWorker", Status: model.TaskStatusRunning, CurrentRunID: &runID}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := tasks.Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if err := runs.Create(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	router := &fakeTenantRouter{}
	coordinator := NewAgentRunnerCoordinator(tasks, runs, executions, &scriptedRunnerClient{endpoint: "default"})
	coordinator.setTenantRunnerRouter(router)
	request := runner.CreateExecutionRequest{
		ExecutionID: runID, TaskID: task.TaskID, Role: task.Role,
		WorkspacePath: t.TempDir(), PromptFile: "prompt.md", SchemaFile: "result-schema.json", TimeoutSeconds: 1800,
		Provider: runner.ProviderConfig{Name: "fake", Revision: 3, Model: "model-a", Effort: "high"},
	}
	return &routingWorld{coordinator: coordinator, router: router, task: task, run: run, request: request}
}

func TestAgentRunnerCoordinatorDispatchesThroughTenantRouter(t *testing.T) {
	world := newAgentRunnerRoutingWorld(t)
	clientA := &scriptedRunnerClient{endpoint: "http://runner-a"}
	world.router.attempt = RunnerAttempt{Client: clientA, Endpoint: "http://runner-a"}

	projection, err := world.coordinator.Start(context.Background(), world.task, world.run, world.request)
	if err != nil {
		t.Fatalf("start through router: %v", err)
	}
	if clientA.createCalls != 1 {
		t.Fatalf("expected create on runner-a, calls=%d", clientA.createCalls)
	}
	if projection.RunnerEndpoint != "http://runner-a" {
		t.Fatalf("projection did not record pool endpoint: %q", projection.RunnerEndpoint)
	}
}

func TestAgentRunnerCoordinatorRotatesAfterDefinitiveMiss(t *testing.T) {
	world := newAgentRunnerRoutingWorld(t)
	clientA := &scriptedRunnerClient{endpoint: "http://runner-a", createErr: &runner.HTTPError{StatusCode: http.StatusUnprocessableEntity, Body: "invalid create"}}
	clientA.getErr = &runner.HTTPError{Method: "GET", Path: "/api/v1/executions/run-routing-core", StatusCode: http.StatusNotFound}
	clientB := &scriptedRunnerClient{endpoint: "http://runner-b"}
	attemptA := RunnerAttempt{Client: clientA, Endpoint: "http://runner-a"}
	attemptB := RunnerAttempt{Client: clientB, Endpoint: "http://runner-b"}
	attemptA.next = func() (RunnerAttempt, bool) { return attemptB, true }
	world.router.attempt = attemptA

	projection, err := world.coordinator.Start(context.Background(), world.task, world.run, world.request)
	if err != nil {
		t.Fatalf("rotate to runner-b: %v", err)
	}
	if clientA.createCalls != 1 || clientA.getCalls != 1 {
		t.Fatalf("runner-a should see one create and one confirming get: create=%d get=%d", clientA.createCalls, clientA.getCalls)
	}
	if clientB.createCalls != 1 {
		t.Fatalf("runner-b should receive the rotated create, calls=%d", clientB.createCalls)
	}
	if projection.RunnerEndpoint != "http://runner-b" {
		t.Fatalf("projection did not record rotated endpoint: %q", projection.RunnerEndpoint)
	}
}

func TestAgentRunnerCoordinatorDoesNotRotatePinnedRunAfterDefinitiveMiss(t *testing.T) {
	world := newAgentRunnerRoutingWorld(t)
	clientA := &scriptedRunnerClient{endpoint: "http://runner-a", createErr: &runner.HTTPError{StatusCode: http.StatusUnprocessableEntity, Body: "invalid create"}}
	clientA.getErr = &runner.HTTPError{Method: "GET", Path: "/api/v1/executions/run-routing-core", StatusCode: http.StatusNotFound}
	clientB := &scriptedRunnerClient{endpoint: "http://runner-b"}
	attemptA := RunnerAttempt{Client: clientA, Endpoint: "http://runner-a"}
	attemptA.next = func() (RunnerAttempt, bool) { return RunnerAttempt{Client: clientB, Endpoint: "http://runner-b"}, true }
	world.router.attempt = attemptA
	world.router.pinned = map[string]ReviewerRunnerClient{"http://runner-a": clientA}
	world.run.RunnerEndpoint = "http://runner-a"
	if err := world.coordinator.runs.PinRunnerEndpointIfEmpty(context.Background(), world.run.RunID, world.run.RunnerEndpoint); err != nil {
		t.Fatal(err)
	}

	_, err := world.coordinator.Start(context.Background(), world.task, world.run, world.request)
	if err == nil || !strings.Contains(err.Error(), "invalid create") {
		t.Fatalf("pinned run must surface the definitive rejection without rotating, got %v", err)
	}
	if clientA.createCalls != 1 || clientA.getCalls != 1 {
		t.Fatalf("pinned runner should see one create and confirming get: create=%d get=%d", clientA.createCalls, clientA.getCalls)
	}
	if clientB.createCalls != 0 {
		t.Fatalf("pinned run must not be dispatched to another runner, calls=%d", clientB.createCalls)
	}
	stored, err := world.coordinator.executions.GetByRunID(context.Background(), world.run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.RunnerEndpoint != world.run.RunnerEndpoint || stored.Status != model.AgentExecutionStatusFailed {
		t.Fatalf("definitive rejection should settle on the pinned endpoint: %+v", stored)
	}
}

func TestAgentRunnerCoordinatorDoesNotSelectPoolAgainAfterPin(t *testing.T) {
	world := newAgentRunnerRoutingWorld(t)
	clientA := &scriptedRunnerClient{endpoint: "http://runner-a"}
	world.router.attempt = RunnerAttempt{Client: clientA, Endpoint: "http://runner-a"}
	world.router.pinned = map[string]ReviewerRunnerClient{"http://runner-a": clientA}
	ctx := context.Background()
	if _, err := world.coordinator.resolveAndPinEndpoint(ctx, world.task, world.run); err != nil {
		t.Fatal(err)
	}
	if _, err := world.coordinator.Start(ctx, world.task, world.run, world.request); err != nil {
		t.Fatal(err)
	}
	if world.router.taskCalls != 1 || clientA.createCalls != 1 {
		t.Fatalf("pinned dispatch consumed another pool slot: selections=%d creates=%d", world.router.taskCalls, clientA.createCalls)
	}
}

func TestAgentRunnerCoordinatorRefusesRotationOnAmbiguousFailure(t *testing.T) {
	world := newAgentRunnerRoutingWorld(t)
	clientA := &scriptedRunnerClient{endpoint: "http://runner-a", createErr: errors.New("connection refused")}
	clientA.getErr = errors.New("connection refused")
	clientB := &scriptedRunnerClient{endpoint: "http://runner-b"}
	attemptA := RunnerAttempt{Client: clientA, Endpoint: "http://runner-a"}
	attemptA.next = func() (RunnerAttempt, bool) {
		t.Errorf("rotation attempted after ambiguous failure")
		return RunnerAttempt{Client: clientB, Endpoint: "http://runner-b"}, true
	}
	world.router.attempt = attemptA

	// 结果不确定：不轮换、不判失败——意图保持 dispatching 待对账，本函数按
	// 已受理返回（轮询按原端点收敛，确定未接纳前不产生第二个执行）。
	projection, err := world.coordinator.Start(context.Background(), world.task, world.run, world.request)
	if err != nil {
		t.Fatalf("ambiguous outcome must not fail the dispatch: %v", err)
	}
	if projection.Status != model.AgentExecutionStatusDispatching || projection.RunnerEndpoint != "http://runner-a" {
		t.Fatalf("intent must stay dispatching on endpoint-a: %+v", projection)
	}
	if clientA.createCalls != 1 || clientA.getCalls != 1 {
		t.Fatalf("runner-a should see one create and one read-back: create=%d get=%d", clientA.createCalls, clientA.getCalls)
	}
	if clientB.createCalls != 0 {
		t.Fatalf("runner-b must not receive dispatch on ambiguous failure, calls=%d", clientB.createCalls)
	}
}

func TestAgentRunnerCoordinatorObservesOnPinnedEndpoint(t *testing.T) {
	world := newAgentRunnerRoutingWorld(t)
	clientA := &scriptedRunnerClient{endpoint: "http://runner-a"}
	clientB := &scriptedRunnerClient{endpoint: "http://runner-b"}
	world.router.attempt = RunnerAttempt{Client: clientA, Endpoint: "http://runner-a"}
	// 派发落在 runner-a 之外的场景由轮换测试覆盖；这里直接让路由把观测钉到
	// runner-b，验证 Observe 不再打默认/派发客户端。
	world.router.attempt = RunnerAttempt{Client: clientB, Endpoint: "http://runner-b"}
	world.router.pinned = map[string]ReviewerRunnerClient{"http://runner-b": clientB}

	if _, err := world.coordinator.Start(context.Background(), world.task, world.run, world.request); err != nil {
		t.Fatalf("start: %v", err)
	}
	projection, err := world.coordinator.Observe(context.Background(), world.run.RunID)
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if projection.Execution.RunnerEndpoint != "http://runner-b" {
		t.Fatalf("unexpected stored endpoint: %q", projection.Execution.RunnerEndpoint)
	}
	if clientB.eventsCalls == 0 {
		t.Fatalf("observe did not hit the pinned endpoint client")
	}
	if clientA.eventsCalls != 0 || clientA.getCalls != 0 {
		t.Fatalf("observe leaked to the wrong endpoint client: events=%d get=%d", clientA.eventsCalls, clientA.getCalls)
	}
}

func TestAgentRunnerCoordinatorSurfacesFirstCreateErrorWhenPoolExhausted(t *testing.T) {
	world := newAgentRunnerRoutingWorld(t)
	clientA := &scriptedRunnerClient{endpoint: "http://runner-a", createErr: &runner.HTTPError{StatusCode: http.StatusUnprocessableEntity, Body: "runner-a refused"}}
	clientA.getErr = &runner.HTTPError{Method: "GET", StatusCode: http.StatusNotFound}
	clientB := &scriptedRunnerClient{endpoint: "http://runner-b", createErr: &runner.HTTPError{StatusCode: http.StatusUnprocessableEntity, Body: "runner-b refused"}}
	clientB.getErr = &runner.HTTPError{Method: "GET", StatusCode: http.StatusNotFound}
	attemptA := RunnerAttempt{Client: clientA, Endpoint: "http://runner-a"}
	attemptA.next = func() (RunnerAttempt, bool) { return RunnerAttempt{Client: clientB, Endpoint: "http://runner-b"}, true }
	world.router.attempt = attemptA

	_, err := world.coordinator.Start(context.Background(), world.task, world.run, world.request)
	if err == nil || !strings.Contains(err.Error(), "runner-a refused") {
		t.Fatalf("exhausted pool must surface the FIRST endpoint error, got %v", err)
	}
}

func TestAgentRunnerCoordinatorSurfacesRouterFailure(t *testing.T) {
	world := newAgentRunnerRoutingWorld(t)
	world.router.attemptErr = errors.New("control plane unavailable")

	if _, err := world.coordinator.Start(context.Background(), world.task, world.run, world.request); err == nil {
		t.Fatalf("expected router failure to abort dispatch")
	}
}
