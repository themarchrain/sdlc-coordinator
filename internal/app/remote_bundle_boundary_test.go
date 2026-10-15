package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sdlc-coordinator/internal/adapters/agentrunner"
	"sdlc-coordinator/internal/agent"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/workers"
	"sdlc-coordinator/internal/workspaceproto"

	orchestrator "sdlc-coordinator/internal/orchestrator"
)

// bundleReadingProvider 是从 Runner 本机工作区读任务包的假 Provider：三段
// 内容都可读才返回结构化结果。协调器本地写入在 Runner 文件系统上不可见，
// 任务包没有跨过边界时本 Provider 直接失败；记录读到的内容与工作区路径。
type bundleReadingProvider struct {
	files map[string]string
	path  string
}

func (p *bundleReadingProvider) Execute(_ context.Context, request runner.ProviderRequest, _ runner.EmitFunc) (json.RawMessage, error) {
	p.path = request.WorkspacePath
	p.files = map[string]string{}
	for _, name := range []string{agent.PromptFileName, agent.ResultSchemaFileName, agent.TaskContextFileName} {
		raw, err := os.ReadFile(filepath.Join(request.WorkspacePath, name))
		if err != nil {
			return nil, fmt.Errorf("read task bundle file %s from runner workspace: %w", name, err)
		}
		p.files[name] = string(raw)
	}
	return json.RawMessage(fmt.Sprintf(
		`{"task_id":%q,"run_id":%q,"outcome":"insufficient_context","summary":"bundle boundary probe","blocker_reason":"probe"}`,
		request.TaskID, request.ExecutionID)), nil
}

// probeEndpointWorld 是一个假 Runner 端点：真实 WorkspaceOps + 计数包装。
type probeEndpointWorld struct {
	server       *httptest.Server
	provider     *bundleReadingProvider
	prepareCalls atomic.Int32
	bundleCalls  atomic.Int32
	createCalls  atomic.Int32
}

const probeToken = "bundle-boundary-token"

func newProbeRunnerEndpoint(t *testing.T) (*probeEndpointWorld, string) {
	t.Helper()
	dataRoot := t.TempDir()
	provider := &bundleReadingProvider{}
	service, err := runner.NewService(filepath.Join(t.TempDir(), "executions"), map[string]runner.Provider{"probe-cli": provider})
	if err != nil {
		t.Fatalf("runner service: %v", err)
	}
	ops, err := runner.NewWorkspaceOps(runner.WorkspaceOpsConfig{
		DataRoot: dataRoot,
		Authorize: func(context.Context, workspaceproto.AuthorizationRequest) (workspaceproto.AuthorizationDecision, error) {
			return workspaceproto.AuthorizationDecision{Authorized: true}, nil
		},
	})
	if err != nil {
		t.Fatalf("workspace ops: %v", err)
	}
	world := &probeEndpointWorld{provider: provider}
	handler := runner.NewHTTPHandler(service, runner.WithWorkspaceOperations(ops, probeToken))
	world.server = httptest.NewServer(countingEndpointHandler{inner: handler, world: world})
	return world, dataRoot
}

// countingEndpointHandler 统计端点上收到的准备/投递/执行创建调用数。
type countingEndpointHandler struct {
	inner http.Handler
	world *probeEndpointWorld
}

func (h countingEndpointHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/internal/workspace/prepare"):
		h.world.prepareCalls.Add(1)
	case strings.HasSuffix(r.URL.Path, "/internal/workspace/bundle"):
		h.world.bundleCalls.Add(1)
	case strings.HasSuffix(r.URL.Path, "/api/v1/executions"):
		h.world.createCalls.Add(1)
	}
	h.inner.ServeHTTP(w, r)
}

// staticPoolRouter 把任务池固定指派到一个端点（刻意非默认端点）。
type staticPoolRouter struct {
	poolClient orchestrator.ReviewerRunnerClient
	endpoint   string
}

func (r staticPoolRouter) AttemptForTask(context.Context, *model.Task) (orchestrator.RunnerAttempt, error) {
	return orchestrator.NewRunnerAttempt(r.poolClient, r.endpoint, nil), nil
}

func (r staticPoolRouter) ClientForEndpoint(endpoint string) (orchestrator.ReviewerRunnerClient, bool) {
	if strings.TrimSpace(endpoint) == r.endpoint {
		return r.poolClient, true
	}
	return nil, false
}

// waitForSucceededExecution 轮询等待假 Provider 执行完成（Provider 立即返回，
// 轮询只是消费异步执行状态）。
func waitForSucceededExecution(t *testing.T, client *agentrunner.Client, runID string) {
	t.Helper()
	for i := 0; i < 200; i++ {
		execution, err := client.Get(context.Background(), runID)
		if err == nil && execution != nil && (execution.Status == runner.StatusSucceeded || execution.Status == runner.StatusFailed) {
			if execution.Status == runner.StatusFailed {
				t.Fatalf("probe execution failed: %s %s", execution.ErrorCode, execution.ErrorMessage)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the probe execution to finish")
}

// TestDeveloperDispatchDeliversTaskBundleAcrossFilesystemBoundary 锁定 P0 与
// 端点归属（审查 #1/#2）：协调器与 Runner 各自独占文件系统（无共享卷、路径
// 不重叠），且任务被路由到非默认端点时，开发者派发全链必须走通——任务包经
// 受控通道落到 run 所属 Runner 本机工作区、执行请求路径用 Runner 返回值、
// Provider 读到三段内容且与协调器渲染产物逐字节一致；默认端点收到零次操作
// （准备/投递/执行只命中钉住的归属端点）。
func TestDeveloperDispatchDeliversTaskBundleAcrossFilesystemBoundary(t *testing.T) {
	baseURL, pathWithNamespace, originDir, seedSHA := newRemoteDeliveryOrigin(t)
	ctx := context.Background()

	defaultEndpoint, _ := newProbeRunnerEndpoint(t)
	defer defaultEndpoint.server.Close()
	targetEndpoint, targetRoot := newProbeRunnerEndpoint(t)
	defer targetEndpoint.server.Close()

	db, err := store.Open("file:bundle_boundary_" + model.NewID("t") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db,
		&model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.AgentExecutionEvent{},
		&model.DeveloperDeliveryCheckpoint{}, &model.Effect{}, &model.Artifact{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	taskRepo := repository.NewTaskRepository(db)
	runRepo := repository.NewRunRepository(db)
	execRepo := repository.NewAgentExecutionRepository(db)
	checkpoints := repository.NewDeveloperDeliveryCheckpointRepository(db)

	task := &model.Task{
		TaskID: "task-bundle-boundary", Type: "develop_from_issue", Role: "DeveloperWorker",
		DedupeKey:  "bundle-boundary:1",
		TargetRepo: "group/project", SourceRepo: "group/project",
		DeliveryBranch: "agent/task-bundle-boundary", Status: model.TaskStatusPending,
		ContextJSON: `{"project":{"path":"group/project"},"repos":[{"name":"project","url":"` + fileURLOf(t, originDir) + `","branch":"main","access":"writable"}]}`,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	repoRow := &registeredrepo.RegisteredRepository{
		RepositoryID: "repo-bundle", Provider: "github", BaseURL: baseURL,
		PathWithNamespace: pathWithNamespace, Status: registeredrepo.StatusEnabled,
		GitAuthorName: "AI Developer", GitAuthorEmail: "ai-developer@localhost",
	}
	// 协调器工作区根是另一个目录树（模拟 coordinator 容器内路径）。
	coordinatorWorkspaceRoot := t.TempDir()
	// 按 run 归属端点路由：读 run 行钉住的端点（首次派发时为空 → 默认端点）。
	remote := agentrunner.NewRoutedRemoteWorkspace(defaultEndpoint.server.URL, probeToken, 1, nil, func(ctx context.Context, runID string) (string, error) {
		record, err := runRepo.GetByRunID(ctx, runID)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(record.RunnerEndpoint), nil
	})
	preparer := agentrunner.NewRemoteDeveloperPreparer(remote)
	defaultClient := agentrunner.NewClient(defaultEndpoint.server.URL, nil)
	poolClient := agentrunner.NewClient(targetEndpoint.server.URL, nil)
	settings := orchestrator.ReviewerRunnerSettings{
		WorkspaceRoot: coordinatorWorkspaceRoot,
		Provider:      runner.ProviderConfig{Name: "probe-cli", Revision: 1},
		ResolveRepository: func(context.Context, *model.Task) (*registeredrepo.RegisteredRepository, error) {
			return repoRow, nil
		},
	}
	coordinator := orchestrator.NewDeveloperRunnerCoordinator(
		taskRepo, runRepo, execRepo, preparer, remote, defaultClient, &recordingSubmitter{}, settings,
	)
	coordinator.SetDeveloperDeliveryCheckpoints(checkpoints)
	coordinator.SetTaskBundlePublisher(remote)
	// 刻意的池指派：任务的池 = 非默认端点。
	coordinator.SetTenantRunnerRouter(staticPoolRouter{poolClient: poolClient, endpoint: targetEndpoint.server.URL})

	orch := orchestrator.New(taskRepo, runRepo,
		orchestrator.WithDeveloperRunner(coordinator, settings),
		orchestrator.WithRunnerArtifactStore(poolClient),
	)
	result, err := orch.DispatchDeveloperTask(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("dispatch developer task: %v", err)
	}
	if result == nil || result.Run == nil {
		t.Fatal("dispatch returned no run")
	}
	runID := result.Run.RunID
	waitForSucceededExecution(t, poolClient, runID)

	// 任务包实际到达归属端点（非默认）的 run 工作区：Provider 读到全部三段。
	if len(targetEndpoint.provider.files) != 3 {
		t.Fatalf("provider did not read the task bundle from the runner workspace: %v", targetEndpoint.provider.files)
	}
	runnerRoot := filepath.Join(targetRoot, "workspaces", "runs", runID)
	if targetEndpoint.provider.path != runnerRoot {
		t.Fatalf("provider workspace path = %q, want runner-side %q", targetEndpoint.provider.path, runnerRoot)
	}
	if strings.HasPrefix(targetEndpoint.provider.path, coordinatorWorkspaceRoot) {
		t.Fatalf("provider read a coordinator-side path %q: the bundle never crossed the filesystem boundary", targetEndpoint.provider.path)
	}
	// 内容与协调器渲染产物逐字节一致（共享渲染器）。
	reloadedTask, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	reloadedRun, err := runRepo.GetByRunID(ctx, runID)
	if err != nil {
		t.Fatalf("reload run: %v", err)
	}
	bundle, err := workers.BuildDeveloperRunnerTaskBundle(reloadedTask, reloadedRun)
	if err != nil {
		t.Fatalf("build expected bundle: %v", err)
	}
	rendered, err := agent.RenderTaskBundle(bundle)
	if err != nil {
		t.Fatalf("render expected bundle: %v", err)
	}
	for name, want := range rendered {
		if got := targetEndpoint.provider.files[name]; got != string(want) {
			t.Fatalf("delivered %s differs from coordinator rendering:\n got %q\nwant %q", name, got, string(want))
		}
	}
	// 端点归属：默认端点收到零次准备/投递/执行创建。
	if defaultEndpoint.prepareCalls.Load() != 0 {
		t.Fatalf("default endpoint received %d prepare calls, want 0 (workspace work must follow the run endpoint)", defaultEndpoint.prepareCalls.Load())
	}
	if defaultEndpoint.bundleCalls.Load() != 0 {
		t.Fatalf("default endpoint received %d bundle calls, want 0", defaultEndpoint.bundleCalls.Load())
	}
	if defaultEndpoint.createCalls.Load() != 0 {
		t.Fatalf("default endpoint received %d execution creates, want 0", defaultEndpoint.createCalls.Load())
	}
	if targetEndpoint.prepareCalls.Load() == 0 || targetEndpoint.bundleCalls.Load() == 0 || targetEndpoint.createCalls.Load() == 0 {
		t.Fatalf("target endpoint missing operations: prepare=%d bundle=%d create=%d",
			targetEndpoint.prepareCalls.Load(), targetEndpoint.bundleCalls.Load(), targetEndpoint.createCalls.Load())
	}
	// 端点与 Runner 侧路径都钉进了 run 行（恢复路径按同一目标重放）。
	if strings.TrimSpace(reloadedRun.RunnerEndpoint) != targetEndpoint.server.URL {
		t.Fatalf("run runner endpoint = %q, want %q", reloadedRun.RunnerEndpoint, targetEndpoint.server.URL)
	}
	if strings.TrimSpace(reloadedRun.RunnerWorkspacePath) != runnerRoot {
		t.Fatalf("run runner workspace path = %q, want %q", reloadedRun.RunnerWorkspacePath, runnerRoot)
	}
	// 基线检查点钉住种子 SHA（准备真的发生在归属端点、仓库已检出）。
	checkpoint, err := checkpoints.Load(ctx, runID)
	if err != nil || checkpoint == nil || checkpoint.BaselineSHA != seedSHA {
		t.Fatalf("baseline checkpoint = %+v err=%v, want baseline %s", checkpoint, err, seedSHA)
	}
}

// fileURLOf 把本地目录转为 file:/// 形态 URL（与 newRemoteDeliveryOrigin 同规）。
func fileURLOf(t *testing.T, dir string) string {
	t.Helper()
	return "file:///" + strings.TrimPrefix(filepath.ToSlash(dir), "/")
}
