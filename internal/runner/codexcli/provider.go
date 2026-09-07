package codexcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"sdlc-coordinator/internal/runner"
)

const (
	providerName       = "coordinator_provider"
	providerAPIKeyEnv  = "CODEX_PROVIDER_API_KEY"
	defaultOpenAIBase  = "https://api.openai.com/v1"
	defaultCodexBinary = "codex"
)

var headerNamePattern = regexp.MustCompile(`^[!#$%&'*+.^_` + "`" + `|~0-9A-Za-z-]+$`)

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
		binary = defaultCodexBinary
	}
	return &Provider{binary: binary, command: command}
}

func (p *Provider) Execute(ctx context.Context, request runner.ProviderRequest, emit runner.EmitFunc) (json.RawMessage, error) {
	if p == nil || p.command == nil {
		return nil, errors.New("codex command runner is required")
	}
	promptPath, err := safeTaskFile(request.WorkspacePath, request.PromptFile)
	if err != nil {
		return nil, fmt.Errorf("read codex prompt: %w", err)
	}
	schemaPath, err := safeTaskFile(request.WorkspacePath, request.SchemaFile)
	if err != nil {
		return nil, fmt.Errorf("read codex result schema: %w", err)
	}
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		return nil, fmt.Errorf("read codex prompt: %w", err)
	}
	schemaRaw, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("read codex result schema: %w", err)
	}
	adaptedSchema, err := adaptResponsesSchema(schemaRaw)
	if err != nil {
		return nil, err
	}

	runtimeRoot, err := os.MkdirTemp("", "agent-runner-codex-")
	if err != nil {
		return nil, fmt.Errorf("create isolated codex home: %w", err)
	}
	defer os.RemoveAll(runtimeRoot)
	codexHome := filepath.Join(runtimeRoot, "codex-home")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		return nil, fmt.Errorf("create isolated codex home: %w", err)
	}
	finalPath := filepath.Join(runtimeRoot, "codex-final.json")
	adaptedSchemaPath := filepath.Join(runtimeRoot, "codex-result-schema.json")
	if err := os.WriteFile(adaptedSchemaPath, adaptedSchema, 0o600); err != nil {
		return nil, fmt.Errorf("write adapted codex result schema: %w", err)
	}

	headerConfig, err := buildHeaderEnvironment(request.Provider.Headers)
	if err != nil {
		return nil, err
	}
	environment := allowedBaseEnvironment(os.Environ())
	environment = append(environment, "CODEX_HOME="+codexHome)
	if apiKey := strings.TrimSpace(request.Provider.APIKey); apiKey != "" {
		environment = append(environment, providerAPIKeyEnv+"="+apiKey)
	}
	environment = append(environment, headerConfig.environment...)

	baseURL := strings.TrimRight(strings.TrimSpace(request.Provider.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultOpenAIBase
	}
	args := []string{
		"exec", "--json", "--ephemeral", "--ignore-user-config", "--ignore-rules",
		"--dangerously-bypass-approvals-and-sandbox", "--skip-git-repo-check",
		"--color", "never",
		"--disable", "plugins", "--disable", "recommended_plugins",
		"--disable", "remote_plugin", "--disable", "plugin_sharing",
		"-c", `agents.enabled=false`,
		"-c", `web_search="disabled"`,
		"--output-last-message", finalPath,
		"-C", request.WorkspacePath,
		"-c", `model_provider="` + providerName + `"`,
		"-c", `model_providers.` + providerName + `.name="Coordinator Managed Provider"`,
		"-c", `model_providers.` + providerName + `.base_url=` + strconv.Quote(baseURL),
		"-c", `model_providers.` + providerName + `.env_key="` + providerAPIKeyEnv + `"`,
		"-c", `model_providers.` + providerName + `.wire_api="responses"`,
		"-c", `model_providers.` + providerName + `.requires_openai_auth=false`,
	}
	if headerConfig.inlineTable != "" {
		args = append(args, "-c", `model_providers.`+providerName+`.env_http_headers=`+headerConfig.inlineTable)
	}
	if model := strings.TrimSpace(request.Provider.Model); model != "" {
		args = append(args, "-m", model)
	}
	if effort := strings.TrimSpace(request.Provider.Effort); effort != "" {
		args = append(args, "-c", `model_reasoning_effort=`+strconv.Quote(effort))
	}
	// stdin 占位符必须在全部选项之后：codex 0.148.0 拒绝出现在选项之前的 "-"
	// （unexpected argument '-'），0.150+ 两种顺序都接受。
	args = append(args, "-")

	parser := eventParser{emit: emit, provider: request.Provider}
	if err := p.command.Run(ctx, commandSpec{
		Binary: p.binary, Args: args, Dir: request.WorkspacePath, Env: environment, Stdin: string(prompt),
	}, parser.consume); err != nil {
		if parser.failureMessage != "" {
			detail := parser.failureMessage
			if parser.failureCode != "" {
				detail = parser.failureCode + ": " + detail
			}
			return nil, fmt.Errorf("run codex cli: %s", detail)
		}
		return nil, fmt.Errorf("run codex cli: %s", redactProviderError(err.Error(), request.Provider))
	}
	result, err := os.ReadFile(finalPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("codex final result is missing")
		}
		return nil, fmt.Errorf("read codex final result: %w", err)
	}
	if normalized, ok := normalizeFinalResult(result); ok {
		return normalized, nil
	}
	diagnosticsRoot, diagnosticsErr := persistInvalidFinalDiagnostics(request.WorkspacePath, result, adaptedSchema, request.Provider)
	if diagnosticsErr != nil {
		return nil, fmt.Errorf("codex final result is invalid JSON; preserve diagnostics: %w", diagnosticsErr)
	}
	return nil, fmt.Errorf("codex final result is invalid JSON; diagnostics saved to %s", diagnosticsRoot)
}

func redactProviderError(message string, provider runner.ProviderConfig) string {
	secrets := make([]string, 0, len(provider.Headers)+1)
	if value := strings.TrimSpace(provider.APIKey); value != "" {
		secrets = append(secrets, value)
	}
	for _, value := range provider.Headers {
		if value = strings.TrimSpace(value); value != "" {
			secrets = append(secrets, value)
		}
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, secret := range secrets {
		message = strings.ReplaceAll(message, secret, "[REDACTED]")
	}
	return message
}

type headerEnvironment struct {
	inlineTable string
	environment []string
}

func buildHeaderEnvironment(headers map[string]string) (headerEnvironment, error) {
	if len(headers) == 0 {
		return headerEnvironment{}, nil
	}
	names := make([]string, 0, len(headers))
	for rawName, value := range headers {
		name := strings.TrimSpace(rawName)
		if name != rawName || !headerNamePattern.MatchString(name) || strings.ContainsAny(value, "\r\n") {
			return headerEnvironment{}, errors.New("codex custom headers contain an unsafe name or value")
		}
		names = append(names, name)
	}
	sort.Strings(names)
	entries := make([]string, 0, len(names))
	environment := make([]string, 0, len(names))
	for index, name := range names {
		environmentName := fmt.Sprintf("CODEX_PROVIDER_HEADER_%03d", index)
		entries = append(entries, strconv.Quote(name)+"="+strconv.Quote(environmentName))
		environment = append(environment, environmentName+"="+headers[name])
	}
	return headerEnvironment{inlineTable: "{" + strings.Join(entries, ",") + "}", environment: environment}, nil
}

func safeTaskFile(root, name string) (string, error) {
	root = strings.TrimSpace(root)
	name = strings.TrimSpace(name)
	if root == "" || name == "" {
		return "", errors.New("workspace path and task filename are required")
	}
	if filepath.IsAbs(name) || filepath.Base(name) != name || name == "." || name == ".." {
		return "", errors.New("task filename must be a safe workspace root entry")
	}
	return filepath.Join(root, name), nil
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

type eventParser struct {
	emit           runner.EmitFunc
	provider       runner.ProviderConfig
	failureCode    string
	failureMessage string
}

func (p *eventParser) consume(line []byte) error {
	var event map[string]any
	if err := json.Unmarshal(line, &event); err != nil {
		return fmt.Errorf("decode codex event: %w", err)
	}
	eventType, _ := event["type"].(string)
	switch eventType {
	case "thread.started":
		p.emitEvent("codex_session", selectFields(event, "thread_id"))
	case "turn.started":
		p.emitEvent("codex_turn", selectFields(event, "turn_id"))
	case "item.started", "item.completed":
		item, _ := event["item"].(map[string]any)
		payload := map[string]any{}
		if value := stringValue(item, "id"); value != "" {
			payload["item_id"] = value
		}
		if value := stringValue(item, "type"); value != "" {
			payload["item_type"] = value
		}
		if value := stringValue(item, "status"); value != "" {
			payload["status"] = value
		} else {
			payload["status"] = strings.TrimPrefix(eventType, "item.")
		}
		if stringValue(item, "type") == "error" {
			if message := stringValue(item, "message"); message != "" {
				payload["message"] = redactProviderError(message, p.provider)
			}
			if errorObject, ok := item["error"].(map[string]any); ok {
				if message := stringValue(errorObject, "message"); message != "" {
					payload["message"] = redactProviderError(message, p.provider)
				}
			}
		}
		p.emitEvent("codex_progress", payload)
	case "turn.completed":
		usage, _ := event["usage"].(map[string]any)
		p.emitEvent("codex_usage", selectFields(usage, "input_tokens", "cached_input_tokens", "output_tokens", "total_tokens"))
	case "turn.failed":
		payload := p.failurePayload(event)
		p.failureCode, _ = payload["error_code"].(string)
		p.failureMessage, _ = payload["message"].(string)
		p.emitEvent("codex_error", payload)
	}
	return nil
}

func (p *eventParser) failurePayload(event map[string]any) map[string]any {
	errorObject, _ := event["error"].(map[string]any)
	code := stringValue(errorObject, "code")
	if code == "" {
		code = stringValue(event, "error_code")
	}
	message := stringValue(errorObject, "message")
	if message == "" {
		message = stringValue(event, "message")
	}
	if embeddedCode, embeddedMessage := embeddedProviderError(message); embeddedMessage != "" {
		if code == "" {
			code = embeddedCode
		}
		message = embeddedMessage
	}
	payload := map[string]any{}
	if code != "" {
		payload["error_code"] = code
	}
	if message != "" {
		payload["message"] = redactProviderError(message, p.provider)
	}
	return payload
}

func embeddedProviderError(message string) (string, string) {
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(message)), &envelope) != nil || strings.TrimSpace(envelope.Error.Message) == "" {
		return "", ""
	}
	code := strings.TrimSpace(envelope.Error.Code)
	if code == "" {
		code = strings.TrimSpace(envelope.Error.Type)
	}
	return code, strings.TrimSpace(envelope.Error.Message)
}

func stringValue(source map[string]any, name string) string {
	value, _ := source[name].(string)
	return strings.TrimSpace(value)
}

func (p *eventParser) emitEvent(eventType string, payload map[string]any) {
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
