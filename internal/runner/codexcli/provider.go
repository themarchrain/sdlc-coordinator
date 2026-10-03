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

	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/runner"
)

const (
	providerName       = "coordinator_provider"
	providerAPIKeyEnv  = "CODEX_PROVIDER_API_KEY"
	runTokenEnv        = "COORDINATOR_RUN_TOKEN"
	mcpServerName      = "coordinator"
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
	if request.ResultSubmission != nil {
		// 生产形态带 --ignore-user-config，config.toml 的 MCP 被屏蔽，-c 覆盖是
		// 唯一注入路径；token 走 bearer_token_env_var 从进程环境读。
		args = append(args,
			"-c", `mcp_servers.`+mcpServerName+`.url=`+strconv.Quote(request.ResultSubmission.EndpointURL),
			"-c", `mcp_servers.`+mcpServerName+`.bearer_token_env_var=`+strconv.Quote(runTokenEnv),
		)
		environment = append(environment, runTokenEnv+"="+request.ResultSubmission.BearerToken)
	}
	// stdin 占位符必须在全部选项之后：codex 0.148.0 拒绝出现在选项之前的 "-"
	// （unexpected argument '-'），0.150+ 两种顺序都接受。
	args = append(args, "-")

	parser := eventParser{
		emit: emit, provider: request.Provider, channel: request.ResultSubmission,
		taskID: request.TaskID, runID: request.ExecutionID,
	}
	if toolName, ok := results.ToolNameForWorkerRole(request.Role); ok {
		parser.expectedTool = toolName
	}
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
		return nil, fmt.Errorf("run codex cli: %s", redactProviderError(err.Error(), request.Provider, request.ResultSubmission))
	}
	if parser.turnFailed {
		// turn.failed 是 CLI 的终止失败事件:即使进程正常返回、最终消息是
		// 合法 JSON,也不得记成成功,更不得被接受回执覆盖。
		detail := parser.failureMessage
		if detail == "" {
			detail = "turn failed"
		}
		if parser.failureCode != "" {
			detail = parser.failureCode + ": " + detail
		}
		return nil, fmt.Errorf("run codex cli: %s", detail)
	}
	result, err := os.ReadFile(finalPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if marker := parser.deliveryMarker(); marker != nil {
				// 最终消息缺失但主通道已被接受:按交付标记收尾,执行记录正常终态化。
				return marker, nil
			}
			return nil, errors.New("codex final result is missing")
		}
		// 非缺失类读取故障是真实执行异常,不接受回执覆盖。
		return nil, fmt.Errorf("read codex final result: %w", err)
	}
	if normalized, ok := normalizeFinalResult(result); ok {
		if runner.IsDeliveryMarker(normalized) {
			// 内部交付标记只能由 Provider 在核验协调器回执后自行生成:Agent
			// 最终消息仿写的标记一律不采信,即使确有真回执也重新生成,绝不
			// 把 Agent 文本透传成内部协议信号。
			if marker := parser.deliveryMarker(); marker != nil {
				return marker, nil
			}
			diagnosticsRoot, diagnosticsErr := persistInvalidFinalDiagnostics(request.WorkspacePath, result, adaptedSchema, request.Provider, request.ResultSubmission)
			if diagnosticsErr != nil {
				return nil, fmt.Errorf("codex final result spoofs the coordinator delivery marker; preserve diagnostics: %w", diagnosticsErr)
			}
			return nil, fmt.Errorf("codex final result spoofs the coordinator delivery marker; diagnostics saved to %s", diagnosticsRoot)
		}
		return normalized, nil
	}
	if marker := parser.deliveryMarker(); marker != nil {
		// 最终消息非 JSON 但主通道已被接受:不再落无效结果诊断。
		return marker, nil
	}
	diagnosticsRoot, diagnosticsErr := persistInvalidFinalDiagnostics(request.WorkspacePath, result, adaptedSchema, request.Provider, request.ResultSubmission)
	if diagnosticsErr != nil {
		return nil, fmt.Errorf("codex final result is invalid JSON; preserve diagnostics: %w", diagnosticsErr)
	}
	return nil, fmt.Errorf("codex final result is invalid JSON; diagnostics saved to %s", diagnosticsRoot)
}

func redactProviderError(message string, provider runner.ProviderConfig, channel *runner.ResultSubmissionChannel) string {
	return runner.RedactSecrets(message, runner.ProviderSecrets(provider, channel))
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
	channel        *runner.ResultSubmissionChannel
	failureCode    string
	failureMessage string
	// 本次执行的业务身份与角色工具名:接受回执必须与之完全对齐才可采信。
	taskID       string
	runID        string
	expectedTool string
	// acceptedReceipt 表示已观测到本 run 的 MCP 成功回执;turnFailed 表示
	// 出现终止失败事件,Execute 在读取最终结果前即按失败收尾。
	acceptedReceipt bool
	turnFailed      bool
}

// knownNonFatal reports provider-side notices that every session emits but
// that must not be classified as failures (probe quirk: "Model metadata ...
// not found" fires once per session for any non-OpenAI model name).
func knownNonFatal(message string) bool {
	return strings.Contains(message, "Model metadata") && strings.Contains(message, "not found")
}

// mcpResultText joins text content blocks of an MCP tool result.
func mcpResultText(result map[string]any) string {
	content, _ := result["content"].([]any)
	texts := make([]string, 0, len(content))
	for _, block := range content {
		blockMap, _ := block.(map[string]any)
		if text, _ := blockMap["text"].(string); text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n")
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
		switch stringValue(item, "type") {
		case "error":
			if message := stringValue(item, "message"); message != "" {
				payload["message"] = redactProviderError(message, p.provider, p.channel)
				if knownNonFatal(message) {
					payload["non_fatal"] = true
				}
			}
			if errorObject, ok := item["error"].(map[string]any); ok {
				if message := stringValue(errorObject, "message"); message != "" {
					payload["message"] = redactProviderError(message, p.provider, p.channel)
					if knownNonFatal(message) {
						payload["non_fatal"] = true
					}
				}
			}
		case "mcp_tool_call":
			payload["server"] = stringValue(item, "server")
			payload["tool"] = stringValue(item, "tool")
			if result, ok := item["result"].(map[string]any); ok {
				if text := mcpResultText(result); text != "" {
					payload["result_text"] = redactProviderError(text, p.provider, p.channel)
				}
			}
			if errorObject, ok := item["error"].(map[string]any); ok {
				if message := stringValue(errorObject, "message"); message != "" {
					payload["error_message"] = redactProviderError(message, p.provider, p.channel)
				}
			}
			if eventType == "item.completed" {
				p.observeAcceptanceReceipt(item)
			}
		}
		p.emitEvent("codex_progress", payload)
	case "turn.completed":
		usage, _ := event["usage"].(map[string]any)
		p.emitEvent("codex_usage", selectFields(usage, "input_tokens", "cached_input_tokens", "output_tokens", "total_tokens"))
	case "turn.failed":
		p.turnFailed = true
		payload := p.failurePayload(event)
		p.failureCode, _ = payload["error_code"].(string)
		p.failureMessage, _ = payload["message"].(string)
		p.emitEvent("codex_error", payload)
	}
	return nil
}

// observeAcceptanceReceipt 只在结构化的工具完成事件上采信 MCP 成功回执:
// 工具完成、无错误、服务与角色工具名匹配、回执 JSON 的 status 为 accepted
// 且 task_id/run_id/run_status 与本次执行对齐才置位。仅配置了通道、仅出现
// 工具调用、或文本自称成功都不算。
func (p *eventParser) observeAcceptanceReceipt(item map[string]any) {
	if p.acceptedReceipt || p.channel == nil || p.expectedTool == "" {
		return
	}
	if stringValue(item, "status") != "completed" {
		return
	}
	if stringValue(item, "server") != mcpServerName || stringValue(item, "tool") != p.expectedTool {
		return
	}
	if errorObject, ok := item["error"].(map[string]any); ok && len(errorObject) > 0 {
		return
	}
	result, ok := item["result"].(map[string]any)
	if !ok {
		return
	}
	if isError, ok := result["isError"].(bool); ok && isError {
		return
	}
	if _, ok := runner.ParseMCPAcceptedReceipt(mcpResultText(result), p.taskID, p.runID); !ok {
		return
	}
	p.acceptedReceipt = true
}

// deliveryMarker 只在存在本 run 的 MCP 成功回执时返回交付标记;CLI 正常
// 退出由调用点保证,turn.failed 在读取最终结果前已被判失败。
func (p *eventParser) deliveryMarker() json.RawMessage {
	if !p.acceptedReceipt {
		return nil
	}
	return runner.NewDeliveryMarker(p.taskID, p.runID)
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
		payload["message"] = redactProviderError(message, p.provider, p.channel)
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
