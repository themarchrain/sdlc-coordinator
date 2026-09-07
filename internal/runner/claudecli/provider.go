package claudecli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"sdlc-coordinator/internal/runner"
)

var headerNamePattern = regexp.MustCompile(`^[!#$%&'*+.^_` + "`" + `|~0-9A-Za-z-]+$`)

const diagnosticsDirectoryName = ".agent-runner-diagnostics"

type Provider struct {
	binary  string
	command commandRunner
}

func New(binary string) *Provider {
	return newProvider(binary, newProcessRunner())
}

func newProvider(binary string, command commandRunner) *Provider {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		binary = "claude"
	}
	return &Provider{binary: binary, command: command}
}

func (p *Provider) Execute(ctx context.Context, request runner.ProviderRequest, emit runner.EmitFunc) (json.RawMessage, error) {
	if p == nil || p.command == nil {
		return nil, errors.New("claude command runner is required")
	}
	prompt, err := readTaskFile(request.WorkspacePath, request.PromptFile)
	if err != nil {
		return nil, fmt.Errorf("read claude prompt: %w", err)
	}
	schemaRaw, err := readTaskFile(request.WorkspacePath, request.SchemaFile)
	if err != nil {
		return nil, fmt.Errorf("read claude result schema: %w", err)
	}
	schema, err := canonicalJSON(schemaRaw)
	if err != nil {
		return nil, fmt.Errorf("decode claude result schema: %w", err)
	}
	runtimeRoot, err := os.MkdirTemp("", "agent-runner-claude-")
	if err != nil {
		return nil, fmt.Errorf("create isolated claude config root: %w", err)
	}
	defer os.RemoveAll(runtimeRoot)
	configDir := filepath.Join(runtimeRoot, "claude-config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return nil, fmt.Errorf("create isolated claude config dir: %w", err)
	}
	environment, err := claudeEnvironment(request.Provider, configDir)
	if err != nil {
		return nil, err
	}
	args := []string{
		"-p", "--bare", "--no-session-persistence", "--output-format", "stream-json", "--verbose",
		"--dangerously-skip-permissions", "--json-schema", string(schema),
	}
	if request.ResultSubmission != nil {
		// claude 的 http MCP 头只接受字面值，token 写进 --mcp-config 文件；
		// 文件放进程临时目录（工作区之外），随 runtimeRoot 一起删除。
		mcpConfigPath, err := writeMCPConfig(runtimeRoot, request.ResultSubmission)
		if err != nil {
			return nil, err
		}
		args = append(args, "--mcp-config", mcpConfigPath, "--strict-mcp-config")
	}
	if model := strings.TrimSpace(request.Provider.Model); model != "" {
		args = append(args, "--model", model)
	}
	if effort := strings.TrimSpace(request.Provider.Effort); effort != "" {
		args = append(args, "--effort", effort)
	}
	parser := streamParser{emit: emit, provider: request.Provider, workspacePath: request.WorkspacePath, channel: request.ResultSubmission}
	err = p.command.Run(ctx, commandSpec{
		Binary: p.binary, Args: args, Dir: request.WorkspacePath, Env: environment, Stdin: string(prompt),
	}, parser.consume)
	if err != nil {
		return nil, fmt.Errorf("run claude cli: %w", err)
	}
	if parser.terminalError != nil {
		return nil, parser.terminalError
	}
	if len(parser.structuredOutput) == 0 {
		return nil, errors.New("claude stream ended without terminal result")
	}
	return append(json.RawMessage(nil), parser.structuredOutput...), nil
}

type streamParser struct {
	emit             runner.EmitFunc
	provider         runner.ProviderConfig
	channel          *runner.ResultSubmissionChannel
	workspacePath    string
	structuredOutput json.RawMessage
	terminalError    error
}

// writeMCPConfig materializes the coordinator MCP server definition for the
// claude CLI. The file sits in the provider's temp runtime root, never in the
// agent workspace, and is removed with the runtime root after the run.
func writeMCPConfig(runtimeRoot string, channel *runner.ResultSubmissionChannel) (string, error) {
	config := map[string]any{
		"mcpServers": map[string]any{
			"coordinator": map[string]any{
				"type": "http",
				"url":  channel.EndpointURL,
				"headers": map[string]string{
					"Authorization": "Bearer " + channel.BearerToken,
				},
			},
		},
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("encode claude mcp config: %w", err)
	}
	path := filepath.Join(runtimeRoot, "mcp-config.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return "", fmt.Errorf("write claude mcp config: %w", err)
	}
	return path, nil
}

func (p *streamParser) consume(line []byte) error {
	var event map[string]any
	if err := json.Unmarshal(line, &event); err != nil {
		return fmt.Errorf("decode claude stream event: %w", err)
	}
	eventType, _ := event["type"].(string)
	subtype, _ := event["subtype"].(string)
	switch eventType {
	case "system":
		switch subtype {
		case "init":
			p.emitEvent("claude_session", selectFields(event, "session_id", "model", "permissionMode", "claude_code_version"))
		case "api_retry":
			p.emitEvent("claude_retry", selectFields(event, "attempt", "max_retries", "retry_delay_ms"))
		}
	case "assistant":
		message, _ := event["message"].(map[string]any)
		payload := selectFields(message, "model")
		if usage, ok := message["usage"].(map[string]any); ok {
			payload["usage"] = selectUsage(usage)
		}
		p.emitEvent("claude_usage", payload)
		for _, block := range contentBlocks(message) {
			if name := stringValue(block, "name"); strings.HasPrefix(name, "submit_") {
				p.emitEvent("claude_progress", map[string]any{
					"item_type": "mcp_tool_call", "tool": name,
					"status": "in_progress", "tool_use_id": stringValue(block, "id"),
				})
			}
		}
	case "result":
		payload := selectFields(event,
			"session_id", "subtype", "stop_reason", "terminal_reason", "duration_ms", "duration_api_ms",
			"ttft_ms", "num_turns", "total_cost_usd", "is_error", "permission_denials",
		)
		if usage, ok := event["usage"].(map[string]any); ok {
			payload["usage"] = selectUsage(usage)
		}
		if modelUsage, ok := event["modelUsage"].(map[string]any); ok {
			payload["model_usage"] = modelUsage
		} else if modelUsage, ok := event["model_usage"].(map[string]any); ok {
			payload["model_usage"] = modelUsage
		}
		p.emitEvent("claude_result", payload)
		if failed, _ := event["is_error"].(bool); failed {
			if subtype == "" {
				subtype = "unknown"
			}
			message := p.failureMessage(event)
			errorPayload := map[string]any{"subtype": subtype}
			if message != "" {
				errorPayload["message"] = message
				p.emitEvent("claude_error", errorPayload)
				p.terminalError = fmt.Errorf("claude terminal result reported %s: %s", subtype, message)
				return nil
			}
			p.emitEvent("claude_error", errorPayload)
			p.terminalError = fmt.Errorf("claude terminal result reported %s", subtype)
			return nil
		}
		structured, exists := event["structured_output"]
		if !exists || structured == nil {
			p.terminalError = p.missingStructuredOutputError(line)
			return nil
		}
		raw, err := json.Marshal(structured)
		if err != nil || !json.Valid(raw) {
			p.terminalError = p.missingStructuredOutputError(line)
			return nil
		}
		p.structuredOutput = raw
	}
	return nil
}

func (p *streamParser) missingStructuredOutputError(line []byte) error {
	diagnosticsRoot, persistErr := p.persistDiagnostics(line)
	if persistErr != nil {
		return fmt.Errorf("claude terminal result is missing structured_output; preserve diagnostics: %w", persistErr)
	}
	return fmt.Errorf("claude terminal result is missing structured_output; diagnostics saved to %s", diagnosticsRoot)
}

func (p *streamParser) persistDiagnostics(line []byte) (string, error) {
	diagnosticsRoot := filepath.Join(p.workspacePath, diagnosticsDirectoryName)
	if err := os.MkdirAll(diagnosticsRoot, 0o700); err != nil {
		return diagnosticsRoot, fmt.Errorf("create Claude diagnostics directory: %w", err)
	}
	redacted := []byte(redactProviderError(string(line), p.provider, p.channel))
	if err := os.WriteFile(filepath.Join(diagnosticsRoot, "claude-result.json"), redacted, 0o600); err != nil {
		return diagnosticsRoot, fmt.Errorf("write invalid Claude result: %w", err)
	}
	return diagnosticsRoot, nil
}

func (p *streamParser) emitEvent(eventType string, payload map[string]any) {
	if p.emit != nil {
		p.emit(eventType, payload)
	}
}

func selectFields(source map[string]any, names ...string) map[string]any {
	result := make(map[string]any, len(names))
	for _, name := range names {
		if value, exists := source[name]; exists {
			result[name] = value
		}
	}
	return result
}

func stringValue(source map[string]any, name string) string {
	value, _ := source[name].(string)
	return strings.TrimSpace(value)
}

func selectUsage(source map[string]any) map[string]any {
	return selectFields(source,
		"input_tokens", "output_tokens", "cache_creation_input_tokens", "cache_read_input_tokens",
		"cache_creation", "server_tool_use",
	)
}

func (p *streamParser) failureMessage(event map[string]any) string {
	message := stringValue(event, "result")
	if message == "" {
		if errorObject, ok := event["error"].(map[string]any); ok {
			message = stringValue(errorObject, "message")
		}
	}
	if message == "" {
		return ""
	}
	return redactProviderError(message, p.provider, p.channel)
}

func redactProviderError(message string, provider runner.ProviderConfig, channel *runner.ResultSubmissionChannel) string {
	return runner.RedactSecrets(message, runner.ProviderSecrets(provider, channel))
}

func readTaskFile(root, name string) ([]byte, error) {
	root = strings.TrimSpace(root)
	name = strings.TrimSpace(name)
	if root == "" || name == "" {
		return nil, errors.New("workspace path and task filename are required")
	}
	if filepath.IsAbs(name) || filepath.Base(name) != name || name == "." || name == ".." {
		return nil, errors.New("task filename must be a safe workspace root entry")
	}
	return os.ReadFile(filepath.Join(root, name))
}

func canonicalJSON(raw []byte) ([]byte, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func claudeEnvironment(config runner.ProviderConfig, configDir string) ([]string, error) {
	environment := allowedBaseEnvironment(os.Environ())
	environment = append(environment,
		"CLAUDE_CONFIG_DIR="+configDir,
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
		// claude >= 2.1.212 auto-backgrounds MCP tool calls at 2 minutes, which
		// collides with the developer git finalize 3-minute sync cap; keep calls foreground.
		"CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS=0",
		"CLAUDE_CODE_DISABLE_UNKNOWN_MODEL_WINDOW_ENFORCEMENT=1",
	)
	if value := strings.TrimSpace(config.APIKey); value != "" {
		environment = append(environment, "ANTHROPIC_API_KEY="+value)
	}
	if value := strings.TrimSpace(config.BaseURL); value != "" {
		environment = append(environment, "ANTHROPIC_BASE_URL="+value)
	}
	if len(config.Headers) > 0 {
		names := make([]string, 0, len(config.Headers))
		headerValues := make(map[string]string, len(config.Headers))
		for rawName, value := range config.Headers {
			name := strings.TrimSpace(rawName)
			if name != rawName || !headerNamePattern.MatchString(name) || strings.ContainsAny(value, "\r\n") {
				return nil, errors.New("claude custom headers contain an unsafe name or value")
			}
			names = append(names, name)
			headerValues[name] = value
		}
		sort.Strings(names)
		lines := make([]string, 0, len(names))
		for _, name := range names {
			lines = append(lines, name+": "+headerValues[name])
		}
		environment = append(environment, "ANTHROPIC_CUSTOM_HEADERS="+strings.Join(lines, "\n"))
	}
	return environment, nil
}

func allowedBaseEnvironment(current []string) []string {
	allowed := map[string]struct{}{
		"PATH": {}, "HOME": {}, "USER": {}, "LANG": {}, "LC_ALL": {}, "TMPDIR": {}, "SHELL": {},
		"SYSTEMROOT": {}, "COMSPEC": {}, "PATHEXT": {}, "TEMP": {}, "TMP": {}, "USERPROFILE": {},
		"APPDATA": {}, "LOCALAPPDATA": {}, "PROGRAMFILES": {}, "PROGRAMFILES(X86)": {},
	}
	result := make([]string, 0, len(current))
	for _, item := range current {
		name, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if _, exists := allowed[strings.ToUpper(name)]; exists {
			result = append(result, item)
		}
	}
	return result
}

func contentBlocks(message map[string]any) []map[string]any {
	blocks, _ := message["content"].([]any)
	result := make([]map[string]any, 0, len(blocks))
	for _, block := range blocks {
		if blockMap, ok := block.(map[string]any); ok {
			result = append(result, blockMap)
		}
	}
	return result
}
