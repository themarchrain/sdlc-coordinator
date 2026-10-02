package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/resulttoken"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/workspace"

	"github.com/gin-gonic/gin"
)

const mcpTestEndpointTask = "task-mcp-handler"

type mcpFixture struct {
	handler     *MCPHandler
	router      *gin.Engine
	secret      []byte
	rejections  *repository.ResultRejectionRepository
	submissions *repository.ResultSubmissionRepository
	tasks       *repository.TaskRepository
	runs        *repository.RunRepository
	now         func() time.Time
}

func newMCPFixture(t *testing.T) mcpFixture {
	return newMCPFixtureWithCodeHost(t, false)
}

func newMCPFixtureWithCodeHost(t *testing.T, withCodeHost bool) mcpFixture {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Artifact{}, &model.Writeback{}, &model.ResultSubmission{}, &model.ResultSubmissionRejection{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	tasks := repository.NewTaskRepository(db)
	runs := repository.NewRunRepository(db)
	submissions := repository.NewResultSubmissionRepository(db)
	rejections := repository.NewResultRejectionRepository(db)
	options := []orchestrator.Option{
		orchestrator.WithArtifactRepository(repository.NewArtifactRepository(db)),
		orchestrator.WithWritebackRepository(repository.NewWritebackRepository(db)),
		orchestrator.WithResultSubmissionRepository(submissions),
	}
	if withCodeHost {
		options = append(options, orchestrator.WithCodeHostBackendResolver(mcpCodeHostResolver{}))
	}
	orch := orchestrator.New(tasks, runs, options...)
	submitter := orchestrator.ReviewerResultSubmitterFunc(func(ctx context.Context, request orchestrator.SubmitResultRequest) (*orchestrator.SubmitResultResponse, error) {
		return orch.SubmitResult(ctx, request)
	})
	finalizer := &stubMCPFinalizer{}
	channel := orchestrator.NewResultChannelCoordinator(finalizer, submitter, orchestrator.ReviewerRunnerSettings{WorkspaceRoot: "D:/tmp/ws", ResolveRepository: stubRunnerRepositoryForMCP()}, "mcp")
	secret, err := resulttoken.NewSecret()
	if err != nil {
		t.Fatalf("new secret: %v", err)
	}
	handler := NewMCPHandler(channel, secret, tasks, runs, rejections, 5)
	router := gin.New()
	router.POST("/mcp", handler.Handle)
	router.DELETE("/mcp", handler.CloseSession)
	router.GET("/mcp", handler.Handle)
	return mcpFixture{handler: handler, router: router, secret: secret, rejections: rejections, submissions: submissions, tasks: tasks, runs: runs, now: time.Now}
}

type stubMCPFinalizer struct{ calls int }

type mcpCodeHostResolver struct{}

func (mcpCodeHostResolver) Resolve(context.Context, *model.Task, *model.Writeback) (codehost.Backend, error) {
	return mcpCodeHostBackend{}, nil
}

type mcpCodeHostBackend struct{}

func (mcpCodeHostBackend) CreateIssue(context.Context, string, codehost.CreateIssueRequest) (*codehost.Issue, error) {
	return &codehost.Issue{IID: 1}, nil
}

func (mcpCodeHostBackend) CreateMergeRequest(context.Context, string, codehost.CreateMergeRequestRequest) (*codehost.MergeRequest, error) {
	return &codehost.MergeRequest{IID: 1}, nil
}

func (mcpCodeHostBackend) CreateIssueNote(context.Context, string, int, codehost.CreateNoteRequest) (*codehost.Note, error) {
	return &codehost.Note{ID: 1}, nil
}

func (mcpCodeHostBackend) CreateMergeRequestNote(_ context.Context, _ string, _ int, request codehost.CreateNoteRequest) (*codehost.Note, error) {
	return &codehost.Note{ID: 1, Body: request.Body}, nil
}

func (mcpCodeHostBackend) SetIssueLabels(context.Context, string, int, codehost.SetLabelsRequest) (*codehost.Issue, error) {
	return &codehost.Issue{IID: 1}, nil
}

func (mcpCodeHostBackend) SetMergeRequestLabels(context.Context, string, int, codehost.SetLabelsRequest) (*codehost.MergeRequest, error) {
	return &codehost.MergeRequest{IID: 1}, nil
}

func (f *stubMCPFinalizer) Finalize(_ context.Context, _ workspace.DeveloperFinalizeRequest) (*workspace.DeveloperDelivery, error) {
	f.calls++
	return &workspace.DeveloperDelivery{CommitSHA: "commit-mcp", ChangedFiles: []string{"main.go"}, DiffSummary: "1 file changed"}, nil
}

// stubRunnerRepositoryForMCP 返回固定注册仓行的解析闭包,派生的 CloneURL 与
// fixture 任务包中的事件 URL 一致,凭据语义由 registeredrepo 行承载。
func stubRunnerRepositoryForMCP() func(context.Context, *model.Task) (*registeredrepo.RegisteredRepository, error) {
	return func(context.Context, *model.Task) (*registeredrepo.RegisteredRepository, error) {
		return &registeredrepo.RegisteredRepository{
			RepositoryID: "repo-mcp-fixture", Provider: "gitlab",
			BaseURL: "https://gitlab.example", PathWithNamespace: "group/project", Token: "git-token",
		}, nil
	}
}

// createDeveloperRun inserts a running developer task/run pair as dispatch would.
func (f mcpFixture) createDeveloperRun(t *testing.T, runID string) {
	t.Helper()
	current := runID
	task := &model.Task{
		TaskID: mcpTestEndpointTask, DedupeKey: "mcp-" + runID, Type: "develop_from_issue", Role: "DeveloperWorker",
		TargetRepo: "group/project", SourceRepo: "group/project", Objective: "fix", AcceptanceCriteria: "tests",
		Status: model.TaskStatusRunning, CurrentRunID: &current, DeliveryBranch: "agent/" + mcpTestEndpointTask,
		ContextJSON: `{"project":{"path":"group/project"},"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"master","access":"writable"}]}`,
	}
	if err := f.tasks.Create(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := f.runs.Create(context.Background(), &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}); err != nil {
		t.Fatalf("create run: %v", err)
	}
}

func (f mcpFixture) token(taskID, runID, role string, issuedAt time.Time) string {
	token, err := resulttoken.Issue(f.secret, resulttoken.Claims{TaskID: taskID, RunID: runID, Role: role, TTL: time.Hour}, issuedAt)
	if err != nil {
		panic(err)
	}
	return token
}

func (f mcpFixture) postRPC(t *testing.T, token string, method string, params any) (int, map[string]any) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response %d: %v: %s", recorder.Code, err, recorder.Body.String())
	}
	return recorder.Code, response
}

func (f mcpFixture) callTool(t *testing.T, token, toolName, payload string) (int, map[string]any, map[string]any) {
	t.Helper()
	code, response := f.postRPC(t, token, "tools/call", map[string]any{
		"name": toolName, "arguments": json.RawMessage(payload),
	})
	result, _ := response["result"].(map[string]any)
	return code, result, mcpToolText(t, result)
}

func mcpToolText(t *testing.T, result map[string]any) map[string]any {
	t.Helper()
	if result == nil {
		t.Fatal("tool response carries no result")
	}
	isError, _ := result["isError"].(bool)
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("tool result carries no content: %#v", result)
	}
	block, _ := content[0].(map[string]any)
	text, _ := block["text"].(string)
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("tool result text is not JSON (isError=%v): %s", isError, text)
	}
	return payload
}

func toolStatus(payload map[string]any) string {
	status, _ := payload["status"].(string)
	return status
}

func TestMCPInitializeEchoesProtocolAndIssuesSession(t *testing.T) {
	f := newMCPFixture(t)
	f.createDeveloperRun(t, "run-mcp-init")
	token := f.token(mcpTestEndpointTask, "run-mcp-init", "DeveloperWorker", f.now())

	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 7, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18"}})
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(string(body)))
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("initialize failed: %d %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Mcp-Session-Id") == "" {
		t.Fatal("initialize must issue a session id header")
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode initialize response: %v", err)
	}
	result, _ := response["result"].(map[string]any)
	if result == nil || result["protocolVersion"] != "2025-06-18" {
		t.Fatalf("initialize must echo requested protocol version: %#v", result)
	}
}

func TestMCPToolsListIsRoleFiltered(t *testing.T) {
	f := newMCPFixture(t)
	f.createDeveloperRun(t, "run-mcp-list")
	token := f.token(mcpTestEndpointTask, "run-mcp-list", "DeveloperWorker", f.now())

	_, response := f.postRPC(t, token, "tools/list", map[string]any{})
	result, _ := response["result"].(map[string]any)
	tools, _ := result["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("developer token must see exactly its own tool: %#v", tools)
	}
	tool, _ := tools[0].(map[string]any)
	if tool["name"] != results.SubmitDeveloperResultTool {
		t.Fatalf("unexpected tool: %#v", tool)
	}
	if tool["inputSchema"] == nil {
		t.Fatal("tool schema must be published")
	}
}

func TestMCPTesterToolListAndSubmissionPersistBusinessResult(t *testing.T) {
	f := newMCPFixtureWithCodeHost(t, true)
	const taskID, runID = "task-mcp-tester", "run-mcp-tester"
	current := runID
	repositoryID := "repo-mcp-fixture"
	task := &model.Task{
		TaskID: taskID, DedupeKey: "mcp-tester-" + runID, Type: "verify_merge_request", Role: "TesterWorker",
		TargetRepo: "group/project", SourceRepo: "group/project", RepositoryID: &repositoryID, Objective: "verify MR", AcceptanceCriteria: "report test evidence",
		Status: model.TaskStatusRunning, CurrentRunID: &current,
		ContextJSON: `{"project":{"path":"group/project"},"merge_request":{"iid":21,"url":"https://gitlab.example/group/project/-/merge_requests/21","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"master","access":"readable"}]}`,
	}
	run := &model.Run{RunID: runID, TaskID: taskID, WorkerRole: "TesterWorker", Status: model.RunStatusRunning,
		InputJSON: `{"verification":{"floor_command":"go test ./..."}}`}
	if err := f.tasks.Create(context.Background(), task); err != nil {
		t.Fatalf("create TesterWorker task: %v", err)
	}
	if err := f.runs.Create(context.Background(), run); err != nil {
		t.Fatalf("create TesterWorker run: %v", err)
	}
	token := f.token(taskID, runID, "TesterWorker", f.now())

	_, response := f.postRPC(t, token, "tools/list", map[string]any{})
	result, _ := response["result"].(map[string]any)
	tools, _ := result["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("TesterWorker token must see exactly its own tool: %#v", tools)
	}
	tool, _ := tools[0].(map[string]any)
	if tool["name"] != results.SubmitTesterResultTool {
		t.Fatalf("unexpected TesterWorker tool: %#v", tool)
	}

	code, toolResult, payload := f.callTool(t, token, results.SubmitTesterResultTool,
		`{"task_id":"task-mcp-tester","run_id":"run-mcp-tester","outcome":"verification_passed","summary":"tests passed","tests":[{"command":"go test ./...","passed":true,"summary":"ok"}]}`)
	if code != http.StatusOK || toolResult["isError"].(bool) || toolStatus(payload) != "accepted" {
		t.Fatalf("TesterWorker submission must be accepted: code=%d result=%#v payload=%#v", code, toolResult, payload)
	}
	storedTask, err := f.tasks.GetByTaskID(context.Background(), taskID)
	if err != nil {
		t.Fatal(err)
	}
	storedRun, err := f.runs.GetByRunID(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if storedTask.Status != model.TaskStatusSucceeded || storedRun.Status != model.RunStatusSucceeded {
		t.Fatalf("accepted TesterWorker result must finalize task and run: task=%s run=%s", storedTask.Status, storedRun.Status)
	}
	submission, err := f.submissions.GetByRunID(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if submission.Role != "TesterWorker" || submission.Status != model.ResultSubmissionStatusSucceeded {
		t.Fatalf("accepted TesterWorker submission must be persisted: %#v", submission)
	}
}

func TestMCPCorrectionLoopConvergesWithAttemptAccounting(t *testing.T) {
	f := newMCPFixture(t)
	f.createDeveloperRun(t, "run-mcp-correct")
	token := f.token(mcpTestEndpointTask, "run-mcp-correct", "DeveloperWorker", f.now())

	// 第 1 次：change_ready 缺 tests —— 结构化拒绝、带路径、剩余次数递减。
	code, result, payload := f.callTool(t, token, results.SubmitDeveloperResultTool,
		`{"task_id":"`+mcpTestEndpointTask+`","run_id":"run-mcp-correct","outcome":"change_ready","summary":"done","merge_request":{"title":"T","description":"D"}}`)
	if code != http.StatusOK || !result["isError"].(bool) {
		t.Fatalf("first attempt must be an error tool result: %d %#v", code, result)
	}
	if toolStatus(payload) != "rejected" {
		t.Fatalf("unexpected status: %#v", payload)
	}
	if payload["attempts_remaining"].(float64) != 4 {
		t.Fatalf("attempts after first rejection: %#v", payload)
	}
	problems, _ := payload["problems"].([]any)
	if len(problems) == 0 {
		t.Fatal("rejection must carry problems")
	}
	firstProblem, _ := problems[0].(map[string]any)
	if !strings.Contains(firstProblem["problem"].(string), "tests") {
		t.Fatalf("problem must surface contract error verbatim: %#v", firstProblem)
	}

	// 第 2 次：按错误修正（换 fix_failed + 规范化别名键）—— 接受并终态化。
	code, result, payload = f.callTool(t, token, results.SubmitDeveloperResultTool,
		`{"taskId":"`+mcpTestEndpointTask+`","runId":"run-mcp-correct","outcome":"fix_failed","summary":"it broke","blockerReason":"tests fail"}`)
	if code != http.StatusOK || result["isError"].(bool) {
		t.Fatalf("corrected attempt must be accepted: %d %#v", code, result)
	}
	if toolStatus(payload) != "accepted" {
		t.Fatalf("unexpected accept payload: %#v", payload)
	}
	runRecord, _ := f.runs.GetByRunID(context.Background(), "run-mcp-correct")
	if runRecord.Status != model.RunStatusFailed {
		t.Fatalf("run must be terminal after acceptance, got %s", runRecord.Status)
	}
	if count, _ := f.rejections.CountByRun(context.Background(), "run-mcp-correct"); count != 1 {
		t.Fatalf("exactly one rejection must be recorded, got %d", count)
	}
}

func TestMCPExhaustionHandsOffToTextChannel(t *testing.T) {
	f := newMCPFixture(t)
	f.createDeveloperRun(t, "run-mcp-exhaust")
	token := f.token(mcpTestEndpointTask, "run-mcp-exhaust", "DeveloperWorker", f.now())
	badPayload := `{"task_id":"` + mcpTestEndpointTask + `","run_id":"run-mcp-exhaust","outcome":"change_ready","summary":"s","merge_request":{"title":"T","description":"D"}}`

	var last map[string]any
	for attempt := 1; attempt <= 5; attempt++ {
		_, result, payload := f.callTool(t, token, results.SubmitDeveloperResultTool, badPayload)
		if !result["isError"].(bool) {
			t.Fatalf("attempt %d must be rejected", attempt)
		}
		last = payload
	}
	if toolStatus(last) != "rejected_final" {
		t.Fatalf("fifth rejection must hand off, got %#v", last)
	}
	if next, _ := last["next_step"].(string); !strings.Contains(next, "最终消息") {
		t.Fatalf("handoff must instruct the text channel: %#v", last)
	}
	// 耗尽后的重试不再累计，也不再重新校验——原样交接。
	_, result, payload := f.callTool(t, token, results.SubmitDeveloperResultTool, badPayload)
	if !result["isError"].(bool) || toolStatus(payload) != "rejected_final" {
		t.Fatalf("post-exhaustion call must return the same handoff: %#v", payload)
	}
	if count, _ := f.rejections.CountByRun(context.Background(), "run-mcp-exhaust"); count != 5 {
		t.Fatalf("rejection rows must stay capped at 5, got %d", count)
	}
}

func TestMCPAlreadyFinalRunReturnsTerminalNotice(t *testing.T) {
	f := newMCPFixture(t)
	f.createDeveloperRun(t, "run-mcp-final")
	token := f.token(mcpTestEndpointTask, "run-mcp-final", "DeveloperWorker", f.now())
	accepted := `{"taskId":"` + mcpTestEndpointTask + `","runId":"run-mcp-final","outcome":"fix_failed","summary":"broke","blockerReason":"x"}`
	_, result, payload := f.callTool(t, token, results.SubmitDeveloperResultTool, accepted)
	if result["isError"].(bool) || toolStatus(payload) != "accepted" {
		t.Fatalf("first submission must be accepted: %#v", payload)
	}
	// 首提即终态：第二把 tools/call（同哈希或异哈希）收到已终态，而非第二个终态。
	_, result, payload = f.callTool(t, token, results.SubmitDeveloperResultTool, accepted)
	if !result["isError"].(bool) || toolStatus(payload) != "already_final" {
		t.Fatalf("second call must be already_final: %#v", payload)
	}
	_, result, payload = f.callTool(t, token, results.SubmitDeveloperResultTool, `{"task_id":"`+mcpTestEndpointTask+`","run_id":"run-mcp-final","outcome":"fix_failed","summary":"different","blocker_reason":"y"}`)
	if !result["isError"].(bool) || toolStatus(payload) != "already_final" {
		t.Fatalf("different payload on terminal run must be already_final: %#v", payload)
	}
}

func TestMCPRejectsBoundaryViolationsWithoutCounting(t *testing.T) {
	f := newMCPFixture(t)
	f.createDeveloperRun(t, "run-mcp-bound")
	goodToken := f.token(mcpTestEndpointTask, "run-mcp-bound", "DeveloperWorker", f.now())

	// 无 token。
	code, _ := f.postRPC(t, "", "tools/list", map[string]any{})
	if code != http.StatusUnauthorized {
		t.Fatalf("missing token must be 401, got %d", code)
	}
	// 坏签名。
	code, _ = f.postRPC(t, goodToken+"x", "tools/list", map[string]any{})
	if code != http.StatusUnauthorized {
		t.Fatalf("tampered token must be 401, got %d", code)
	}
	// 过期。
	expired := f.token(mcpTestEndpointTask, "run-mcp-bound", "DeveloperWorker", f.now().Add(-2*time.Hour))
	code, _ = f.postRPC(t, expired, "tools/list", map[string]any{})
	if code != http.StatusUnauthorized {
		t.Fatalf("expired token must be 401, got %d", code)
	}
	// token 指向不存在的 run（tools/call 才触及 run 存在性）→ 直接 403。
	forbiddenCode := func(token, tool string) int {
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
			"params": map[string]any{"name": tool, "arguments": json.RawMessage("{}")}})
		request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(string(body)))
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		f.router.ServeHTTP(recorder, request)
		return recorder.Code
	}
	if code := forbiddenCode(f.token(mcpTestEndpointTask, "run-unknown", "DeveloperWorker", f.now()), results.SubmitDeveloperResultTool); code != http.StatusForbidden {
		t.Fatalf("unknown run must be 403, got %d", code)
	}
	// 角色不匹配的工具（developer token 调 reviewer 工具）。
	if code := forbiddenCode(goodToken, results.SubmitReviewerResultTool); code != http.StatusForbidden {
		t.Fatalf("role-mismatched tool must be 403, got %d", code)
	}
	if count, _ := f.rejections.CountByRun(context.Background(), "run-mcp-bound"); count != 0 {
		t.Fatalf("boundary violations must not count as corrections, got %d", count)
	}
}

func TestMCPSessionAndUnknownMethods(t *testing.T) {
	f := newMCPFixture(t)
	request := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("DELETE must close cleanly, got %d", recorder.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/mcp", nil)
	recorder = httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET must be 405, got %d", recorder.Code)
	}
	// 未知方法在 JSON-RPC 层内报错（鉴权先行：无 token 一律 401）。
	token := f.token("task-any", "run-any", "DeveloperWorker", f.now())
	code, response := f.postRPC(t, token, "bogus/method", map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("unknown method responds inside JSON-RPC: %d", code)
	}
	rpcErr, _ := response["error"].(map[string]any)
	if rpcErr == nil || rpcErr["code"].(float64) != -32601 {
		t.Fatalf("unknown method must be -32601: %#v", response)
	}
}

type stubMCPEngineSource struct {
	view MCPEngineView
}

func (s stubMCPEngineSource) MCPEngineForTenant(uint) MCPEngineView { return s.view }

// 引擎选择器注入时零值视图一律 fail-closed（503）——含无租户 token
// （其寿命早已耗尽，不存在存活的平台兜底消费方）。
func TestMCPEngineViewFailsClosedWhenTenantEngineUnavailable(t *testing.T) {
	handler := &MCPHandler{engines: stubMCPEngineSource{}}

	if _, err := handler.engineView(resulttoken.Claims{TenantID: 7}); !errors.Is(err, errTenantEngineUnavailable) {
		t.Fatalf("tenant engine miss must fail closed, got %v", err)
	}
	if _, err := handler.engineView(resulttoken.Claims{TenantID: 0}); !errors.Is(err, errTenantEngineUnavailable) {
		t.Fatalf("tenant-zero token must fail closed too, got %v", err)
	}
}

// TestMCPHandleGateCoversEngineOnlyAssembly 以 Handle 级请求锁定门槛语义：
// 默认通道缺席、仅注入引擎选择器（生产装配形态）判已配置并正常服务；
// 选择器返回零值视图时 tools/call 503；双缺席仍是未配置 500。
func TestMCPHandleGateCoversEngineOnlyAssembly(t *testing.T) {
	f := newMCPFixture(t)
	f.createDeveloperRun(t, "run-mcp-gate")
	token := f.token(mcpTestEndpointTask, "run-mcp-gate", "DeveloperWorker", f.now())

	serving := NewMCPHandler(nil, f.secret, nil, nil, nil, 5,
		WithMCPEngineSource(stubMCPEngineSource{view: MCPEngineView{
			Channel: f.handler.channel, Tasks: f.tasks, Runs: f.runs, Rejections: f.rejections,
		}}))
	f.handler = serving
	f.router = gin.New()
	f.router.POST("/mcp", serving.Handle)
	code, response := f.postRPC(t, token, "tools/list", map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("engine-only assembly must pass the configured gate: %d %v", code, response)
	}
	result, _ := response["result"].(map[string]any)
	tools, _ := result["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools/list must serve normally in engine-only assembly: %#v", response)
	}

	f.handler = NewMCPHandler(nil, f.secret, nil, nil, nil, 5, WithMCPEngineSource(stubMCPEngineSource{}))
	f.router = gin.New()
	f.router.POST("/mcp", f.handler.Handle)
	code, response = f.postRPC(t, token, "tools/call", map[string]any{"name": "submit", "arguments": json.RawMessage("{}")})
	if code != http.StatusServiceUnavailable {
		t.Fatalf("zero view must fail closed with 503: %d %v", code, response)
	}
	if errObj, _ := response["error"].(map[string]any); errObj == nil || errObj["message"] != "tenant engine unavailable" {
		t.Fatalf("503 must name the tenant engine: %#v", response)
	}

	unconfigured := NewMCPHandler(nil, f.secret, nil, nil, nil, 5)
	f.handler = unconfigured
	f.router = gin.New()
	f.router.POST("/mcp", unconfigured.Handle)
	code, response = f.postRPC(t, token, "tools/list", map[string]any{})
	if code != http.StatusInternalServerError {
		t.Fatalf("no channel and no engines must stay unconfigured: %d %v", code, response)
	}
}
