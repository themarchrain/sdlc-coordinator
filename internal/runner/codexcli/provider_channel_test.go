package codexcli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

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
