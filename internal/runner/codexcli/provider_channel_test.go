package codexcli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/runner"
)

func channelRequest(workspaceRoot string, channel *runner.ResultSubmissionChannel) runner.ProviderRequest {
	return runner.ProviderRequest{
		ExecutionID: "run-codex-channel", TaskID: "task-codex-channel", Role: "DeveloperWorker",
		WorkspacePath: workspaceRoot, PromptFile: "prompt.md", SchemaFile: "result-schema.json",
		Provider:         runner.ProviderConfig{Name: "codex-cli", Revision: 3, BaseURL: "https://gateway.example/v1", APIKey: "api-secret"},
		ResultSubmission: channel,
	}
}

func writeCodexChannelFiles(t *testing.T, workspaceRoot string) {
	t.Helper()
	writeCodexTestFile(t, workspaceRoot, "prompt.md", "Do the task.\n")
	writeCodexTestFile(t, workspaceRoot, "result-schema.json", `{"type":"object","required":["verdict"],"properties":{"verdict":{"type":"string"}}}`)
}

func TestProviderInjectsCoordinatorMCPServerWhenChannelPresent(t *testing.T) {
	for _, role := range []string{"DeveloperWorker", "TesterWorker"} {
		t.Run(role, func(t *testing.T) {
			workspaceRoot := t.TempDir()
			writeCodexChannelFiles(t, workspaceRoot)
			command := &fakeCommandRunner{lines: [][]byte{[]byte(`{"type":"turn.completed","usage":{}}`)}}
			provider := newProvider("codex-test", command)
			channel := &runner.ResultSubmissionChannel{EndpointURL: "http://coordinator.example:8080/mcp", BearerToken: "run-token-secret-value"}

			request := channelRequest(workspaceRoot, channel)
			request.Role = role
			if _, err := provider.Execute(context.Background(), request, func(string, map[string]any) {}); err != nil {
				t.Fatalf("execute provider: %v", err)
			}

			joinedArgs := strings.Join(command.spec.Args, " ")
			for _, setting := range []string{
				`mcp_servers.coordinator.url="http://coordinator.example:8080/mcp"`,
				`mcp_servers.coordinator.bearer_token_env_var="COORDINATOR_RUN_TOKEN"`,
			} {
				if !containsArgumentPair(command.spec.Args, "-c", setting) {
					t.Fatalf("missing -c %s in %#v", setting, command.spec.Args)
				}
			}
			if strings.Contains(joinedArgs, "run-token-secret-value") {
				t.Fatalf("run token leaked into arguments: %#v", command.spec.Args)
			}
			environment := environmentMap(command.spec.Env)
			if environment["COORDINATOR_RUN_TOKEN"] != "run-token-secret-value" {
				t.Fatalf("run token missing from codex environment: %#v", environment)
			}
			if last := command.spec.Args[len(command.spec.Args)-1]; last != "-" {
				t.Fatalf("stdin placeholder must stay last: %#v", command.spec.Args)
			}
		})
	}
}

func TestProviderOmitsMCPServerWithoutChannel(t *testing.T) {
	for _, role := range []string{"DeveloperWorker", "TesterWorker"} {
		t.Run(role, func(t *testing.T) {
			workspaceRoot := t.TempDir()
			writeCodexChannelFiles(t, workspaceRoot)
			command := &fakeCommandRunner{lines: [][]byte{[]byte(`{"type":"turn.completed","usage":{}}`)}}
			provider := newProvider("codex-test", command)
			request := channelRequest(workspaceRoot, nil)
			request.Role = role
			if _, err := provider.Execute(context.Background(), request, func(string, map[string]any) {}); err != nil {
				t.Fatalf("execute provider: %v", err)
			}
			joinedArgs := strings.Join(command.spec.Args, " ")
			if strings.Contains(joinedArgs, "mcp_servers") || strings.Contains(joinedArgs, "COORDINATOR_RUN_TOKEN") {
				t.Fatalf("mcp config injected without channel: %#v", command.spec.Args)
			}
			if _, exists := environmentMap(command.spec.Env)["COORDINATOR_RUN_TOKEN"]; exists {
				t.Fatal("run token env injected without channel")
			}
		})
	}
}

func mcpToolCallEvents() [][]byte {
	return [][]byte{
		[]byte(`{"type":"item.started","item":{"id":"item_1","type":"mcp_tool_call","server":"coordinator","tool":"submit_developer_result","arguments":{"taskId":"task-1"},"result":null,"error":null,"status":"in_progress"}}`),
		[]byte(`{"type":"item.completed","item":{"id":"item_1","type":"mcp_tool_call","server":"coordinator","tool":"submit_developer_result","arguments":{"taskId":"task-1"},"result":{"content":[{"type":"text","text":"{\"status\":\"accepted\"}"}]},"error":null,"status":"completed"}}`),
	}
}

func TestEventParserSurfacesMCPCallDetails(t *testing.T) {
	var events []capturedProviderEvent
	parser := &eventParser{emit: func(eventType string, payload map[string]any) {
		events = append(events, capturedProviderEvent{eventType: eventType, payload: payload})
	}, provider: runner.ProviderConfig{APIKey: "api-secret"}}
	for _, line := range mcpToolCallEvents() {
		if err := parser.consume(line); err != nil {
			t.Fatalf("consume event: %v", err)
		}
	}
	if len(events) != 2 {
		t.Fatalf("unexpected event count: %d", len(events))
	}
	started, completed := events[0].payload, events[1].payload
	if started["item_type"] != "mcp_tool_call" || started["server"] != "coordinator" || started["tool"] != "submit_developer_result" {
		t.Fatalf("started event missing tool identity: %#v", started)
	}
	if completed["status"] != "completed" {
		t.Fatalf("completed event status: %#v", completed)
	}
	if text, _ := completed["result_text"].(string); !strings.Contains(text, "accepted") {
		t.Fatalf("completed event missing result text: %#v", completed)
	}
}

// 端到端接线测试：channel 必须从 ProviderRequest 一路进入 eventParser，
// 保证生产构造点不再遗漏 token 脱敏（审查 P1 修复的回归锁定）。
func TestExecuteWiresRunTokenRedactionThroughEventParser(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeCodexChannelFiles(t, workspaceRoot)
	leak := `{"type":"item.completed","item":{"id":"item_9","type":"mcp_tool_call","server":"coordinator","tool":"submit_developer_result","arguments":{},"result":null,"error":{"message":"POST failed (Bearer run-token-secret-value)"},"status":"failed"}}`
	command := &fakeCommandRunner{lines: [][]byte{[]byte(leak), []byte(`{"type":"turn.completed","usage":{}}`)}}
	provider := newProvider("codex-test", command)
	channel := &runner.ResultSubmissionChannel{EndpointURL: "http://coordinator.example:8080/mcp", BearerToken: "run-token-secret-value"}

	var events []capturedProviderEvent
	if _, err := provider.Execute(context.Background(), channelRequest(workspaceRoot, channel), func(eventType string, payload map[string]any) {
		events = append(events, capturedProviderEvent{eventType: eventType, payload: payload})
	}); err != nil {
		t.Fatalf("execute provider: %v", err)
	}
	for _, event := range events {
		raw, _ := json.Marshal(event.payload)
		if strings.Contains(string(raw), "run-token-secret-value") {
			t.Fatalf("run token leaked into event stream: %s", raw)
		}
	}
	found := false
	for _, event := range events {
		if message, _ := event.payload["error_message"].(string); strings.Contains(message, "[REDACTED]") {
			found = true
		}
	}
	if !found {
		t.Fatalf("redacted error_message missing from events: %#v", events)
	}
}

func TestEventParserSurfacesFailedMCPCallAndRedactsSecrets(t *testing.T) {
	var events []capturedProviderEvent
	parser := &eventParser{emit: func(eventType string, payload map[string]any) {
		events = append(events, capturedProviderEvent{eventType: eventType, payload: payload})
	}, provider: runner.ProviderConfig{APIKey: "api-secret"},
		channel: &runner.ResultSubmissionChannel{BearerToken: "run-token-secret-value"}}
	line := []byte(`{"type":"item.completed","item":{"id":"item_2","type":"mcp_tool_call","server":"coordinator","tool":"submit_developer_result","arguments":{},"result":null,"error":{"message":"connect failed with run-token-secret-value"},"status":"failed"}}`)
	if err := parser.consume(line); err != nil {
		t.Fatalf("consume event: %v", err)
	}
	payload := events[0].payload
	if payload["status"] != "failed" {
		t.Fatalf("failed status missing: %#v", payload)
	}
	if message, _ := payload["error_message"].(string); !strings.Contains(message, "[REDACTED]") || strings.Contains(message, "run-token-secret-value") {
		t.Fatalf("error message not redacted: %#v", payload)
	}
}

func TestEventParserFlagsMetadataErrorAsNonFatal(t *testing.T) {
	var events []capturedProviderEvent
	parser := &eventParser{emit: func(eventType string, payload map[string]any) {
		events = append(events, capturedProviderEvent{eventType: eventType, payload: payload})
	}}
	line := []byte(`{"type":"item.completed","item":{"id":"item_3","type":"error","message":"Model metadata for 'deepseek-v4-flash' not found. Defaulting to fallback metadata; this can degrade performance and cause issues."}}`)
	if err := parser.consume(line); err != nil {
		t.Fatalf("consume event: %v", err)
	}
	payload := events[0].payload
	if payload["non_fatal"] != true {
		t.Fatalf("metadata error must be flagged non_fatal: %#v", payload)
	}
	if payload["message"] == nil {
		t.Fatalf("message should stay visible for observability: %#v", payload)
	}
}

func TestEventParserDoesNotFlagOtherErrorsAsNonFatal(t *testing.T) {
	var events []capturedProviderEvent
	parser := &eventParser{emit: func(eventType string, payload map[string]any) {
		events = append(events, capturedProviderEvent{eventType: eventType, payload: payload})
	}}
	line := []byte(`{"type":"item.completed","item":{"id":"item_4","type":"error","message":"gateway returned 502"}}`)
	if err := parser.consume(line); err != nil {
		t.Fatalf("consume event: %v", err)
	}
	if _, exists := events[0].payload["non_fatal"]; exists {
		t.Fatalf("real errors must not be flagged non_fatal: %#v", events[0].payload)
	}
	if !strings.Contains(jsonEncode(t, events[0].payload), "502") {
		t.Fatalf("error message lost: %#v", events[0].payload)
	}
}

func jsonEncode(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}

// acceptedReceiptText 构造与协调器 MCP 工具响应一致的接受回执 JSON 文本。
func acceptedReceiptText(taskID, runID, runStatus string) string {
	raw, _ := json.Marshal(map[string]any{
		"status": "accepted", "max_attempts": 5, "task_id": taskID, "run_id": runID,
		"task_status": "succeeded", "run_status": runStatus, "next_step": "stop",
	})
	return string(raw)
}

// statusPayloadText 构造任意 status 的 MCP 工具响应文本。
func statusPayloadText(status, taskID, runID, runStatus string) string {
	raw, _ := json.Marshal(map[string]any{
		"status": status, "task_id": taskID, "run_id": runID, "run_status": runStatus,
	})
	return string(raw)
}

func mcpResultContent(text string) map[string]any {
	return map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}
}

// mcpToolItemEvent 构造 item.started/item.completed 的 MCP 工具调用事件，
// 形状对齐 codex-cli 0.150 的 McpToolCallItem 序列化（server/tool/status/
// result/error）。
func mcpToolItemEvent(eventType, server, tool, status string, content map[string]any, errMessage string) []byte {
	item := map[string]any{
		"id": "item_1", "type": "mcp_tool_call", "server": server, "tool": tool,
		"arguments": map[string]any{}, "status": status, "result": content, "error": nil,
	}
	if errMessage != "" {
		item["error"] = map[string]any{"message": errMessage}
	}
	raw, _ := json.Marshal(map[string]any{"type": eventType, "item": item})
	return raw
}

func channelRequestWithIdentity(workspaceRoot, taskID, runID, role string, channel *runner.ResultSubmissionChannel) runner.ProviderRequest {
	request := channelRequest(workspaceRoot, channel)
	request.TaskID = taskID
	request.ExecutionID = runID
	request.Role = role
	return request
}

func assertDeliveryMarker(t *testing.T, raw json.RawMessage, taskID, runID string) {
	t.Helper()
	var envelope map[string]struct {
		TaskID string `json:"task_id"`
		RunID  string `json:"run_id"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("delivery marker is not valid JSON: %v (%s)", err, raw)
	}
	marker, ok := envelope["delivery_marker"]
	if !ok {
		t.Fatalf("delivery marker envelope missing: %s", raw)
	}
	if marker.TaskID != taskID || marker.RunID != runID || marker.Reason == "" {
		t.Fatalf("delivery marker identity mismatch: %#v", marker)
	}
}

// 主通道回执存在时，最终消息非 JSON 或缺失都必须按交付标记收尾，
// 而不是把执行记成失败；三种角色共用同一 Provider。
func TestProviderMCPAcceptedReceiptFinalizesWhenFinalMessageIsNotJSON(t *testing.T) {
	for _, tc := range []struct{ name, role, tool string }{
		{name: "developer", role: "DeveloperWorker", tool: results.SubmitDeveloperResultTool},
		{name: "reviewer", role: "ReviewerWorker", tool: results.SubmitReviewerResultTool},
		{name: "tester", role: "TesterWorker", tool: results.SubmitTesterResultTool},
	} {
		for _, finalCase := range []struct {
			name    string
			final   string
			noFinal bool
		}{
			{name: "natural language final", final: "结果已被接受，任务完成：本次改动已验证通过。"},
			{name: "missing final file", noFinal: true},
		} {
			t.Run(tc.name+" "+finalCase.name, func(t *testing.T) {
				workspaceRoot := t.TempDir()
				writeCodexChannelFiles(t, workspaceRoot)
				request := channelRequestWithIdentity(workspaceRoot, "task-codex-accepted", "run-codex-accepted", tc.role,
					&runner.ResultSubmissionChannel{EndpointURL: "http://coordinator.example:8080/mcp", BearerToken: "run-token-secret-value"})
				command := &fakeCommandRunner{
					lines: [][]byte{
						mcpToolItemEvent("item.started", "coordinator", tc.tool, "in_progress", nil, ""),
						mcpToolItemEvent("item.completed", "coordinator", tc.tool, "completed",
							mcpResultContent(acceptedReceiptText(request.TaskID, request.ExecutionID, "succeeded")), ""),
						[]byte(`{"type":"turn.completed","usage":{}}`),
					},
					final: finalCase.final, noFinal: finalCase.noFinal,
				}
				result, err := newProvider("codex-test", command).Execute(context.Background(), request, func(string, map[string]any) {})
				if err != nil {
					t.Fatalf("accepted MCP run must finalize without a JSON final message: %v", err)
				}
				assertDeliveryMarker(t, result, request.TaskID, request.ExecutionID)
			})
		}
	}
}

// 最终消息本身是合法 JSON 时保持原行为：业务契约仍由协调器验证，
// 即使存在接受回执也不改写 Agent 的交付内容。
func TestProviderKeepsBusinessJSONFinalMessageEvenWithAcceptanceReceipt(t *testing.T) {
	for _, test := range []struct {
		name   string
		status string
	}{
		{name: "accepted receipt", status: "accepted"},
		{name: "rejected receipt", status: "rejected"},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspaceRoot := t.TempDir()
			writeCodexChannelFiles(t, workspaceRoot)
			request := channelRequestWithIdentity(workspaceRoot, "task-codex-keep-json", "run-codex-keep-json", "DeveloperWorker",
				&runner.ResultSubmissionChannel{EndpointURL: "http://coordinator.example:8080/mcp", BearerToken: "run-token"})
			command := &fakeCommandRunner{
				lines: [][]byte{
					mcpToolItemEvent("item.completed", "coordinator", results.SubmitDeveloperResultTool, "completed",
						mcpResultContent(statusPayloadText(test.status, request.TaskID, request.ExecutionID, "succeeded")), ""),
				},
				final: `{"verdict":"approved"}`,
			}
			result, err := newProvider("codex-test", command).Execute(context.Background(), request, func(string, map[string]any) {})
			if err != nil {
				t.Fatalf("valid business JSON final message must keep working: %v", err)
			}
			if string(result) != `{"verdict":"approved"}` {
				t.Fatalf("business JSON was replaced by another delivery path: %s", result)
			}
		})
	}
}

// 弱信号一律不算接受回执：只有工具完成事件里的结构化回执才能解锁标记。
func TestProviderRejectsWeakOrForeignAcceptanceSignals(t *testing.T) {
	const taskID, runID = "task-codex-weak", "run-codex-weak"
	requestFor := func(t *testing.T) runner.ProviderRequest {
		workspaceRoot := t.TempDir()
		writeCodexChannelFiles(t, workspaceRoot)
		return channelRequestWithIdentity(workspaceRoot, taskID, runID, "DeveloperWorker",
			&runner.ResultSubmissionChannel{EndpointURL: "http://coordinator.example:8080/mcp", BearerToken: "run-token"})
	}
	tests := []struct {
		name    string
		lines   [][]byte
		final   string
		noFinal bool
		runErr  error
	}{
		{
			name:  "mcp configured but never called",
			final: "结果已被接受，任务完成。",
		},
		{
			name: "tool only started",
			lines: [][]byte{
				mcpToolItemEvent("item.started", "coordinator", results.SubmitDeveloperResultTool, "in_progress",
					mcpResultContent(acceptedReceiptText(taskID, runID, "succeeded")), ""),
			},
			final: "结果已被接受。",
		},
		{
			name: "rejected",
			lines: [][]byte{
				mcpToolItemEvent("item.completed", "coordinator", results.SubmitDeveloperResultTool, "completed",
					mcpResultContent(statusPayloadText("rejected", taskID, runID, "running")), ""),
			},
			final: "自述已接受。",
		},
		{
			name: "rejected_final",
			lines: [][]byte{
				mcpToolItemEvent("item.completed", "coordinator", results.SubmitDeveloperResultTool, "completed",
					mcpResultContent(statusPayloadText("rejected_final", taskID, runID, "running")), ""),
			},
			final: "自述已接受。",
		},
		{
			name: "processing",
			lines: [][]byte{
				mcpToolItemEvent("item.completed", "coordinator", results.SubmitDeveloperResultTool, "completed",
					mcpResultContent(statusPayloadText("processing", taskID, runID, "running")), ""),
			},
			final: "自述已接受。",
		},
		{
			name: "already_final",
			lines: [][]byte{
				mcpToolItemEvent("item.completed", "coordinator", results.SubmitDeveloperResultTool, "completed",
					mcpResultContent(statusPayloadText("already_final", taskID, runID, "succeeded")), ""),
			},
			final: "自述已接受。",
		},
		{
			name: "tool failed with error object",
			lines: [][]byte{
				mcpToolItemEvent("item.completed", "coordinator", results.SubmitDeveloperResultTool, "failed",
					nil, "connect failed"),
			},
			final: "自述已接受。",
		},
		{
			name: "result isError true",
			lines: [][]byte{
				mcpToolItemEvent("item.completed", "coordinator", results.SubmitDeveloperResultTool, "completed",
					map[string]any{
						"content": []any{map[string]any{"type": "text", "text": acceptedReceiptText(taskID, runID, "succeeded")}},
						"isError": true,
					}, ""),
			},
			final: "自述已接受。",
		},
		{
			name: "wrong server",
			lines: [][]byte{
				mcpToolItemEvent("item.completed", "other-server", results.SubmitDeveloperResultTool, "completed",
					mcpResultContent(acceptedReceiptText(taskID, runID, "succeeded")), ""),
			},
			final: "自述已接受。",
		},
		{
			name: "wrong tool for role",
			lines: [][]byte{
				mcpToolItemEvent("item.completed", "coordinator", results.SubmitReviewerResultTool, "completed",
					mcpResultContent(acceptedReceiptText(taskID, runID, "succeeded")), ""),
			},
			final: "自述已接受。",
		},
		{
			name: "wrong task identity",
			lines: [][]byte{
				mcpToolItemEvent("item.completed", "coordinator", results.SubmitDeveloperResultTool, "completed",
					mcpResultContent(acceptedReceiptText("task-other", runID, "succeeded")), ""),
			},
			final: "自述已接受。",
		},
		{
			name: "wrong run identity",
			lines: [][]byte{
				mcpToolItemEvent("item.completed", "coordinator", results.SubmitDeveloperResultTool, "completed",
					mcpResultContent(acceptedReceiptText(taskID, "run-other", "succeeded")), ""),
			},
			final: "自述已接受。",
		},
		{
			name: "run not terminal",
			lines: [][]byte{
				mcpToolItemEvent("item.completed", "coordinator", results.SubmitDeveloperResultTool, "completed",
					mcpResultContent(acceptedReceiptText(taskID, runID, "running")), ""),
			},
			final: "自述已接受。",
		},
		{
			name: "malformed receipt JSON",
			lines: [][]byte{
				mcpToolItemEvent("item.completed", "coordinator", results.SubmitDeveloperResultTool, "completed",
					mcpResultContent("{not-json"), ""),
			},
			final: "自述已接受。",
		},
		{
			name:  "natural language self claim only",
			final: "结果已被接受（accepted），任务完成。",
		},
		{
			name: "turn failed after receipt",
			lines: [][]byte{
				mcpToolItemEvent("item.completed", "coordinator", results.SubmitDeveloperResultTool, "completed",
					mcpResultContent(acceptedReceiptText(taskID, runID, "succeeded")), ""),
				[]byte(`{"type":"turn.failed","error":{"code":"server_error","message":"gateway exploded"}}`),
			},
			final: "结果已被接受。",
		},
		{
			name: "command error after receipt",
			lines: [][]byte{
				mcpToolItemEvent("item.completed", "coordinator", results.SubmitDeveloperResultTool, "completed",
					mcpResultContent(acceptedReceiptText(taskID, runID, "succeeded")), ""),
			},
			runErr: errors.New("exit status 1"),
		},
		{
			name: "deadline exceeded after receipt",
			lines: [][]byte{
				mcpToolItemEvent("item.completed", "coordinator", results.SubmitDeveloperResultTool, "completed",
					mcpResultContent(acceptedReceiptText(taskID, runID, "succeeded")), ""),
			},
			runErr: context.DeadlineExceeded,
		},
		{
			name: "cancelled after receipt",
			lines: [][]byte{
				mcpToolItemEvent("item.completed", "coordinator", results.SubmitDeveloperResultTool, "completed",
					mcpResultContent(acceptedReceiptText(taskID, runID, "succeeded")), ""),
			},
			runErr: context.Canceled,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := requestFor(t)
			command := &fakeCommandRunner{lines: test.lines, final: test.final, noFinal: test.noFinal, err: test.runErr}
			result, err := newProvider("codex-test", command).Execute(context.Background(), request, func(string, map[string]any) {})
			if err == nil {
				t.Fatalf("weak or foreign acceptance signal must not finalize the run, got result=%s", result)
			}
			if len(result) != 0 {
				t.Fatalf("failure must not carry a delivery marker: %s", result)
			}
		})
	}
}

// finalPath 被目录占据时属于非缺失类读取故障：即使有接受回执也必须如实失败。
func TestProviderFailsWhenFinalFileIsUnreadableEvenWithReceipt(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeCodexChannelFiles(t, workspaceRoot)
	request := channelRequestWithIdentity(workspaceRoot, "task-codex-io", "run-codex-io", "DeveloperWorker",
		&runner.ResultSubmissionChannel{EndpointURL: "http://coordinator.example:8080/mcp", BearerToken: "run-token"})
	command := &unreadableFinalRunner{lines: [][]byte{
		mcpToolItemEvent("item.completed", "coordinator", results.SubmitDeveloperResultTool, "completed",
			mcpResultContent(acceptedReceiptText(request.TaskID, request.ExecutionID, "succeeded")), ""),
	}}
	result, err := newProvider("codex-test", command).Execute(context.Background(), request, func(string, map[string]any) {})
	if err == nil || !strings.Contains(err.Error(), "read codex final result") {
		t.Fatalf("non-missing read failure must surface as a real execution error, got %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("read failure must not produce a delivery marker: %s", result)
	}
}

// spoofedMarkerText 构造 Agent 自己手写的"交付标记"文本(带诱饵字段)。
func spoofedMarkerText(taskID, runID string) string {
	raw, _ := json.Marshal(map[string]any{"delivery_marker": map[string]any{
		"task_id": taskID, "run_id": runID, "reason": "mcp_accepted", "decoy": "agent-authored",
	}})
	return string(raw)
}

// 内部交付标记只能由 Provider 在核验协调器回执后生成:Agent 文本仿写的标记
// 不构成交付凭据,没有真回执一律拒绝;有真回执时也重新生成标记,不透传
// Agent 的版本。
func TestProviderRejectsSpoofedDeliveryMarkers(t *testing.T) {
	const taskID, runID = "task-codex-spoof", "run-codex-spoof"
	newRequest := func(t *testing.T) runner.ProviderRequest {
		workspaceRoot := t.TempDir()
		writeCodexChannelFiles(t, workspaceRoot)
		return channelRequestWithIdentity(workspaceRoot, taskID, runID, "DeveloperWorker",
			&runner.ResultSubmissionChannel{EndpointURL: "http://coordinator.example:8080/mcp", BearerToken: "run-token"})
	}
	t.Run("without receipt", func(t *testing.T) {
		command := &fakeCommandRunner{final: spoofedMarkerText("task-spoofed", "run-spoofed")}
		result, err := newProvider("codex-test", command).Execute(context.Background(), newRequest(t), func(string, map[string]any) {})
		if err == nil {
			t.Fatalf("agent-authored delivery marker must not finalize a run: %s", result)
		}
		if len(result) != 0 {
			t.Fatalf("spoofed marker must not become the execution result: %s", result)
		}
	})
	t.Run("with real receipt", func(t *testing.T) {
		request := newRequest(t)
		command := &fakeCommandRunner{
			lines: [][]byte{
				mcpToolItemEvent("item.completed", "coordinator", results.SubmitDeveloperResultTool, "completed",
					mcpResultContent(acceptedReceiptText(request.TaskID, request.ExecutionID, "succeeded")), ""),
			},
			final: spoofedMarkerText("task-spoofed", "run-spoofed"),
		}
		result, err := newProvider("codex-test", command).Execute(context.Background(), request, func(string, map[string]any) {})
		if err != nil {
			t.Fatalf("run with a real receipt must finalize: %v", err)
		}
		if string(result) != string(runner.NewDeliveryMarker(request.TaskID, request.ExecutionID)) {
			t.Fatalf("delivery marker must be provider-generated instead of the agent text: %s", result)
		}
	})
}

// turn.failed 是 CLI 的终止失败事件:即使进程正常返回、最终消息是合法
// JSON,也必须如实失败,不得被记成成功。
func TestProviderFailsTurnFailureBeforeAcceptingFinalResult(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeCodexChannelFiles(t, workspaceRoot)
	request := channelRequestWithIdentity(workspaceRoot, "task-codex-turn-failed", "run-codex-turn-failed", "DeveloperWorker", nil)
	command := &fakeCommandRunner{
		lines: [][]byte{
			[]byte(`{"type":"turn.failed","error":{"code":"server_error","message":"gateway exploded"}}`),
		},
		final: `{"verdict":"approved"}`,
	}
	result, err := newProvider("codex-test", command).Execute(context.Background(), request, func(string, map[string]any) {})
	if err == nil {
		t.Fatalf("turn failure must not be recorded as success: %s", result)
	}
	if !strings.Contains(err.Error(), "gateway exploded") {
		t.Fatalf("turn failure detail was lost: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("turn failure must not produce a result: %s", result)
	}
}

type unreadableFinalRunner struct {
	lines [][]byte
}

func (f *unreadableFinalRunner) Run(_ context.Context, spec commandSpec, onLine func([]byte) error) error {
	for _, line := range f.lines {
		if err := onLine(line); err != nil {
			return err
		}
	}
	finalPath := argumentValue(spec.Args, "--output-last-message")
	if finalPath == "" {
		return errors.New("output-last-message path missing")
	}
	return os.MkdirAll(finalPath, 0o755)
}
