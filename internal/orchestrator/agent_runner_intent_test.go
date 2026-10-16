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

	"gorm.io/gorm"
)

// endpointRunner 是端点级假 Runner：记录每端点调用，可注入失败，执行态可
// 预置（模拟远端已受理但响应丢失/投影未落的既有执行）。
type endpointRunner struct {
	endpoint    string
	createCalls int
	getCalls    int
	execution   *runner.Execution
	createErr   error
	getErr      error
	onCreate    func(request runner.CreateExecutionRequest)
}

func (f *endpointRunner) Create(_ context.Context, request runner.CreateExecutionRequest) (*runner.Execution, error) {
	f.createCalls++
	if f.onCreate != nil {
		f.onCreate(request)
	}
	if f.createErr != nil {
		return nil, f.createErr
	}
	hash, err := runner.RequestHash(request)
	if err != nil {
		return nil, err
	}
	f.execution = &runner.Execution{
		ExecutionID: request.ExecutionID, TaskID: request.TaskID, Role: request.Role,
		Provider: request.Provider.Name, Status: runner.StatusRunning, RequestHash: hash,
	}
	copy := *f.execution
	return &copy, nil
}

func (f *endpointRunner) Get(_ context.Context, executionID string) (*runner.Execution, error) {
	f.getCalls++
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.execution == nil || f.execution.ExecutionID != executionID {
		return nil, &runner.HTTPError{StatusCode: http.StatusNotFound}
	}
	copy := *f.execution
	return &copy, nil
}

func (f *endpointRunner) Events(context.Context, string, int64) ([]runner.Event, error) {
	return nil, nil
}

func (f *endpointRunner) Result(context.Context, string) (*runner.Result, error) { return nil, nil }

func (f *endpointRunner) Cancel(context.Context, string) (*runner.Execution, error) { return nil, nil }

// fixedRouter 按固定顺序路由端点，ClientForEndpoint 按记录端点解析。
type fixedRouter struct {
	attempt RunnerAttempt
	byEndpt map[string]ReviewerRunnerClient
}

func (r *fixedRouter) AttemptForTask(context.Context, *model.Task) (RunnerAttempt, error) {
	return r.attempt, nil
}

func (r *fixedRouter) ClientForEndpoint(endpoint string) (ReviewerRunnerClient, bool) {
	client, ok := r.byEndpt[endpoint]
	return client, ok
}

type intentWorld struct {
	t          *testing.T
	db         *gorm.DB
	tasks      *repository.TaskRepository
	runs       *repository.RunRepository
	executions *repository.AgentExecutionRepository
	coord      *AgentRunnerCoordinator
	router     *fixedRouter
	runnerA    *endpointRunner
	runnerB    *endpointRunner
	task       *model.Task
	run        *model.Run
	request    runner.CreateExecutionRequest
}

func newIntentWorld(t *testing.T) *intentWorld {
	t.Helper()
	db, err := store.Open("file:" + model.NewID("intent") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.AgentExecutionEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	tasks := repository.NewTaskRepository(db)
	runs := repository.NewRunRepository(db)
	executions := repository.NewAgentExecutionRepository(db)
	runnerA := &endpointRunner{endpoint: "runner-a"}
	runnerB := &endpointRunner{endpoint: "runner-b"}
	attempt := NewRunnerAttempt(runnerA, "runner-a", func() (RunnerAttempt, bool) {
		return RunnerAttempt{Client: runnerB, Endpoint: "runner-b"}, true
	})
	router := &fixedRouter{
		attempt: attempt,
		byEndpt: map[string]ReviewerRunnerClient{"runner-a": runnerA, "runner-b": runnerB},
	}
	coord := NewAgentRunnerCoordinator(tasks, runs, executions, runnerA)
	coord.setTenantRunnerRouter(router)

	task := &model.Task{TaskID: "task_intent", DedupeKey: "intent:1", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusRunning}
	run := &model.Run{RunID: "run_intent", TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	task.CurrentRunID = &run.RunID
	ctx := context.Background()
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := runs.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	request := runner.CreateExecutionRequest{
		ExecutionID: run.RunID, TaskID: task.TaskID, Role: task.Role,
		WorkspacePath: "/app/workspaces/runs/" + run.RunID, PromptFile: "prompt.md", SchemaFile: "schema.json",
		TimeoutSeconds: 600,
		Provider:       runner.ProviderConfig{Name: "claude-cli", Revision: 3, Model: "model-a"},
	}
	return &intentWorld{t: t, db: db, tasks: tasks, runs: runs, executions: executions, coord: coord, router: router, runnerA: runnerA, runnerB: runnerB, task: task, run: run, request: request}
}

func (w *intentWorld) insertIntent(t *testing.T, endpoint, hash string) {
	t.Helper()
	now := w.run.StartedAt
	_ = now
	intent := &model.AgentExecution{
		RunID: w.run.RunID, ExecutionID: w.run.RunID, Provider: "claude-cli",
		ProviderConfigRevision: 3, ProviderModel: "model-a", RunnerEndpoint: endpoint,
		Status: model.AgentExecutionStatusDispatching, RequestHash: hash,
	}
	if err := w.executions.Create(context.Background(), intent); err != nil {
		t.Fatalf("insert intent: %v", err)
	}
}

// TestStartPersistsIntentBeforeRemoteCreate 先落目标再调用：远端 Create 发起时
// 意图行已在库（dispatching、目标端点、请求哈希与本次请求一致）。
func TestStartPersistsIntentBeforeRemoteCreate(t *testing.T) {
	w := newIntentWorld(t)
	ctx := context.Background()
	hash, err := runner.RequestHash(w.request)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	var observed *model.AgentExecution
	var observedErr error
	w.runnerA.onCreate = func(runner.CreateExecutionRequest) {
		observed, observedErr = w.executions.GetByRunID(ctx, w.run.RunID)
	}

	projection, err := w.coord.Start(ctx, w.task, w.run, w.request)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if observedErr != nil || observed == nil {
		t.Fatalf("intent not persisted before remote create: %v", observedErr)
	}
	if observed.Status != model.AgentExecutionStatusDispatching || observed.RunnerEndpoint != "runner-a" || observed.RequestHash != hash {
		t.Fatalf("intent shape wrong: %+v", observed)
	}
	if projection.Status != model.AgentExecutionStatusRunning || projection.RunnerEndpoint != "runner-a" {
		t.Fatalf("projection = %+v", projection)
	}
	var rows int64
	if err := w.db.Model(&model.AgentExecution{}).Count(&rows).Error; err != nil || rows != 1 {
		t.Fatalf("execution rows = %d err=%v, want 1 (intent reused, not duplicated)", rows, err)
	}
}

// TestStartRecoversAcceptedIntentWithoutSwitchingRunner 远端已受理、投影未落
// 的崩溃窗口：恢复按原端点回读收敛，不换 Runner、不产生第二个执行。
func TestStartRecoversAcceptedIntentWithoutSwitchingRunner(t *testing.T) {
	w := newIntentWorld(t)
	ctx := context.Background()
	hash, err := runner.RequestHash(w.request)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	w.runnerA.execution = &runner.Execution{
		ExecutionID: w.run.RunID, TaskID: w.task.TaskID, Role: w.task.Role,
		Provider: "claude-cli", Status: runner.StatusRunning, RequestHash: hash,
	}
	w.insertIntent(t, "runner-a", hash)

	projection, err := w.coord.Start(ctx, w.task, w.run, w.request)
	if err != nil {
		t.Fatalf("start recovery: %v", err)
	}
	if projection.Status != model.AgentExecutionStatusRunning {
		t.Fatalf("projection status = %s", projection.Status)
	}
	if w.runnerA.createCalls != 0 {
		t.Fatalf("recovery must read back instead of re-creating (create calls = %d)", w.runnerA.createCalls)
	}
	if w.runnerB.createCalls != 0 || w.runnerB.getCalls != 0 {
		t.Fatalf("recovery touched another runner: create=%d get=%d", w.runnerB.createCalls, w.runnerB.getCalls)
	}
	var rows int64
	if err := w.db.Model(&model.AgentExecution{}).Count(&rows).Error; err != nil || rows != 1 {
		t.Fatalf("execution rows = %d err=%v (second execution created?)", rows, err)
	}
}

func TestStartKeepsPersistedIntentWhenRunnerReturns404(t *testing.T) {
	w := newIntentWorld(t)
	ctx := context.Background()
	hash, err := runner.RequestHash(w.request)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	w.insertIntent(t, "runner-a", hash)

	_, err = w.coord.Start(ctx, w.task, w.run, w.request)
	if !errors.Is(err, errDispatchOutcomeUnknown) {
		t.Fatalf("start error = %v, want unknown dispatch outcome", err)
	}
	stored, err := w.executions.GetByRunID(ctx, w.run.RunID)
	if err != nil || stored.Status != model.AgentExecutionStatusDispatching || stored.RunnerEndpoint != "runner-a" {
		t.Fatalf("persisted intent changed after runner 404: execution=%+v err=%v", stored, err)
	}
	if w.runnerA.createCalls != 0 {
		t.Fatalf("runner 404 must not trigger a second create (calls = %d)", w.runnerA.createCalls)
	}
	if w.runnerB.createCalls != 0 || w.runnerB.getCalls != 0 {
		t.Fatalf("unknown outcome rotated endpoints: create=%d get=%d", w.runnerB.createCalls, w.runnerB.getCalls)
	}
}

// TestStartKeepsIntentWhenOutcomeUnknown 传输层歧义失败（Create 与回读都失败）：
// 保持意图待对账并按已受理返回，不判失败、不轮换端点。
func TestStartKeepsIntentWhenOutcomeUnknown(t *testing.T) {
	w := newIntentWorld(t)
	ctx := context.Background()
	transport := errors.New("connection reset by peer")
	w.runnerA.createErr = transport
	w.runnerA.getErr = transport

	projection, err := w.coord.Start(ctx, w.task, w.run, w.request)
	if err != nil {
		t.Fatalf("ambiguous outcome must not fail the dispatch: %v", err)
	}
	if projection.Status != model.AgentExecutionStatusDispatching {
		t.Fatalf("projection status = %s, want dispatching", projection.Status)
	}
	stored, err := w.executions.GetByRunID(ctx, w.run.RunID)
	if err != nil || stored.Status != model.AgentExecutionStatusDispatching || stored.RunnerEndpoint != "runner-a" {
		t.Fatalf("intent not kept for reconciliation: %+v err=%v", stored, err)
	}
	if w.runnerB.createCalls != 0 || w.runnerB.getCalls != 0 {
		t.Fatalf("ambiguous outcome must not rotate: create=%d get=%d", w.runnerB.createCalls, w.runnerB.getCalls)
	}
}

// TestStartSettlesDefinitelyRejectedIntent 全部端点确定性未接纳（Create 失败、
// 回读 404、池耗尽）：意图行落为 failed 并返回错误，任务失败路径可照常重试。
func TestStartSettlesDefinitelyRejectedIntent(t *testing.T) {
	w := newIntentWorld(t)
	ctx := context.Background()
	// 单端点池：无下一个候选。
	w.router.attempt = NewRunnerAttempt(w.runnerA, "runner-a", nil)
	w.runnerA.createErr = &runner.HTTPError{StatusCode: http.StatusUnprocessableEntity, Body: "runner rejected the create"}

	_, err := w.coord.Start(ctx, w.task, w.run, w.request)
	if err == nil {
		t.Fatal("definite rejection must surface as an error")
	}
	stored, lookupErr := w.executions.GetByRunID(ctx, w.run.RunID)
	if lookupErr != nil {
		t.Fatalf("intent missing after settle: %v", lookupErr)
	}
	if stored.Status != model.AgentExecutionStatusFailed || !strings.Contains(stored.ErrorMessage, "runner rejected the create") {
		t.Fatalf("intent not settled as failed: %+v", stored)
	}
}

// TestObserveDispatchingIntent 观测对派发意图的两个分支：远端已受理则收敛
// 为远端状态；Runner 404 不能证明持久卷未丢失，因此保留 unknown 供人工/恢复对账。
func TestObserveDispatchingIntent(t *testing.T) {
	ctx := context.Background()

	t.Run("accepted", func(t *testing.T) {
		w := newIntentWorld(t)
		hash, err := runner.RequestHash(w.request)
		if err != nil {
			t.Fatalf("hash: %v", err)
		}
		w.runnerA.execution = &runner.Execution{
			ExecutionID: w.run.RunID, TaskID: w.task.TaskID, Role: w.task.Role,
			Provider: "claude-cli", Status: runner.StatusRunning, RequestHash: hash,
		}
		w.insertIntent(t, "runner-a", hash)
		observation, err := w.coord.Observe(ctx, w.run.RunID)
		if err != nil {
			t.Fatalf("observe: %v", err)
		}
		if observation.Execution.Status != model.AgentExecutionStatusRunning {
			t.Fatalf("converged status = %s", observation.Execution.Status)
		}
		stored, err := w.executions.GetByRunID(ctx, w.run.RunID)
		if err != nil || stored.Status != model.AgentExecutionStatusRunning {
			t.Fatalf("stored status = %+v err=%v", stored, err)
		}
	})

	t.Run("not accepted", func(t *testing.T) {
		w := newIntentWorld(t)
		hash, err := runner.RequestHash(w.request)
		if err != nil {
			t.Fatalf("hash: %v", err)
		}
		w.insertIntent(t, "runner-a", hash)
		_, err = w.coord.Observe(ctx, w.run.RunID)
		if !errors.Is(err, errDispatchOutcomeUnknown) {
			t.Fatalf("observe err = %v, want dispatch outcome unknown", err)
		}
		stored, lookupErr := w.executions.GetByRunID(ctx, w.run.RunID)
		if lookupErr != nil || stored.Status != model.AgentExecutionStatusDispatching {
			t.Fatalf("intent must stay dispatching: %+v err=%v", stored, lookupErr)
		}
	})
}

func TestTerminalRunWithUnknownIntentDoesNotStartRunner(t *testing.T) {
	w := newIntentWorld(t)
	ctx := context.Background()
	hash, err := runner.RequestHash(w.request)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	w.insertIntent(t, "runner-a", hash)
	w.run.Status = model.RunStatusSucceeded
	w.run.ExternalSystem = "agent-runner"
	if err := w.runs.Update(ctx, w.run); err != nil {
		t.Fatalf("finalize run: %v", err)
	}
	active, err := w.runs.ListTerminalWithActiveExecution(ctx, 10)
	if err != nil || len(active) != 1 {
		t.Fatalf("dispatching terminal run should be swept: runs=%v err=%v", active, err)
	}
	if _, err := w.coord.Observe(ctx, w.run.RunID); !errors.Is(err, errDispatchOutcomeUnknown) {
		t.Fatalf("observe terminal intent err = %v, want unknown outcome", err)
	}
	stored, err := w.executions.GetByRunID(ctx, w.run.RunID)
	if err != nil || stored.Status != model.AgentExecutionStatusDispatching || w.runnerA.createCalls != 0 {
		t.Fatalf("terminal intent must stay unresolved without relaunch: execution=%+v creates=%d err=%v", stored, w.runnerA.createCalls, err)
	}
	active, err = w.runs.ListTerminalWithActiveExecution(ctx, 10)
	if err != nil || len(active) != 1 {
		t.Fatalf("unknown terminal intent must remain visible for manual recovery: runs=%v err=%v", active, err)
	}
	if _, err := w.coord.Start(ctx, w.task, w.run, w.request); !errors.Is(err, errDispatchOutcomeUnknown) {
		t.Fatalf("start terminal unknown intent err = %v, want unknown outcome", err)
	}
	if w.runnerA.createCalls != 0 {
		t.Fatalf("start recreated terminal execution: %d", w.runnerA.createCalls)
	}
}

func TestStartDoesNotReplayUnknownIntentAfterRunFinalizes(t *testing.T) {
	w := newIntentWorld(t)
	ctx := context.Background()
	hash, err := runner.RequestHash(w.request)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	w.insertIntent(t, "runner-a", hash)
	w.run.Status = model.RunStatusSucceeded
	if err := w.runs.Update(ctx, w.run); err != nil {
		t.Fatalf("finalize run: %v", err)
	}
	if _, err := w.coord.Start(ctx, w.task, w.run, w.request); !errors.Is(err, errDispatchOutcomeUnknown) {
		t.Fatalf("reconcile terminal intent err = %v, want unknown outcome", err)
	}
	stored, err := w.executions.GetByRunID(ctx, w.run.RunID)
	if err != nil || stored.Status != model.AgentExecutionStatusDispatching || w.runnerA.createCalls != 0 {
		t.Fatalf("terminal unknown intent must not be replayed or settled: execution=%+v creates=%d err=%v", stored, w.runnerA.createCalls, err)
	}
}
