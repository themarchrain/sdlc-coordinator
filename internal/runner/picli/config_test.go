package picli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sdlc-coordinator/internal/runner"
)

func TestNormalizeGatewayBaseURL(t *testing.T) {
	tests := []struct {
		raw  string
		api  string
		want string
	}{
		{raw: "https://gw.example/v1/", api: "openai-completions", want: "https://gw.example/v1"},
		{raw: "https://gw.example/v1", api: "anthropic-messages", want: "https://gw.example"},
		{raw: "https://gw.example/anthropic/v1/", api: "anthropic-messages", want: "https://gw.example/anthropic"},
		{raw: "https://gw.example/v1", api: "openai-responses", want: "https://gw.example/v1"},
		{raw: "  https://gw.example/v1//  ", api: "", want: "https://gw.example/v1"},
	}
	for _, test := range tests {
		if got := normalizeGatewayBaseURL(test.raw, test.api); got != test.want {
			t.Errorf("normalizeGatewayBaseURL(%q, %q) = %q, want %q", test.raw, test.api, got, test.want)
		}
	}
}

func TestWriteAgentDirWritesModelsJSON(t *testing.T) {
	root := t.TempDir()
	provider := runner.ProviderConfig{
		Name: "pi-cli", API: "openai-completions", BaseURL: "https://gw.example/v1",
		APIKey: "sk-test", Model: "deepseek-v4-flash", Effort: "high",
		Headers: map[string]string{"X-Tenant": "acme"},
	}
	agentDir, environment, err := writeAgentDir(root, provider, nil)
	if err != nil {
		t.Fatalf("write agent dir: %v", err)
	}
	if agentDir != filepath.Join(root, "pi-agent") {
		t.Fatalf("agent dir = %q", agentDir)
	}
	models := readJSONFile(t, filepath.Join(agentDir, "models.json"))
	providers := models["providers"].(map[string]any)
	coordinator := providers[providerID].(map[string]any)
	if coordinator["baseUrl"] != "https://gw.example/v1" || coordinator["api"] != "openai-completions" {
		t.Fatalf("unexpected provider wire config: %#v", coordinator)
	}
	if coordinator["apiKey"] != "$PI_API_KEY" {
		t.Fatalf("api key must be interpolated from the environment: %#v", coordinator)
	}
	headers := coordinator["headers"].(map[string]any)
	if headers["X-Tenant"] != "$PI_HEADER_000" {
		t.Fatalf("custom headers must resolve from the environment: %#v", headers)
	}
	modelsList := coordinator["models"].([]any)
	entry := modelsList[0].(map[string]any)
	if entry["id"] != "deepseek-v4-flash" || entry["name"] != "deepseek-v4-flash" {
		t.Fatalf("unexpected model identity: %#v", entry)
	}
	if entry["reasoning"] != true || entry["contextWindow"] != float64(128000) || entry["maxTokens"] != float64(16384) {
		t.Fatalf("unexpected model capabilities: %#v", entry)
	}
	cost := entry["cost"].(map[string]any)
	for _, name := range []string{"input", "output", "cacheRead", "cacheWrite"} {
		if cost[name] != float64(0) {
			t.Fatalf("cost.%s must be zero: %#v", name, cost)
		}
	}
	env := environmentMap(environment)
	if env["PI_API_KEY"] != "sk-test" || env["PI_HEADER_000"] != "acme" {
		t.Fatalf("provider credentials missing from env: %#v", env)
	}
	if env["PI_CODING_AGENT_DIR"] != agentDir {
		t.Fatalf("PI_CODING_AGENT_DIR must point at the generated agent dir: %#v", env)
	}
	assertFileMode(t, filepath.Join(agentDir, "models.json"), 0o600)
	assertFileMode(t, filepath.Join(agentDir, "settings.json"), 0o600)
}

func TestWriteAgentDirDefaultsAPIWhenEmpty(t *testing.T) {
	agentDir, _, err := writeAgentDir(t.TempDir(), runner.ProviderConfig{BaseURL: "https://gw.example/v1", Model: "m"}, nil)
	if err != nil {
		t.Fatalf("write agent dir: %v", err)
	}
	models := readJSONFile(t, filepath.Join(agentDir, "models.json"))
	coordinator := models["providers"].(map[string]any)[providerID].(map[string]any)
	if coordinator["api"] != "openai-completions" {
		t.Fatalf("empty api must default to openai-completions: %#v", coordinator)
	}
}

func TestWriteAgentDirRequiresBaseURLAndModel(t *testing.T) {
	tests := []struct {
		name     string
		provider runner.ProviderConfig
		want     string
	}{
		{name: "missing base url", provider: runner.ProviderConfig{Model: "m"}, want: "pi provider base url is required"},
		{name: "missing model", provider: runner.ProviderConfig{BaseURL: "https://gw.example/v1"}, want: "pi provider model is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := writeAgentDir(t.TempDir(), test.provider, nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestWriteAgentDirRejectsUnsafeHeaderName(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
	}{
		{name: "invalid name", headers: map[string]string{"X Bad": "value"}},
		{name: "newline in value", headers: map[string]string{"X-Good": "value\r\nX-Bad: injected"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := writeAgentDir(t.TempDir(), runner.ProviderConfig{
				BaseURL: "https://gw.example/v1", Model: "m", Headers: test.headers,
			}, nil)
			if err == nil {
				t.Fatal("expected unsafe header to be rejected")
			}
		})
	}
}

func TestWriteAgentDirWritesMCPJSONOnlyWithChannel(t *testing.T) {
	root := t.TempDir()
	agentDir, environment, err := writeAgentDir(root, runner.ProviderConfig{BaseURL: "https://gw.example/v1", Model: "m"}, nil)
	if err != nil {
		t.Fatalf("write agent dir: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(agentDir, "mcp.json")); !os.IsNotExist(statErr) {
		t.Fatalf("mcp.json must not exist without a result channel: %v", statErr)
	}
	if _, exists := environmentMap(environment)["PI_MCP_TOKEN"]; exists {
		t.Fatal("run token must not be injected without a result channel")
	}

	root = t.TempDir()
	channel := &runner.ResultSubmissionChannel{EndpointURL: "http://coordinator/mcp", BearerToken: "run-token"}
	agentDir, environment, err = writeAgentDir(root, runner.ProviderConfig{BaseURL: "https://gw.example/v1", Model: "m"}, channel)
	if err != nil {
		t.Fatalf("write agent dir: %v", err)
	}
	mcp := readJSONFile(t, filepath.Join(agentDir, "mcp.json"))
	coordinator := mcp["mcpServers"].(map[string]any)[mcpServerName].(map[string]any)
	if coordinator["url"] != "http://coordinator/mcp" || coordinator["exposure"] != "direct" {
		t.Fatalf("unexpected mcp server config: %#v", coordinator)
	}
	if coordinator["timeout"] != float64(mcpTimeoutSeconds) || mcpTimeoutSeconds != 600 {
		t.Fatalf("mcp timeout must be %d seconds: %#v", mcpTimeoutSeconds, coordinator)
	}
	if coordinator["description"] != "Coordinator result channel" {
		t.Fatalf("unexpected mcp description: %#v", coordinator)
	}
	headers := coordinator["headers"].(map[string]any)
	if headers["Authorization"] != "Bearer ${PI_MCP_TOKEN}" {
		t.Fatalf("mcp token must be interpolated from the environment: %#v", headers)
	}
	if environmentMap(environment)["PI_MCP_TOKEN"] != "run-token" {
		t.Fatalf("run token missing from env: %#v", environment)
	}
	assertFileMode(t, filepath.Join(agentDir, "mcp.json"), 0o600)
	if raw, readErr := os.ReadFile(filepath.Join(agentDir, "mcp.json")); readErr != nil || strings.Contains(string(raw), "run-token") {
		t.Fatalf("run token must not be written to mcp.json: %s (%v)", raw, readErr)
	}
}

func TestWriteAgentDirWritesRetryDisabled(t *testing.T) {
	agentDir, _, err := writeAgentDir(t.TempDir(), runner.ProviderConfig{BaseURL: "https://gw.example/v1", Model: "m"}, nil)
	if err != nil {
		t.Fatalf("write agent dir: %v", err)
	}
	settings := readJSONFile(t, filepath.Join(agentDir, "settings.json"))
	retry := settings["retry"].(map[string]any)
	if retry["enabled"] != false {
		t.Fatalf("agent-level retry must be disabled so the coordinator owns retries: %#v", retry)
	}
}

func readJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return value
}

func environmentMap(values []string) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		if name, content, ok := strings.Cut(value, "="); ok {
			result[name] = content
		}
	}
	return result
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o, want %o", path, got, want)
	}
}
