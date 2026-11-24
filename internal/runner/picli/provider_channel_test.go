package picli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/runner"
)

func TestProviderWritesMCPChannelConfig(t *testing.T) {
	workspaceRoot := t.TempDir()
	writePiTestFile(t, workspaceRoot, "prompt.md", "Do the task.\n")
	writePiTestFile(t, workspaceRoot, "result-schema.json", `{"type":"object"}`)
	command := &fakeCommandRunner{lines: [][]byte{
		acceptedToolEndLine(results.SubmitDeveloperResultTool, "task-pi-channel", "run-pi-channel"),
		finalMessageLine("已提交。"),
	}}
	var mcpRaw []byte
	command.onRun = func(spec commandSpec) {
		agentDir := environmentMap(spec.Env)[agentDirEnv]
		raw, err := os.ReadFile(filepath.Join(agentDir, "mcp.json"))
		if err != nil {
			t.Errorf("read mcp.json: %v", err)
			return
		}
		mcpRaw = raw
	}
	_, err := newProvider("pi-test", command).Execute(context.Background(), runner.ProviderRequest{
		ExecutionID: "run-pi-channel", TaskID: "task-pi-channel", Role: "DeveloperWorker",
		WorkspacePath: workspaceRoot, PromptFile: "prompt.md", SchemaFile: "result-schema.json",
		Provider: providerConfigForTest(),
		ResultSubmission: &runner.ResultSubmissionChannel{
			EndpointURL: "http://coordinator.example:8080/mcp", BearerToken: "run-token-secret-value",
		},
	}, func(string, map[string]any) {})
	if err != nil {
		t.Fatalf("execute provider: %v", err)
	}
	if strings.Contains(strings.Join(command.spec.Args, " "), "run-token-secret-value") {
		t.Fatalf("run token leaked into arguments: %#v", command.spec.Args)
	}
	if environmentMap(command.spec.Env)[mcpTokenEnv] != "run-token-secret-value" {
		t.Fatalf("run token missing from the child environment")
	}
	var config struct {
		MCPServers map[string]struct {
			URL         string            `json:"url"`
			Exposure    string            `json:"exposure"`
			Timeout     int               `json:"timeout"`
			Description string            `json:"description"`
			Headers     map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(mcpRaw, &config); err != nil {
		t.Fatalf("decode mcp.json: %v (%s)", err, mcpRaw)
	}
	server, ok := config.MCPServers[mcpServerName]
	if !ok {
		t.Fatalf("coordinator server missing: %s", mcpRaw)
	}
	if server.URL != "http://coordinator.example:8080/mcp" || server.Exposure != "direct" || server.Timeout != 600 {
		t.Fatalf("unexpected mcp server entry: %#v", server)
	}
	if server.Headers["Authorization"] != "Bearer ${PI_MCP_TOKEN}" {
		t.Fatalf("mcp token must stay an interpolation: %#v", server.Headers)
	}
	if strings.Contains(string(mcpRaw), "run-token-secret-value") {
		t.Fatalf("run token was written to mcp.json: %s", mcpRaw)
	}
}

func TestProviderOmitsMCPWhenChannelClosed(t *testing.T) {
	workspaceRoot := t.TempDir()
	writePiTestFile(t, workspaceRoot, "prompt.md", "Do the task.\n")
	writePiTestFile(t, workspaceRoot, "result-schema.json", `{"type":"object"}`)
	command := &fakeCommandRunner{lines: [][]byte{finalMessageLine(`{"verdict":"approved"}`)}}
	var hasMCPFile bool
	command.onRun = func(spec commandSpec) {
		agentDir := environmentMap(spec.Env)[agentDirEnv]
		if _, err := os.Stat(filepath.Join(agentDir, "mcp.json")); err == nil {
			hasMCPFile = true
		}
	}
	_, err := newProvider("pi-test", command).Execute(context.Background(), runner.ProviderRequest{
		ExecutionID: "run-pi-no-channel", TaskID: "task-pi-no-channel", Role: "ReviewerWorker",
		WorkspacePath: workspaceRoot, PromptFile: "prompt.md", SchemaFile: "result-schema.json",
		Provider: providerConfigForTest(),
	}, func(string, map[string]any) {})
	if err != nil {
		t.Fatalf("execute provider: %v", err)
	}
	if hasMCPFile {
		t.Fatal("mcp.json must not exist without a result channel")
	}
	if _, exists := environmentMap(command.spec.Env)[mcpTokenEnv]; exists {
		t.Fatal("run token env must not exist without a result channel")
	}
}
