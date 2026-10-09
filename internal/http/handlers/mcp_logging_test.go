package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/resulttoken"
)

// MCP 内部错误日志形态:observer 锁死 Error 级别 + request_id/error 字段,
// 同时断言对外 JSON-RPC 响应与迁移前完全一致(-32603 + 通用文案)。

// errRunRepository 是 GetByRunID 恒报非 NotFound 错误的仓储桩:触发
// internalError 的基础设施故障路径(签名有效、run 查询本身炸了)。
type errRunRepository struct {
	orchestrator.RunRepository
	err error
}

func (r *errRunRepository) GetByRunID(context.Context, string) (*model.Run, error) {
	return nil, r.err
}

func TestMCPInternalErrorLogsErrorWithRequestIDAndErrorFields(t *testing.T) {
	f := newMCPFixture(t)
	core, logs := observer.New(zapcore.DebugLevel)
	f.handler.logger = zap.New(core)
	// 引擎视图的 Runs 换成报错桩:走 view.Runs.GetByRunID 的非 NotFound 分支。
	// Channel 沿用 fixture 真值,否则 engineView 先判零值视图 503,到不了目标分支。
	f.handler.engines = stubMCPEngineSource{view: MCPEngineView{
		Channel: f.handler.channel,
		Runs: &errRunRepository{
			RunRepository: f.runs,
			err:           errors.New("driver: disk I/O error"),
		},
	}}

	runID := "run-mcp-log"
	if err := f.runs.Create(context.Background(), &model.Run{RunID: runID, TaskID: mcpTestEndpointTask, WorkerRole: "DeveloperWorker", Status: model.RunStatusRunning}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	token := f.token(mcpTestEndpointTask, runID, "DeveloperWorker", f.now())
	code, response := f.postRPC(t, token, "tools/call", map[string]any{
		"name": "submit_developer_result", "arguments": json.RawMessage(`{}`),
	})

	// 对外响应与迁移前完全一致:HTTP 200 + JSON-RPC -32603 + 通用文案,
	// 内部错误细节不回显。
	if code != http.StatusOK {
		t.Fatalf("internal error must stay HTTP 200 inside JSON-RPC, got %d", code)
	}
	rpcErr, ok := response["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %#v", response)
	}
	if rpcErr["code"].(float64) != -32603 {
		t.Fatalf("expected -32603, got %v", rpcErr["code"])
	}
	if rpcErr["message"] != "Internal error" {
		t.Fatalf("expected generic message, got %v", rpcErr["message"])
	}
	if data, _ := rpcErr["data"].(string); !strings.Contains(data, "temporarily unavailable") {
		t.Fatalf("expected generic data, got %v", rpcErr["data"])
	}

	// 日志形态:恰好一条 Error,字段 request_id/error 齐全,值保真。
	entries := logs.FilterLevelExact(zapcore.ErrorLevel).FilterMessage("mcp: internal error").All()
	if len(entries) != 1 {
		t.Fatalf("expect exactly one error entry, got %d", len(entries))
	}
	fields := entries[0].ContextMap()
	// request_id 按原样保真:postRPC 编码的 id 是 JSON 数字 1,RawMessage
	// 原样保存为 `1`(无引号),与迁移前 log.Printf("%s") 输出一致。
	if fields["request_id"] != "1" {
		t.Fatalf("request_id field mismatch: %#v", fields)
	}
	if fields["error"] != "driver: disk I/O error" {
		t.Fatalf("error field mismatch: %#v", fields)
	}
	if strings.Contains(entries[0].Message, "driver") || strings.Contains(entries[0].Message, "disk") {
		t.Fatalf("internal details must stay in fields, not leak into message: %q", entries[0].Message)
	}
}

// 生产装配路径:NewMCPHandler 未传 logger 时回退 Nop(构造可用,行为不变);
// 显式传 WithMCPLogger 时真实 logger 生效——两条路径在装配函数上验证,
// 不依赖手工字段赋值。
func TestMCPHandlerLoggerAssembly(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)

	// 生产形态:显式注入。
	handler := NewMCPHandler(nil, []byte("secret-at-least-32-bytes-long!!!"), nil, nil, nil, 5, WithMCPLogger(zap.New(core)))
	handler.internalError(nil, json.RawMessage(`"req-1"`), errors.New("boom"))
	entries := logs.FilterLevelExact(zapcore.ErrorLevel).FilterMessage("mcp: internal error").All()
	if len(entries) != 1 {
		t.Fatalf("explicit logger must receive the event, got %d entries", len(entries))
	}
	// 手工构造的 id 是 JSON 字符串形态,RawMessage 原样含引号——原样保真。
	if entries[0].ContextMap()["request_id"] != `"req-1"` {
		t.Fatalf("request_id field mismatch: %#v", entries[0].ContextMap())
	}

	// 测试/最小装配:未传 logger 回退 Nop,调用不 panic、不产出观测事件。
	nopHandler := NewMCPHandler(nil, []byte("secret-at-least-32-bytes-long!!!"), nil, nil, nil, 5)
	nopHandler.internalError(nil, json.RawMessage(`"req-2"`), errors.New("boom"))
	if got := logs.FilterMessage("mcp: internal error").Len(); got != 1 {
		t.Fatalf("nop fallback must not produce observable entries, total=%d", got)
	}
}

// token TTL 语义回归哨兵:WithMCPLogger 不触碰鉴权;此测试锁定 token 过期
// 仍走 401(不进 internalError),防止后续改动把鉴权失败误记为基础设施错误。
func TestMCPExpiredTokenStaysUnauthorized(t *testing.T) {
	f := newMCPFixture(t)
	core, logs := observer.New(zapcore.DebugLevel)
	f.handler.logger = zap.New(core)

	issuedAt := f.now().Add(-2 * time.Hour)
	token, err := resulttoken.Issue(f.secret, resulttoken.Claims{
		TaskID: mcpTestEndpointTask, RunID: "run-expired", Role: "DeveloperWorker", TTL: time.Hour,
	}, issuedAt)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	code, response := f.postRPC(t, token, "tools/list", map[string]any{})
	if code != http.StatusUnauthorized {
		t.Fatalf("expired token must be 401, got %d", code)
	}
	if logs.Len() != 0 {
		t.Fatalf("unauthorized path must not log internal errors, got %d entries", logs.Len())
	}
	// 401 响应是既有扁平形态 {"error":"unauthorized"},非 JSON-RPC error
	// object(code/message/data):锁定它未被新装配改变。
	if response["error"] != "unauthorized" || response["code"] != nil {
		t.Fatalf("401 body shape changed, got %#v", response)
	}
}
