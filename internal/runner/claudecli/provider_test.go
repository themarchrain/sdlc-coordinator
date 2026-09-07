package claudecli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sdlc-coordinator/internal/runner"
)

func TestProviderBuildsSafeHeadlessCommandAndReturnsStructuredOutput(t *testing.T) {
	t.Setenv("GITLAB_TOKEN", "must-not-reach-claude")
	workspaceRoot := t.TempDir()
	writeClaudeTestFile(t, workspaceRoot, "prompt.md", "Review the prepared repository.\n")
	writeClaudeTestFile(t, workspaceRoot, "result-schema.json", `{ "required": ["verdict"], "properties": { "verdict": { "type": "string" } }, "type": "object" }`)
	command := &fakeCommandRunner{lines: [][]byte{
		[]byte(`{"type":"system","subtype":"init","session_id":"session-1","model":"requested-model","permissionMode":"bypassPermissions","claude_code_version":"2.1.201","apiKeySource":"ANTHROPIC_API_KEY"}`),
		[]byte(`{"type":"assistant","message":{"model":"gateway-model","usage":{"input_tokens":120,"cache_read_input_tokens":80,"output_tokens":12}}}`),
		[]byte(`{"type":"result","subtype":"success","session_id":"session-1","is_error":false,"duration_ms":1450,"duration_api_ms":1200,"ttft_ms":320,"num_turns":2,"total_cost_usd":0.01,"usage":{"input_tokens":120,"cache_read_input_tokens":80,"output_tokens":12},"structured_output":{"verdict":"approved"}}`),
	}}
	configDirStatErr := error(nil)
	command.onRun = func(spec commandSpec) {
		dir := environmentMap(spec.Env)["CLAUDE_CONFIG_DIR"]
		if _, err := os.Stat(dir); err != nil {
			configDirStatErr = fmt.Errorf("stat %q: %w", dir, err)
		}
	}
	provider := newProvider("claude-test", command)
	var events []capturedProviderEvent
	result, err := provider.Execute(context.Background(), runner.ProviderRequest{
		ExecutionID: "run-claude", TaskID: "task-claude", Role: "ReviewerWorker",
		WorkspacePath: workspaceRoot, PromptFile: "prompt.md", SchemaFile: "result-schema.json",
		Provider: runner.ProviderConfig{
			Name: "claude-cli", Revision: 3, BaseURL: "https://gateway.example", APIKey: "api-secret",
			Headers: map[string]string{"X-Tenant": "example-tenant", "Authorization": "header-secret"},
			Model:   "model-alpha", Effort: "high",
		},
	}, func(eventType string, payload map[string]any) {
		events = append(events, capturedProviderEvent{eventType: eventType, payload: payload})
	})
	if err != nil {
		t.Fatalf("execute provider: %v", err)
	}
	if string(result) != `{"verdict":"approved"}` {
		t.Fatalf("unexpected structured output: %s", result)
	}

	wantArgs := []string{
		"-p", "--bare", "--no-session-persistence", "--output-format", "stream-json", "--verbose",
		"--dangerously-skip-permissions", "--json-schema", `{"properties":{"verdict":{"type":"string"}},"required":["verdict"],"type":"object"}`,
		"--model", "model-alpha", "--effort", "high",
	}
	if command.spec.Binary != "claude-test" || command.spec.Dir != workspaceRoot || command.spec.Stdin != "Review the prepared repository.\n" {
		t.Fatalf("unexpected command spec: %#v", command.spec)
	}
	if !reflect.DeepEqual(command.spec.Args, wantArgs) {
		t.Fatalf("unexpected args:\nwant %#v\n got %#v", wantArgs, command.spec.Args)
	}
	if strings.Contains(strings.Join(command.spec.Args, " "), "api-secret") || strings.Contains(strings.Join(command.spec.Args, " "), "header-secret") {
		t.Fatalf("secret leaked into arguments: %#v", command.spec.Args)
	}
	environment := environmentMap(command.spec.Env)
	if environment["ANTHROPIC_API_KEY"] != "api-secret" || environment["ANTHROPIC_BASE_URL"] != "https://gateway.example" {
		t.Fatalf("provider credentials missing from child environment: %#v", environment)
	}
	if environment["ANTHROPIC_CUSTOM_HEADERS"] != "Authorization: header-secret\nX-Tenant: example-tenant" {
		t.Fatalf("custom headers are not deterministic: %q", environment["ANTHROPIC_CUSTOM_HEADERS"])
	}
	if _, exists := environment["GITLAB_TOKEN"]; exists {
		t.Fatalf("unapproved runner environment leaked into Claude: %#v", environment)
	}
	configDir := environment["CLAUDE_CONFIG_DIR"]
	if configDir == "" {
		t.Fatalf("claude config dir is not isolated: %#v", environment)
	}
	if configDirStatErr != nil {
		t.Fatalf("CLAUDE_CONFIG_DIR does not point at a prepared directory when the child starts: %v", configDirStatErr)
	}
	for _, home := range []string{os.Getenv("HOME"), os.Getenv("USERPROFILE")} {
		if home != "" && strings.Contains(configDir, home) {
			t.Fatalf("CLAUDE_CONFIG_DIR must not live under the real home: %q", configDir)
		}
	}
	if environment["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"] != "1" {
		t.Fatalf("claude nonessential traffic must be disabled: %#v", environment)
	}
	if len(events) != 3 || events[0].eventType != "claude_session" || events[1].eventType != "claude_usage" || events[2].eventType != "claude_result" {
		t.Fatalf("unexpected normalized events: %#v", events)
	}
	for _, event := range events {
		eventJSON, err := json.Marshal(event.payload)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(eventJSON), "api-secret") || strings.Contains(string(eventJSON), "header-secret") || strings.Contains(string(eventJSON), "ANTHROPIC_API_KEY") {
			t.Fatalf("secret or auth source leaked into event: %s", eventJSON)
		}
	}
	if events[1].payload["model"] != "gateway-model" || events[2].payload["duration_ms"] != float64(1450) {
		t.Fatalf("important telemetry was not preserved: %#v", events)
	}
}

func TestProviderRejectsInvalidStreamOrTerminalResult(t *testing.T) {
	tests := []struct {
		name        string
		lines       [][]byte
		want        string
		diagnostics string
	}{
		{name: "malformed jsonl", lines: [][]byte{[]byte(`{"type":`)}, want: "decode claude stream event"},
		{name: "missing result", lines: [][]byte{[]byte(`{"type":"system","subtype":"init"}`)}, want: "terminal result"},
		{name: "missing structured output", lines: [][]byte{[]byte(`{"type":"result","is_error":false,"result":"free text with sk-ant-secret-123"}`)}, want: "structured_output", diagnostics: "free text with [REDACTED]"},
		{name: "reported error", lines: [][]byte{[]byte(`{"type":"result","is_error":true,"subtype":"error_max_turns","result":"stopped"}`)}, want: "error_max_turns"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeClaudeTestFile(t, root, "prompt.md", "review")
			writeClaudeTestFile(t, root, "schema.json", `{"type":"object"}`)
			provider := newProvider("claude", &fakeCommandRunner{lines: test.lines})
			_, err := provider.Execute(context.Background(), runner.ProviderRequest{
				ExecutionID: "run-invalid", TaskID: "task-invalid", Role: "ReviewerWorker", WorkspacePath: root,
				PromptFile: "prompt.md", SchemaFile: "schema.json",
				Provider: runner.ProviderConfig{Name: "claude-cli", APIKey: "sk-ant-secret-123"},
			}, func(string, map[string]any) {})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
			if test.diagnostics == "" {
				if _, statErr := os.Stat(filepath.Join(root, ".agent-runner-diagnostics")); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("diagnostics must only be persisted for missing structured output: %v", statErr)
				}
				return
			}
			raw, readErr := os.ReadFile(filepath.Join(root, ".agent-runner-diagnostics", "claude-result.json"))
			if readErr != nil {
				t.Fatalf("expected diagnostics to be persisted: %v", readErr)
			}
			if !strings.Contains(string(raw), test.diagnostics) {
				t.Fatalf("diagnostics must carry redacted terminal detail: %s", raw)
			}
			if strings.Contains(string(raw), "sk-ant-secret-123") {
				t.Fatalf("diagnostics leaked secrets: %s", raw)
			}
			if !strings.Contains(err.Error(), "diagnostics saved to") {
				t.Fatalf("error must reference the diagnostics directory: %v", err)
			}
		})
	}
}

func TestProviderRejectsUnsafeTaskFilesAndHeaders(t *testing.T) {
	root := t.TempDir()
	writeClaudeTestFile(t, root, "prompt.md", "review")
	writeClaudeTestFile(t, root, "schema.json", `{"type":"object"}`)
	provider := newProvider("claude", &fakeCommandRunner{})
	tests := []struct {
		name       string
		promptFile string
		headers    map[string]string
	}{
		{name: "prompt traversal", promptFile: "../prompt.md"},
		{name: "header name newline", promptFile: "prompt.md", headers: map[string]string{"X-Good\nX-Bad": "value"}},
		{name: "header value newline", promptFile: "prompt.md", headers: map[string]string{"X-Good": "value\nX-Bad: injected"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := provider.Execute(context.Background(), runner.ProviderRequest{
				ExecutionID: "run-unsafe", TaskID: "task-unsafe", Role: "ReviewerWorker", WorkspacePath: root,
				PromptFile: test.promptFile, SchemaFile: "schema.json",
				Provider: runner.ProviderConfig{Name: "claude-cli", Headers: test.headers},
			}, func(string, map[string]any) {})
			if err == nil {
				t.Fatal("expected unsafe provider request to fail")
			}
		})
	}
}

func TestProviderReturnsCommandFailure(t *testing.T) {
	root := t.TempDir()
	writeClaudeTestFile(t, root, "prompt.md", "review")
	writeClaudeTestFile(t, root, "schema.json", `{"type":"object"}`)
	provider := newProvider("claude", &fakeCommandRunner{err: errors.New("exit status 1: gateway unavailable")})
	_, err := provider.Execute(context.Background(), runner.ProviderRequest{
		ExecutionID: "run-command", TaskID: "task-command", Role: "ReviewerWorker", WorkspacePath: root,
		PromptFile: "prompt.md", SchemaFile: "schema.json", Provider: runner.ProviderConfig{Name: "claude-cli"},
	}, func(string, map[string]any) {})
	if err == nil || !strings.Contains(err.Error(), "gateway unavailable") {
		t.Fatalf("expected command error, got %v", err)
	}
}

func TestProviderSurfacesRedactedTerminalFailure(t *testing.T) {
	root := t.TempDir()
	writeClaudeTestFile(t, root, "prompt.md", "review")
	writeClaudeTestFile(t, root, "schema.json", `{"type":"object"}`)
	command := &fakeCommandRunner{lines: [][]byte{
		[]byte(`{"type":"system","subtype":"init","session_id":"session-1"}`),
		[]byte(`{"type":"result","subtype":"error_during_execution","is_error":true,"result":"request failed with key sk-ant-secret-123 at https://host"}`),
	}}
	provider := newProvider("claude", command)
	var events []capturedProviderEvent
	_, err := provider.Execute(context.Background(), runner.ProviderRequest{
		ExecutionID: "run-fail", TaskID: "task-fail", Role: "ReviewerWorker", WorkspacePath: root,
		PromptFile: "prompt.md", SchemaFile: "schema.json",
		Provider: runner.ProviderConfig{
			Name: "claude-cli", APIKey: "sk-ant-secret-123",
			Headers: map[string]string{"X-Tenant": "tenant-secret"},
		},
	}, func(eventType string, payload map[string]any) {
		events = append(events, capturedProviderEvent{eventType: eventType, payload: payload})
	})
	if err == nil || !strings.Contains(err.Error(), "error_during_execution") || !strings.Contains(err.Error(), "request failed with key [REDACTED] at https://host") {
		t.Fatalf("terminal error must carry redacted detail: %v", err)
	}
	if strings.Contains(err.Error(), "sk-ant-secret-123") || strings.Contains(err.Error(), "tenant-secret") {
		t.Fatalf("terminal error leaked secrets: %v", err)
	}
	if len(events) != 3 || events[2].eventType != "claude_error" {
		t.Fatalf("expected claude_error event: %#v", events)
	}
	if events[2].payload["subtype"] != "error_during_execution" || events[2].payload["message"] != "request failed with key [REDACTED] at https://host" {
		t.Fatalf("unexpected claude_error payload: %#v", events[2].payload)
	}
}

func TestProviderRedactsProviderError(t *testing.T) {
	config := runner.ProviderConfig{
		APIKey:  "short",
		Headers: map[string]string{"X-Long": "a-very-long-secret-value"},
	}
	if message := redactProviderError("short and a-very-long-secret-value", config, nil); message != "[REDACTED] and [REDACTED]" {
		t.Fatalf("unexpected redaction: %q", message)
	}
	if message := redactProviderError("clean failure", config, nil); message != "clean failure" {
		t.Fatalf("unexpected passthrough: %q", message)
	}
}

func TestClaudeEnvironmentInjectsRuntimeBehaviorGuards(t *testing.T) {
	configDir := t.TempDir()
	config := runner.ProviderConfig{APIKey: "api-secret", BaseURL: "https://gateway.example"}
	environment, err := claudeEnvironment(config, configDir)
	if err != nil {
		t.Fatalf("claude environment: %v", err)
	}
	for _, want := range []string{
		"CLAUDE_CONFIG_DIR=" + configDir,
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
		"ANTHROPIC_API_KEY=api-secret",
		"ANTHROPIC_BASE_URL=https://gateway.example",
		"CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS=0",
		"CLAUDE_CODE_DISABLE_UNKNOWN_MODEL_WINDOW_ENFORCEMENT=1",
	} {
		if !environmentContains(environment, want) {
			t.Fatalf("claude environment missing %q: %#v", want, environment)
		}
	}
}

func TestClaudeEnvironmentGuardInjectionIsTheOnlyEntry(t *testing.T) {
	t.Setenv("CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS", "120000")
	t.Setenv("CLAUDE_CODE_DISABLE_UNKNOWN_MODEL_WINDOW_ENFORCEMENT", "0")
	environment, err := claudeEnvironment(runner.ProviderConfig{}, t.TempDir())
	if err != nil {
		t.Fatalf("claude environment: %v", err)
	}
	injected := map[string]string{
		"CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS":                   "CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS=0",
		"CLAUDE_CODE_DISABLE_UNKNOWN_MODEL_WINDOW_ENFORCEMENT": "CLAUDE_CODE_DISABLE_UNKNOWN_MODEL_WINDOW_ENFORCEMENT=1",
	}
	for name, want := range injected {
		entries := environmentEntries(environment, name)
		// The guarantee is allowedBaseEnvironment's closed allowlist: ambient
		// CLAUDE_CODE_* entries cannot survive into the child env, so the slice
		// holds exactly the injected entry. No dedup layer may be relied upon:
		// os/exec keeps only the last duplicate while libc getenv reads the
		// first, so a leaked duplicate base entry would be a real bug.
		if len(entries) != 1 || entries[0] != want {
			t.Fatalf("injected %s must be the only entry for its name, got %d entries: %#v", name, len(entries), entries)
		}
	}
}

type fakeCommandRunner struct {
	spec  commandSpec
	lines [][]byte
	err   error
	onRun func(spec commandSpec)
}

func (f *fakeCommandRunner) Run(_ context.Context, spec commandSpec, onLine func([]byte) error) error {
	f.spec = spec
	if f.onRun != nil {
		f.onRun(spec)
	}
	for _, line := range f.lines {
		if err := onLine(line); err != nil {
			return err
		}
	}
	return f.err
}

type capturedProviderEvent struct {
	eventType string
	payload   map[string]any
}

func writeClaudeTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func environmentMap(values []string) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		name, content, ok := strings.Cut(value, "=")
		if ok {
			result[name] = content
		}
	}
	return result
}

func environmentContains(values []string, entry string) bool {
	for _, value := range values {
		if value == entry {
			return true
		}
	}
	return false
}

func environmentEntries(values []string, name string) []string {
	entries := make([]string, 0, 1)
	for _, value := range values {
		if entryName, _, cut := strings.Cut(value, "="); cut && entryName == name {
			entries = append(entries, value)
		}
	}
	return entries
}
