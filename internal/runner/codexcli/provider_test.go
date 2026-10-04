package codexcli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/runner"
)

func TestProviderBuildsIsolatedResponsesCommandAndReturnsStructuredOutput(t *testing.T) {
	t.Setenv("CODEX_HOME", `C:\Users\Administrator\.codex`)
	t.Setenv("GITLAB_TOKEN", "must-not-reach-codex")
	workspaceRoot := t.TempDir()
	writeCodexTestFile(t, workspaceRoot, "prompt.md", "Review the prepared repository.\n")
	writeCodexTestFile(t, workspaceRoot, "result-schema.json", `{"type":"object","required":["verdict"],"properties":{"verdict":{"type":"string"}}}`)
	command := &fakeCommandRunner{lines: [][]byte{
		[]byte(`{"type":"thread.started","thread_id":"thread-1"}`),
		[]byte(`{"type":"turn.completed","usage":{"input_tokens":120,"cached_input_tokens":80,"output_tokens":12}}`),
	}}
	provider := newProvider("codex-test", command)
	var events []capturedProviderEvent
	result, err := provider.Execute(context.Background(), runner.ProviderRequest{
		ExecutionID: "run-codex", TaskID: "task-codex", Role: "ReviewerWorker",
		WorkspacePath: workspaceRoot, PromptFile: "prompt.md", SchemaFile: "result-schema.json",
		Provider: runner.ProviderConfig{
			Name: "codex-cli", Revision: 3, BaseURL: "https://gateway.example/v1", APIKey: "api-secret",
			Headers: map[string]string{"X-User-Id": "10001", "X-Product-Name": "product-secret", "Authorization": "header-secret"},
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

	if command.spec.Binary != "codex-test" || command.spec.Dir != workspaceRoot || command.spec.Stdin != "Review the prepared repository.\n" {
		t.Fatalf("unexpected command spec: %#v", command.spec)
	}
	for _, required := range []string{
		"exec", "--json", "--ephemeral", "--ignore-user-config", "--ignore-rules",
		"--dangerously-bypass-approvals-and-sandbox", "--skip-git-repo-check",
		"--output-last-message", "-C", workspaceRoot, "-m", "model-alpha",
		`model_provider="coordinator_provider"`, `model_providers.coordinator_provider.wire_api="responses"`,
		`model_reasoning_effort="high"`,
	} {
		if !containsArgument(command.spec.Args, required) {
			t.Fatalf("missing argument %q in %#v", required, command.spec.Args)
		}
	}
	// codex 0.148.0 只接受 stdin 占位符 "-" 出现在全部选项之后（回归锁定）。
	if last := command.spec.Args[len(command.spec.Args)-1]; last != "-" {
		t.Fatalf("stdin placeholder must be the final argument, got %#v", command.spec.Args)
	}
	if containsArgument(command.spec.Args[:2], "-") {
		t.Fatalf("stdin placeholder must not precede options: %#v", command.spec.Args)
	}
	if containsArgument(command.spec.Args, "--output-schema") {
		t.Fatalf("Codex must not send --output-schema: %#v", command.spec.Args)
	}
	if !containsArgumentPair(command.spec.Args, "--color", "never") {
		t.Fatalf("--color never was not applied: %#v", command.spec.Args)
	}
	for _, feature := range []string{"plugins", "recommended_plugins", "remote_plugin", "plugin_sharing"} {
		if !containsArgumentPair(command.spec.Args, "--disable", feature) {
			t.Fatalf("Codex plugin feature %q was not disabled: %#v", feature, command.spec.Args)
		}
	}
	for _, setting := range []string{`agents.enabled=false`, `web_search="disabled"`} {
		if !containsArgumentPair(command.spec.Args, "-c", setting) {
			t.Fatalf("Codex runtime setting %q was not applied: %#v", setting, command.spec.Args)
		}
	}
	joinedArgs := strings.Join(command.spec.Args, " ")
	for _, secret := range []string{"api-secret", "header-secret", "10001", "product-secret"} {
		if strings.Contains(joinedArgs, secret) {
			t.Fatalf("secret/header value %q leaked into arguments: %#v", secret, command.spec.Args)
		}
	}
	environment := environmentMap(command.spec.Env)
	if environment["CODEX_PROVIDER_API_KEY"] != "api-secret" {
		t.Fatalf("API key missing from child environment: %#v", environment)
	}
	if environment["CODEX_HOME"] == "" || environment["CODEX_HOME"] == `C:\Users\Administrator\.codex` {
		t.Fatalf("Codex home was not isolated: %#v", environment)
	}
	if _, exists := environment["GITLAB_TOKEN"]; exists {
		t.Fatalf("unapproved environment leaked into Codex: %#v", environment)
	}
	assertHeaderEnvironment(t, command.spec.Args, environment, "X-User-Id", "10001")
	assertHeaderEnvironment(t, command.spec.Args, environment, "X-Product-Name", "product-secret")
	assertHeaderEnvironment(t, command.spec.Args, environment, "Authorization", "header-secret")
	if len(events) != 2 || events[0].eventType != "codex_session" || events[1].eventType != "codex_usage" {
		t.Fatalf("unexpected normalized events: %#v", events)
	}
	if events[0].payload["thread_id"] != "thread-1" || events[1].payload["input_tokens"] != float64(120) {
		t.Fatalf("important telemetry was not preserved: %#v", events)
	}
	for _, event := range events {
		raw, _ := json.Marshal(event.payload)
		if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "10001") {
			t.Fatalf("secret leaked into event: %s", raw)
		}
	}
}

func TestProviderAdaptsOptionalResponseSchemaWithoutChangingWorkspaceContract(t *testing.T) {
	root := t.TempDir()
	writeCodexTestFile(t, root, "prompt.md", "return a result")
	original := `{
		"type":"object",
		"required":["task_id"],
		"properties":{
			"task_id":{"type":"string"},
			"blocker_reason":{"type":["string","null"]},
			"details":{"type":"object","required":["message"],"properties":{"message":{"type":"string"},"line":{"type":"integer"}},"additionalProperties":false}
		},
		"additionalProperties":false
	}`
	writeCodexTestFile(t, root, "result-schema.json", original)
	command := &fakeCommandRunner{final: `{"task_id":"task-1","blocker_reason":null,"details":null}`}

	provider := newProvider("codex", command)
	if _, err := provider.Execute(context.Background(), runner.ProviderRequest{
		ExecutionID: "run-schema", TaskID: "task-schema", Role: "DeveloperWorker", WorkspacePath: root,
		PromptFile: "prompt.md", SchemaFile: "result-schema.json", Provider: runner.ProviderConfig{Name: "codex-cli"},
	}, nil); err != nil {
		t.Fatalf("execute provider: %v", err)
	}

	if containsArgument(command.spec.Args, "--output-schema") {
		t.Fatalf("Codex must not send --output-schema: %#v", command.spec.Args)
	}
	if got, err := os.ReadFile(filepath.Join(root, "result-schema.json")); err != nil || string(got) != original {
		t.Fatalf("workspace contract schema changed: content=%s err=%v", got, err)
	}
}

func TestProviderParsesNestedTurnFailureAndRedactsSecrets(t *testing.T) {
	root := t.TempDir()
	writeCodexTestFile(t, root, "prompt.md", "review")
	writeCodexTestFile(t, root, "schema.json", `{"type":"object","properties":{},"required":[],"additionalProperties":false}`)
	command := &fakeCommandRunner{
		lines: [][]byte{[]byte(`{"type":"turn.failed","error":{"code":"invalid_request_error","message":"request rejected for api-secret and tenant-secret"}}`)},
		err:   errors.New("exit status 1"),
	}
	provider := newProvider("codex", command)
	var events []capturedProviderEvent
	_, err := provider.Execute(context.Background(), runner.ProviderRequest{
		ExecutionID: "run-failed", TaskID: "task-failed", Role: "DeveloperWorker", WorkspacePath: root,
		PromptFile: "prompt.md", SchemaFile: "schema.json",
		Provider: runner.ProviderConfig{Name: "codex-cli", APIKey: "api-secret", Headers: map[string]string{"X-Tenant": "tenant-secret"}},
	}, func(eventType string, payload map[string]any) {
		events = append(events, capturedProviderEvent{eventType: eventType, payload: payload})
	})
	if err == nil {
		t.Fatal("expected command failure")
	}
	if !strings.Contains(err.Error(), "invalid_request_error") || !strings.Contains(err.Error(), "request rejected") {
		t.Fatalf("provider failure did not preserve normalized turn error: %v", err)
	}
	if len(events) != 1 || events[0].eventType != "codex_error" {
		t.Fatalf("unexpected error events: %#v", events)
	}
	payload := events[0].payload
	if payload["error_code"] != "invalid_request_error" {
		t.Fatalf("nested error code was not preserved: %#v", payload)
	}
	message, _ := payload["message"].(string)
	if !strings.Contains(message, "[REDACTED]") || strings.Contains(message, "api-secret") || strings.Contains(message, "tenant-secret") {
		t.Fatalf("nested error message was not safely preserved: %#v", payload)
	}
}

func TestProviderAcceptsSingleMarkdownFencedJSONResult(t *testing.T) {
	root := t.TempDir()
	writeCodexTestFile(t, root, "prompt.md", "return a result")
	writeCodexTestFile(t, root, "schema.json", `{"type":"object","required":["outcome"],"properties":{"outcome":{"type":"string"}},"additionalProperties":false}`)
	provider := newProvider("codex", &fakeCommandRunner{final: "```json\n{\"outcome\":\"change_ready\"}\n```\n"})

	result, err := provider.Execute(context.Background(), runner.ProviderRequest{
		ExecutionID: "run-fenced", TaskID: "task-fenced", Role: "DeveloperWorker", WorkspacePath: root,
		PromptFile: "prompt.md", SchemaFile: "schema.json", Provider: runner.ProviderConfig{Name: "codex-cli"},
	}, nil)
	if err != nil {
		t.Fatalf("fenced JSON result should be recovered: %v", err)
	}
	if string(result) != `{"outcome":"change_ready"}` {
		t.Fatalf("unexpected normalized fenced result: %s", result)
	}
}

func TestProviderAcceptsTerminalMarkdownFencedJSONAfterSummary(t *testing.T) {
	root := t.TempDir()
	writeCodexTestFile(t, root, "prompt.md", "return a result")
	writeCodexTestFile(t, root, "schema.json", `{"type":"object","required":["outcome"],"properties":{"outcome":{"type":"string"}},"additionalProperties":false}`)
	provider := newProvider("codex", &fakeCommandRunner{final: "完成改动并通过验证。\n\n```json\n{\"outcome\":\"change_ready\"}\n```\n"})

	result, err := provider.Execute(context.Background(), runner.ProviderRequest{
		ExecutionID: "run-fenced-summary", TaskID: "task-fenced-summary", Role: "DeveloperWorker", WorkspacePath: root,
		PromptFile: "prompt.md", SchemaFile: "schema.json", Provider: runner.ProviderConfig{Name: "codex-cli"},
	}, nil)
	if err != nil {
		t.Fatalf("terminal fenced JSON after summary should be recovered: %v", err)
	}
	if string(result) != `{"outcome":"change_ready"}` {
		t.Fatalf("unexpected normalized fenced result: %s", result)
	}
}

func TestProviderEmitsCodexProgressEvents(t *testing.T) {
	root := t.TempDir()
	writeCodexTestFile(t, root, "prompt.md", "review")
	writeCodexTestFile(t, root, "schema.json", `{"type":"object","properties":{},"required":[],"additionalProperties":false}`)
	command := &fakeCommandRunner{
		lines: [][]byte{
			[]byte(`{"type":"turn.started","turn_id":"turn-1"}`),
			[]byte(`{"type":"item.completed","item":{"id":"item-1","type":"command_execution","status":"completed"}}`),
		},
		final: `{}`,
	}
	provider := newProvider("codex", command)
	var events []capturedProviderEvent
	if _, err := provider.Execute(context.Background(), runner.ProviderRequest{
		ExecutionID: "run-progress", TaskID: "task-progress", Role: "DeveloperWorker", WorkspacePath: root,
		PromptFile: "prompt.md", SchemaFile: "schema.json", Provider: runner.ProviderConfig{Name: "codex-cli"},
	}, func(eventType string, payload map[string]any) {
		events = append(events, capturedProviderEvent{eventType: eventType, payload: payload})
	}); err != nil {
		t.Fatalf("execute provider: %v", err)
	}
	if len(events) != 2 || events[0].eventType != "codex_turn" || events[1].eventType != "codex_progress" {
		t.Fatalf("unexpected progress events: %#v", events)
	}
	if events[0].payload["turn_id"] != "turn-1" || events[1].payload["item_type"] != "command_execution" || events[1].payload["status"] != "completed" {
		t.Fatalf("unexpected progress payloads: %#v", events)
	}
}

func TestProviderKeepsCodexErrorItemMessage(t *testing.T) {
	root := t.TempDir()
	writeCodexTestFile(t, root, "prompt.md", "review")
	writeCodexTestFile(t, root, "schema.json", `{"type":"object","properties":{},"required":[],"additionalProperties":false}`)
	command := &fakeCommandRunner{
		lines: [][]byte{
			[]byte(`{"type":"item.completed","item":{"id":"item_0","type":"error","status":"completed","error":{"message":"We're currently experiencing high demand, which may cause temporary errors."}}}`),
		},
		final: `{}`,
	}
	provider := newProvider("codex", command)
	var events []capturedProviderEvent
	if _, err := provider.Execute(context.Background(), runner.ProviderRequest{
		ExecutionID: "run-error-item", TaskID: "task-error-item", Role: "DeveloperWorker", WorkspacePath: root,
		PromptFile: "prompt.md", SchemaFile: "schema.json", Provider: runner.ProviderConfig{Name: "codex-cli"},
	}, func(eventType string, payload map[string]any) {
		events = append(events, capturedProviderEvent{eventType: eventType, payload: payload})
	}); err != nil {
		t.Fatalf("execute provider: %v", err)
	}
	if len(events) != 1 || events[0].eventType != "codex_progress" {
		t.Fatalf("unexpected error item events: %#v", events)
	}
	if events[0].payload["item_type"] != "error" || events[0].payload["message"] != "We're currently experiencing high demand, which may cause temporary errors." {
		t.Fatalf("error item message was dropped: %#v", events[0].payload)
	}
}

func TestProviderPersistsInvalidFinalOutputAndAdaptedSchema(t *testing.T) {
	root := t.TempDir()
	writeCodexTestFile(t, root, "prompt.md", "return a result")
	writeCodexTestFile(t, root, "schema.json", `{"type":"object","required":["outcome"],"properties":{"outcome":{"type":"string"}},"additionalProperties":false}`)
	provider := newProvider("codex", &fakeCommandRunner{final: "result follows: {not-json}"})

	_, err := provider.Execute(context.Background(), runner.ProviderRequest{
		ExecutionID: "run-invalid-final", TaskID: "task-invalid-final", Role: "DeveloperWorker", WorkspacePath: root,
		PromptFile: "prompt.md", SchemaFile: "schema.json", Provider: runner.ProviderConfig{Name: "codex-cli"},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), ".agent-runner-diagnostics") {
		t.Fatalf("invalid result should report durable diagnostics path: %v", err)
	}
	diagnosticsRoot := filepath.Join(root, ".agent-runner-diagnostics")
	finalOutput, readErr := os.ReadFile(filepath.Join(diagnosticsRoot, "codex-final-output.txt"))
	if readErr != nil || string(finalOutput) != "result follows: {not-json}" {
		t.Fatalf("invalid final output was not preserved: content=%q err=%v", finalOutput, readErr)
	}
	adaptedSchema, readErr := os.ReadFile(filepath.Join(diagnosticsRoot, "codex-result-schema.json"))
	if readErr != nil || !json.Valid(adaptedSchema) {
		t.Fatalf("adapted schema was not preserved: content=%q err=%v", adaptedSchema, readErr)
	}
}

func TestProviderParsesErrorJSONEmbeddedInTurnFailureMessage(t *testing.T) {
	root := t.TempDir()
	writeCodexTestFile(t, root, "prompt.md", "review")
	writeCodexTestFile(t, root, "schema.json", `{"type":"object","properties":{},"required":[],"additionalProperties":false}`)
	command := &fakeCommandRunner{
		lines: [][]byte{[]byte(`{"type":"turn.failed","error":{"message":"{\"error\":{\"message\":\"Required properties must match all properties\",\"type\":\"invalid_request_error\",\"code\":\"invalid_request_error\"}}"}}`)},
		err:   errors.New("exit status 1"),
	}
	provider := newProvider("codex", command)
	var events []capturedProviderEvent
	_, err := provider.Execute(context.Background(), runner.ProviderRequest{
		ExecutionID: "run-embedded", TaskID: "task-embedded", Role: "DeveloperWorker", WorkspacePath: root,
		PromptFile: "prompt.md", SchemaFile: "schema.json", Provider: runner.ProviderConfig{Name: "codex-cli"},
	}, func(eventType string, payload map[string]any) {
		events = append(events, capturedProviderEvent{eventType: eventType, payload: payload})
	})
	if err == nil || !strings.Contains(err.Error(), "Required properties must match all properties") {
		t.Fatalf("embedded turn failure was not returned: %v", err)
	}
	if len(events) != 1 || events[0].payload["error_code"] != "invalid_request_error" || events[0].payload["message"] != "Required properties must match all properties" {
		t.Fatalf("embedded turn failure was not normalized: %#v", events)
	}
}

func TestResponsesSchemaAdapterSupportsActiveWorkerContracts(t *testing.T) {
	tests := []struct {
		name   string
		schema map[string]any
	}{
		{name: "developer", schema: results.NewDeveloperAgentContract().InputSchema()},
		{name: "reviewer", schema: results.NewReviewerContract().InputSchema()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, err := json.Marshal(test.schema)
			if err != nil {
				t.Fatal(err)
			}
			adapted, err := adaptResponsesSchema(raw)
			if err != nil {
				t.Fatalf("adapt schema: %v", err)
			}
			var schema map[string]any
			if err := json.Unmarshal(adapted, &schema); err != nil {
				t.Fatalf("decode adapted schema: %v", err)
			}
			assertAllObjectPropertiesRequired(t, schema)
		})
	}
}

func TestProviderRejectsUnsafeTaskFilesHeadersAndInvalidResults(t *testing.T) {
	root := t.TempDir()
	writeCodexTestFile(t, root, "prompt.md", "review")
	writeCodexTestFile(t, root, "schema.json", `{"type":"object"}`)
	tests := []struct {
		name       string
		promptFile string
		headers    map[string]string
		final      string
		lines      [][]byte
		want       string
	}{
		{name: "prompt traversal", promptFile: "../prompt.md", want: "safe workspace root entry"},
		{name: "header name newline", promptFile: "prompt.md", headers: map[string]string{"X-Good\nX-Bad": "value"}, want: "unsafe"},
		{name: "header value newline", promptFile: "prompt.md", headers: map[string]string{"X-Good": "value\nX-Bad: injected"}, want: "unsafe"},
		{name: "malformed JSONL", promptFile: "prompt.md", lines: [][]byte{[]byte(`{"type":`)}, final: `{"ok":true}`, want: "decode codex event"},
		{name: "missing final", promptFile: "prompt.md", want: "final result"},
		{name: "invalid final", promptFile: "prompt.md", final: "not-json", want: "invalid JSON"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := newProvider("codex", &fakeCommandRunner{lines: test.lines, final: test.final})
			_, err := provider.Execute(context.Background(), runner.ProviderRequest{
				ExecutionID: "run-unsafe", TaskID: "task-unsafe", Role: "ReviewerWorker", WorkspacePath: root,
				PromptFile: test.promptFile, SchemaFile: "schema.json", Provider: runner.ProviderConfig{Name: "codex-cli", Headers: test.headers},
			}, func(string, map[string]any) {})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestProviderIgnoresUnknownWellFormedEventsAndReturnsCommandFailure(t *testing.T) {
	root := t.TempDir()
	writeCodexTestFile(t, root, "prompt.md", "review")
	writeCodexTestFile(t, root, "schema.json", `{"type":"object"}`)
	provider := newProvider("codex", &fakeCommandRunner{lines: [][]byte{[]byte(`{"type":"future.event","secret":"do-not-emit"}`)}, final: `{"ok":true}`})
	var events []capturedProviderEvent
	result, err := provider.Execute(context.Background(), runner.ProviderRequest{
		ExecutionID: "run-unknown", TaskID: "task-unknown", Role: "ReviewerWorker", WorkspacePath: root,
		PromptFile: "prompt.md", SchemaFile: "schema.json", Provider: runner.ProviderConfig{Name: "codex-cli"},
	}, func(eventType string, payload map[string]any) {
		events = append(events, capturedProviderEvent{eventType, payload})
	})
	if err != nil || string(result) != `{"ok":true}` || len(events) != 0 {
		t.Fatalf("unknown event changed execution: result=%s events=%#v err=%v", result, events, err)
	}

	failing := newProvider("codex", &fakeCommandRunner{err: errors.New("exit status 1: gateway unavailable")})
	_, err = failing.Execute(context.Background(), runner.ProviderRequest{
		ExecutionID: "run-failure", TaskID: "task-failure", Role: "ReviewerWorker", WorkspacePath: root,
		PromptFile: "prompt.md", SchemaFile: "schema.json", Provider: runner.ProviderConfig{Name: "codex-cli"},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "gateway unavailable") {
		t.Fatalf("expected command failure, got %v", err)
	}
}

func TestProviderRedactsGatewaySecretsFromCommandFailure(t *testing.T) {
	root := t.TempDir()
	writeCodexTestFile(t, root, "prompt.md", "review")
	writeCodexTestFile(t, root, "schema.json", `{"type":"object"}`)
	provider := newProvider("codex", &fakeCommandRunner{err: errors.New("request rejected for api-secret and tenant-secret")})

	_, err := provider.Execute(context.Background(), runner.ProviderRequest{
		ExecutionID: "run-redacted", TaskID: "task-redacted", Role: "ReviewerWorker", WorkspacePath: root,
		PromptFile: "prompt.md", SchemaFile: "schema.json",
		Provider: runner.ProviderConfig{
			Name: "codex-cli", APIKey: "api-secret", Headers: map[string]string{"X-Tenant": "tenant-secret"},
		},
	}, nil)
	if err == nil {
		t.Fatal("expected command failure")
	}
	if strings.Contains(err.Error(), "api-secret") || strings.Contains(err.Error(), "tenant-secret") {
		t.Fatalf("gateway secret leaked through command failure: %v", err)
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("redacted failure lost useful diagnostic shape: %v", err)
	}
}

type fakeCommandRunner struct {
	spec    commandSpec
	lines   [][]byte
	final   string
	noFinal bool
	err     error
}

func (f *fakeCommandRunner) Run(_ context.Context, spec commandSpec, onLine func([]byte) error) error {
	f.spec = spec
	for _, line := range f.lines {
		if err := onLine(line); err != nil {
			return err
		}
	}
	if f.err != nil {
		return f.err
	}
	finalPath := argumentValue(spec.Args, "--output-last-message")
	if f.noFinal {
		return nil
	}
	if f.final == "" && len(f.lines) > 0 {
		f.final = `{"verdict":"approved"}`
	}
	if f.final != "" {
		if err := os.WriteFile(finalPath, []byte(f.final), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func assertStrictRequiredProperties(t *testing.T, schema map[string]any) {
	t.Helper()
	properties, _ := schema["properties"].(map[string]any)
	required, _ := schema["required"].([]any)
	if len(required) != len(properties) {
		t.Fatalf("required properties do not cover object: required=%#v properties=%#v", required, properties)
	}
	seen := make(map[string]bool, len(required))
	for _, name := range required {
		seen[name.(string)] = true
	}
	for name := range properties {
		if !seen[name] {
			t.Fatalf("property %q is not required: %#v", name, required)
		}
	}
}

func assertAllObjectPropertiesRequired(t *testing.T, schema map[string]any) {
	t.Helper()
	if variants, ok := schema["anyOf"].([]any); ok {
		for _, rawVariant := range variants {
			if variant, ok := rawVariant.(map[string]any); ok {
				assertAllObjectPropertiesRequired(t, variant)
			}
		}
		return
	}
	if _, ok := schema["properties"].(map[string]any); ok {
		assertStrictRequiredProperties(t, schema)
		for _, property := range schema["properties"].(map[string]any) {
			if child, ok := property.(map[string]any); ok {
				assertAllObjectPropertiesRequired(t, child)
			}
		}
	}
	if items, ok := schema["items"].(map[string]any); ok {
		assertAllObjectPropertiesRequired(t, items)
	}
}

type capturedProviderEvent struct {
	eventType string
	payload   map[string]any
}

func writeCodexTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func containsArgument(args []string, expected string) bool {
	for _, argument := range args {
		if argument == expected {
			return true
		}
	}
	return false
}

func containsArgumentPair(args []string, name, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name && args[index+1] == value {
			return true
		}
	}
	return false
}

func argumentValue(args []string, name string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name {
			return args[index+1]
		}
	}
	return ""
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

func assertHeaderEnvironment(t *testing.T, args []string, environment map[string]string, header, value string) {
	t.Helper()
	for index, argument := range args {
		if argument != "-c" || index+1 >= len(args) || !strings.Contains(args[index+1], "env_http_headers") || !strings.Contains(args[index+1], header) {
			continue
		}
		for name, got := range environment {
			if got == value && strings.Contains(args[index+1], name) {
				return
			}
		}
	}
	t.Fatalf("header %q was not mapped through an environment variable; args=%#v env=%#v", header, args, environment)
}

func TestProviderCommandArgumentsAreDeterministic(t *testing.T) {
	first, err := buildHeaderEnvironment(map[string]string{"X-B": "2", "X-A": "1"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := buildHeaderEnvironment(map[string]string{"X-A": "1", "X-B": "2"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("header mapping is not deterministic: %#v != %#v", first, second)
	}
}
