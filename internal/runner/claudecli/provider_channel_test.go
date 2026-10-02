package claudecli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sdlc-coordinator/internal/runner"
)

func TestProviderInjectsCoordinatorMCPConfigWhenChannelPresent(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeClaudeTestFile(t, workspaceRoot, "prompt.md", "Do the task.\n")
	writeClaudeTestFile(t, workspaceRoot, "result-schema.json", `{"type":"object","required":["verdict"],"properties":{"verdict":{"type":"string"}}}`)
	command := &fakeCommandRunner{lines: [][]byte{
		[]byte(`{"type":"result","subtype":"success","is_error":false,"structured_output":{"verdict":"approved"}}`),
	}}
	var configRaw []byte
	command.onRun = func(spec commandSpec) {
		configFlag := ""
		for index, arg := range spec.Args {
			if arg == "--mcp-config" && index+1 < len(spec.Args) {
				configFlag = spec.Args[index+1]
			}
		}
		if configFlag == "" {
			t.Errorf("--mcp-config missing: %#v", spec.Args)
			return
		}
		if !filepath.IsAbs(configFlag) || strings.Contains(configFlag, workspaceRoot) {
			t.Errorf("mcp config must live outside the workspace: %q", configFlag)
			return
		}
		raw, err := os.ReadFile(configFlag)
		if err != nil {
			t.Errorf("read mcp config: %v", err)
			return
		}
		configRaw = raw
	}
	provider := newProvider("claude-test", command)
	channel := &runner.ResultSubmissionChannel{EndpointURL: "http://coordinator.example:8080/mcp", BearerToken: "run-token-secret-value"}

	var events []capturedProviderEvent
	_, err := provider.Execute(context.Background(), runner.ProviderRequest{
		ExecutionID: "run-claude-channel", TaskID: "task-claude-channel", Role: "ReviewerWorker",
		WorkspacePath: workspaceRoot, PromptFile: "prompt.md", SchemaFile: "result-schema.json",
		Provider:         runner.ProviderConfig{Name: "claude-cli", Revision: 2, APIKey: "api-secret"},
		ResultSubmission: channel,
	}, func(eventType string, payload map[string]any) {
		events = append(events, capturedProviderEvent{eventType: eventType, payload: payload})
	})
	if err != nil {
		t.Fatalf("execute provider: %v", err)
	}

	joinedArgs := strings.Join(command.spec.Args, " ")
	if !strings.Contains(joinedArgs, "--strict-mcp-config") {
		t.Fatalf("--strict-mcp-config missing: %#v", command.spec.Args)
	}
	if strings.Contains(joinedArgs, "run-token-secret-value") {
		t.Fatalf("run token leaked into arguments: %#v", command.spec.Args)
	}
	if configRaw == nil {
		t.Fatal("mcp config was not captured during run")
	}
	var config struct {
		MCPServers map[string]struct {
			Type    string            `json:"type"`
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(configRaw, &config); err != nil {
		t.Fatalf("decode mcp config: %v", err)
	}
	server, ok := config.MCPServers["coordinator"]
	if !ok || server.Type != "http" || server.URL != "http://coordinator.example:8080/mcp" {
		t.Fatalf("unexpected mcp config: %s", configRaw)
	}
	if server.Headers["Authorization"] != "Bearer run-token-secret-value" {
		t.Fatalf("authorization header missing: %#v", server.Headers)
	}
}

func TestProviderOmitsMCPConfigWithoutChannel(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeClaudeTestFile(t, workspaceRoot, "prompt.md", "Do the task.\n")
	writeClaudeTestFile(t, workspaceRoot, "result-schema.json", `{"type":"object","required":["verdict"],"properties":{"verdict":{"type":"string"}}}`)
	command := &fakeCommandRunner{lines: [][]byte{
		[]byte(`{"type":"result","subtype":"success","is_error":false,"structured_output":{"verdict":"approved"}}`),
	}}
	provider := newProvider("claude-test", command)
	if _, err := provider.Execute(context.Background(), runner.ProviderRequest{
		ExecutionID: "run-claude-plain", TaskID: "task-claude-plain", Role: "ReviewerWorker",
		WorkspacePath: workspaceRoot, PromptFile: "prompt.md", SchemaFile: "result-schema.json",
		Provider: runner.ProviderConfig{Name: "claude-cli", Revision: 2, APIKey: "api-secret"},
	}, func(string, map[string]any) {}); err != nil {
		t.Fatalf("execute provider: %v", err)
	}
	joinedArgs := strings.Join(command.spec.Args, " ")
	if strings.Contains(joinedArgs, "mcp") || strings.Contains(joinedArgs, "mcp-config") {
		t.Fatalf("mcp flags injected without channel: %#v", command.spec.Args)
	}
}

func TestProviderInjectsCoordinatorMCPConfigForTesterWorker(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeClaudeTestFile(t, workspaceRoot, "prompt.md", "Verify the task.\n")
	writeClaudeTestFile(t, workspaceRoot, "result-schema.json", `{"type":"object","required":["verdict"],"properties":{"verdict":{"type":"string"}}}`)
	command := &fakeCommandRunner{lines: [][]byte{
		[]byte(`{"type":"result","subtype":"success","is_error":false,"structured_output":{"verdict":"passed"}}`),
	}}
	var configRaw []byte
	command.onRun = func(spec commandSpec) {
		for index, arg := range spec.Args {
			if arg == "--mcp-config" && index+1 < len(spec.Args) {
				raw, err := os.ReadFile(spec.Args[index+1])
				if err != nil {
					t.Errorf("read mcp config: %v", err)
					return
				}
				configRaw = raw
				return
			}
		}
		t.Errorf("--mcp-config missing: %#v", spec.Args)
	}
	provider := newProvider("claude-test", command)
	channel := &runner.ResultSubmissionChannel{EndpointURL: "http://coordinator.example:8080/mcp", BearerToken: "tester-run-token"}
	_, err := provider.Execute(context.Background(), runner.ProviderRequest{
		ExecutionID: "run-claude-tester", TaskID: "task-claude-tester", Role: "TesterWorker",
		WorkspacePath: workspaceRoot, PromptFile: "prompt.md", SchemaFile: "result-schema.json",
		Provider: runner.ProviderConfig{Name: "claude-cli", Revision: 2}, ResultSubmission: channel,
	}, func(string, map[string]any) {})
	if err != nil {
		t.Fatalf("execute TesterWorker provider: %v", err)
	}
	var config struct {
		MCPServers map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(configRaw, &config); err != nil {
		t.Fatalf("decode mcp config: %v", err)
	}
	server := config.MCPServers["coordinator"]
	if server.URL != channel.EndpointURL || server.Headers["Authorization"] != "Bearer "+channel.BearerToken {
		t.Fatalf("TesterWorker MCP config missing endpoint or run credential: %#v", server)
	}
}

func TestProviderOmitsMCPConfigForTesterWorkerWithoutChannel(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeClaudeTestFile(t, workspaceRoot, "prompt.md", "Verify the task.\n")
	writeClaudeTestFile(t, workspaceRoot, "result-schema.json", `{"type":"object","required":["verdict"],"properties":{"verdict":{"type":"string"}}}`)
	command := &fakeCommandRunner{lines: [][]byte{
		[]byte(`{"type":"result","subtype":"success","is_error":false,"structured_output":{"verdict":"passed"}}`),
	}}
	provider := newProvider("claude-test", command)
	if _, err := provider.Execute(context.Background(), runner.ProviderRequest{
		ExecutionID: "run-claude-tester-plain", TaskID: "task-claude-tester-plain", Role: "TesterWorker",
		WorkspacePath: workspaceRoot, PromptFile: "prompt.md", SchemaFile: "result-schema.json",
		Provider: runner.ProviderConfig{Name: "claude-cli", Revision: 2},
	}, func(string, map[string]any) {}); err != nil {
		t.Fatalf("execute TesterWorker provider: %v", err)
	}
	joinedArgs := strings.Join(command.spec.Args, " ")
	if strings.Contains(joinedArgs, "mcp") || strings.Contains(joinedArgs, "mcp-config") {
		t.Fatalf("mcp flags injected without channel: %#v", command.spec.Args)
	}
}

func TestClaudeStreamSurfacesSubmitToolUse(t *testing.T) {
	var events []capturedProviderEvent
	parser := &streamParser{emit: func(eventType string, payload map[string]any) {
		events = append(events, capturedProviderEvent{eventType: eventType, payload: payload})
	}}
	line := []byte(`{"type":"assistant","message":{"model":"m","content":[{"type":"text","text":"submitting"},{"type":"tool_use","id":"toolu_1","name":"submit_reviewer_result","input":{"task_id":"t"}}]}}`)
	if err := parser.consume(line); err != nil {
		t.Fatalf("consume: %v", err)
	}
	found := false
	for _, event := range events {
		if event.payload["item_type"] == "mcp_tool_call" && event.payload["tool"] == "submit_reviewer_result" {
			found = true
		}
	}
	if !found {
		t.Fatalf("submit tool_use was not surfaced: %#v", events)
	}
}

func TestClaudeStreamIgnoresNonSubmitToolUse(t *testing.T) {
	var events []capturedProviderEvent
	parser := &streamParser{emit: func(eventType string, payload map[string]any) {
		events = append(events, capturedProviderEvent{eventType: eventType, payload: payload})
	}}
	line := []byte(`{"type":"assistant","message":{"model":"m","content":[{"type":"tool_use","id":"toolu_2","name":"Bash","input":{"command":"ls"}}]}}`)
	if err := parser.consume(line); err != nil {
		t.Fatalf("consume: %v", err)
	}
	for _, event := range events {
		if event.payload["item_type"] == "mcp_tool_call" {
			t.Fatalf("non-submit tool use must not be surfaced as mcp call: %#v", event.payload)
		}
	}
}

func TestClaudeRedactCoversRunToken(t *testing.T) {
	parser := &streamParser{
		provider: runner.ProviderConfig{APIKey: "api-secret"},
		channel:  &runner.ResultSubmissionChannel{BearerToken: "run-token-secret-value"},
	}
	message := parser.failureMessage(map[string]any{"result": "connect failed with run-token-secret-value"})
	if strings.Contains(message, "run-token-secret-value") || !strings.Contains(message, "[REDACTED]") {
		t.Fatalf("run token not redacted: %s", message)
	}
}
