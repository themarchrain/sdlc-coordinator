package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
)

// 端到端矫正回路：真 SQLite + 真 Orchestrator + 真 ResultChannelCoordinator +
// /mcp 端点。复刻收敛路径——前 2 次校验拒绝（不同问题）、第 3 次
// 修正后接受；拒绝行、留痕、终态全部落库。
func TestMCPEndToEndCorrectionLoopFinalizesRun(t *testing.T) {
	f := newMCPFixture(t)
	f.createDeveloperRun(t, "run-mcp-e2e")
	token := f.token(mcpTestEndpointTask, "run-mcp-e2e", "DeveloperWorker", f.now())

	// initialize + tools/list（codex/claude 会话开场的两步）。
	code, response := f.postRPC(t, token, "initialize", map[string]any{"protocolVersion": "2025-06-18"})
	if code != http.StatusOK || response["result"] == nil {
		t.Fatalf("initialize failed: %d", code)
	}
	_, listResponse := f.postRPC(t, token, "tools/list", map[string]any{})
	tools, _ := listResponse["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("role-filtered tool catalog expected, got %#v", tools)
	}

	// 第 1 次：change_ready 缺 tests。
	_, result, payload := f.callTool(t, token, results.SubmitDeveloperResultTool,
		`{"task_id":"`+mcpTestEndpointTask+`","run_id":"run-mcp-e2e","outcome":"change_ready","summary":"done","merge_request":{"title":"T","description":"D"}}`)
	if !result["isError"].(bool) || toolStatus(payload) != "rejected" {
		t.Fatalf("attempt 1 must be rejected: %#v", payload)
	}
	if !strings.Contains(problemText(t, payload), "tests") {
		t.Fatalf("attempt 1 problems must name tests: %#v", payload)
	}

	// 第 2 次：补了 tests 但 summary 为空 + 顺带一个 camelCase 别名（规范化后仍因 summary 被拒）。
	_, result, payload = f.callTool(t, token, results.SubmitDeveloperResultTool,
		`{"taskId":"`+mcpTestEndpointTask+`","runId":"run-mcp-e2e","outcome":"change_ready","summary":"","tests":[{"command":"go test ./...","passed":true,"summary":"green"}],"mergeRequest":{"title":"T","description":"D"}}`)
	if !result["isError"].(bool) || toolStatus(payload) != "rejected" {
		t.Fatalf("attempt 2 must be rejected: %#v", payload)
	}
	if payload["attempt"].(float64) != 2 || payload["attempts_remaining"].(float64) != 3 {
		t.Fatalf("attempt accounting broken: %#v", payload)
	}

	// 第 3 次：补全 summary —— 接受并终态化（fix_failed 语义，无 git 交付需求）。
	_, result, payload = f.callTool(t, token, results.SubmitDeveloperResultTool,
		`{"taskId":"`+mcpTestEndpointTask+`","runId":"run-mcp-e2e","outcome":"fix_failed","summary":"root cause fixed but tests still red","blockerReason":"remaining failure in pkg/x"}`)
	if result["isError"].(bool) || toolStatus(payload) != "accepted" {
		t.Fatalf("attempt 3 must be accepted: %#v", payload)
	}
	if payload["run_status"] != string(model.RunStatusFailed) {
		t.Fatalf("run must be terminal, got %#v", payload)
	}

	runRecord, err := f.runs.GetByRunID(context.Background(), "run-mcp-e2e")
	if err != nil {
		t.Fatalf("load run: %v", err)
	}
	if runRecord.Status != model.RunStatusFailed {
		t.Fatalf("run persisted status: %s", runRecord.Status)
	}
	task, _ := f.tasks.GetByTaskID(context.Background(), mcpTestEndpointTask)
	if task.Status != model.TaskStatusHumanRequired && task.Status != model.TaskStatusBlocked && task.Status != model.TaskStatusRetryPending {
		t.Fatalf("task must follow canonical failed semantics, got %s", task.Status)
	}
	if count, _ := f.rejections.CountByRun(context.Background(), "run-mcp-e2e"); count != 2 {
		t.Fatalf("exactly two rejection rows expected, got %d", count)
	}

	// 首提即终态后的第四把调用：已终态，不再计数。
	_, result, payload = f.callTool(t, token, results.SubmitDeveloperResultTool,
		`{"task_id":"`+mcpTestEndpointTask+`","run_id":"run-mcp-e2e","outcome":"fix_failed","summary":"another","blocker_reason":"z"}`)
	if !result["isError"].(bool) || toolStatus(payload) != "already_final" {
		t.Fatalf("post-terminal call must be already_final: %#v", payload)
	}
	if count, _ := f.rejections.CountByRun(context.Background(), "run-mcp-e2e"); count != 2 {
		t.Fatalf("already_final must not add rows, got %d", count)
	}
}

func problemText(t *testing.T, payload map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(payload["problems"])
	if err != nil {
		t.Fatalf("marshal problems: %v", err)
	}
	return string(raw)
}
