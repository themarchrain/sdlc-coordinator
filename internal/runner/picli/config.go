package picli

import (
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

const (
	providerID        = "coordinator"
	mcpServerName     = "coordinator"
	mcpTimeoutSeconds = 600

	defaultAPI = "openai-completions"

	apiKeyEnv    = "PI_API_KEY"
	mcpTokenEnv  = "PI_MCP_TOKEN"
	agentDirEnv  = "PI_CODING_AGENT_DIR"
	headerEnvFmt = "PI_HEADER_%03d"
)

var headerNamePattern = regexp.MustCompile(`^[!#$%&'*+.^_` + "`" + `|~0-9A-Za-z-]+$`)

// normalizeGatewayBaseURL 统一网关地址形态:去掉尾部 /,anthropic 协议再剥
// 一层 /v1,因为 pi 自己会拼 /v1/messages。同一份地址因此能在 codex 与 pi
// 的三种协议间复用。
func normalizeGatewayBaseURL(raw, api string) string {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	if strings.EqualFold(strings.TrimSpace(api), "anthropic-messages") {
		value = strings.TrimSuffix(value, "/v1")
	}
	return value
}

// writeAgentDir 在 run 专属临时根下生成隔离的 PI_CODING_AGENT_DIR,秘密只
// 以环境变量名出现在文件里,真实值随返回的 env 传给子进程。
func writeAgentDir(root string, provider runner.ProviderConfig, channel *runner.ResultSubmissionChannel) (string, []string, error) {
	baseURL := normalizeGatewayBaseURL(provider.BaseURL, provider.API)
	if baseURL == "" {
		return "", nil, errors.New("pi provider base url is required")
	}
	model := strings.TrimSpace(provider.Model)
	if model == "" {
		return "", nil, errors.New("pi provider model is required")
	}
	api := strings.TrimSpace(provider.API)
	if api == "" {
		api = defaultAPI
	}

	agentDir := filepath.Join(root, "pi-agent")
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		return "", nil, fmt.Errorf("create pi agent dir: %w", err)
	}

	environment := []string{
		agentDirEnv + "=" + agentDir,
		apiKeyEnv + "=" + provider.APIKey,
	}
	coordinator := map[string]any{
		"baseUrl": baseURL,
		"api":     api,
		"apiKey":  "$" + apiKeyEnv,
		"models": []map[string]any{{
			"id": model, "name": model, "reasoning": true,
			"contextWindow": 128000, "maxTokens": 16384,
			"cost": map[string]any{"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0},
		}},
	}
	headerEnv, headers, err := headerEnvironment(provider.Headers)
	if err != nil {
		return "", nil, err
	}
	if len(headers) > 0 {
		coordinator["headers"] = headers
	}
	environment = append(environment, headerEnv...)
	if err := writeJSON(filepath.Join(agentDir, "models.json"), map[string]any{
		"providers": map[string]any{providerID: coordinator},
	}); err != nil {
		return "", nil, err
	}
	// 重试由协调器统一负责:pi 自带的会话级重试会拉长单次 run 的耗时,
	// 并让失败归属变得含混。
	if err := writeJSON(filepath.Join(agentDir, "settings.json"), map[string]any{
		"retry": map[string]any{"enabled": false},
	}); err != nil {
		return "", nil, err
	}
	if channel != nil {
		if err := writeJSON(filepath.Join(agentDir, "mcp.json"), map[string]any{
			"mcpServers": map[string]any{
				mcpServerName: map[string]any{
					"url":         channel.EndpointURL,
					"description": "Coordinator result channel",
					// 开发者交付在协调器内同步执行 git 操作,上限 3 分钟,
					// 默认 60 秒会切断提交并造成"已交付但结果被判失败"。
					"timeout":  mcpTimeoutSeconds,
					"exposure": "direct",
					"headers": map[string]any{
						"Authorization": "Bearer ${" + mcpTokenEnv + "}",
					},
				},
			},
		}); err != nil {
			return "", nil, err
		}
		environment = append(environment, mcpTokenEnv+"="+channel.BearerToken)
	}
	return agentDir, environment, nil
}

// headerEnvironment 按名字排序给自定义头编号,返回环境变量条目与写进
// models.json 的插值映射。
func headerEnvironment(headers map[string]string) ([]string, map[string]any, error) {
	if len(headers) == 0 {
		return nil, nil, nil
	}
	names := make([]string, 0, len(headers))
	for rawName, value := range headers {
		name := strings.TrimSpace(rawName)
		if name != rawName || !headerNamePattern.MatchString(name) || strings.ContainsAny(value, "\r\n") {
			return nil, nil, errors.New("pi custom headers contain an unsafe name or value")
		}
		names = append(names, name)
	}
	sort.Strings(names)
	environment := make([]string, 0, len(names))
	resolved := make(map[string]any, len(names))
	for index, name := range names {
		envName := fmt.Sprintf(headerEnvFmt, index)
		environment = append(environment, envName+"="+headers[name])
		resolved[name] = "$" + envName
	}
	return environment, resolved, nil
}

func writeJSON(path string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}
