package picli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"sdlc-coordinator/internal/failure"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/runner"
)

const (
	defaultBinary = "pi"
	// toolSet 固定能力面:运行镜像没有 ripgrep,pi 内置的 grep/find 比
	// BusyBox 工具更可靠,也与 Claude Code 的工具面对齐。
	toolSet = "read,bash,edit,write,grep,find,ls"
)

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
		binary = defaultBinary
	}
	return &Provider{binary: binary, command: command}
}

func (p *Provider) Execute(ctx context.Context, request runner.ProviderRequest, emit runner.EmitFunc) (json.RawMessage, error) {
	if p == nil || p.command == nil {
		return nil, errors.New("pi command runner is required")
	}
	prompt, err := readTaskFile(request.WorkspacePath, request.PromptFile)
	if err != nil {
		return nil, wrapSetupFailure("read pi prompt", err)
	}
	// pi 不消费 schema 文件,但提示词引用它:只校验存在性,缺失就是初始化
	// 阶段的配置问题。
	if _, err := readTaskFile(request.WorkspacePath, request.SchemaFile); err != nil {
		return nil, wrapSetupFailure("read pi result schema", err)
	}
	// 运行时根放在 run 工作区里:launcher 以 agent 用户拉起 CLI 前会把整个
	// 工作区 chown 给 agent,而 /tmp 下的 0700 目录对 agent 不可读,pi 就读
	// 不到 models.json 了。位置与 codex 相同,不落进 repos/ 里的 git 工作树。
	runtimeRoot, err := os.MkdirTemp(request.WorkspacePath, ".agent-runner-pi-")
	if err != nil {
		return nil, wrapSetupFailure("create isolated pi runtime root", err)
	}
	defer os.RemoveAll(runtimeRoot)
	_, providerEnv, err := writeAgentDir(runtimeRoot, request.Provider, request.ResultSubmission)
	if err != nil {
		return nil, setupFailure(failure.CodeMissingConfiguration, err)
	}
	environment := allowedBaseEnvironment(os.Environ())
	environment = append(environment, providerEnv...)
	environment = append(environment,
		"PI_OFFLINE=1", "PI_SKIP_VERSION_CHECK=1", "PI_TELEMETRY=0",
	)

	args := []string{
		"--mode", "json", "--no-session",
		"--provider", providerID, "--model", strings.TrimSpace(request.Provider.Model),
	}
	if effort := strings.TrimSpace(request.Provider.Effort); effort != "" {
		args = append(args, "--thinking", effort)
	}
	// 工作区的 .pi/ 与 AGENTS.md 一律不加载:同一次 run 的行为只由协调器
	// 下发的配置决定。--no-extensions / --no-mcp 会关掉 MCP 结果通道,
	// 因此明确不加。
	args = append(args,
		"--no-context-files", "--no-skills", "--no-prompt-templates", "--no-themes",
		"--no-approve", "--offline",
		"--tools", toolSet,
	)

	parser := streamParser{
		emit: emit, provider: request.Provider, channel: request.ResultSubmission,
		taskID: request.TaskID, runID: request.ExecutionID,
	}
	if toolName, ok := results.ToolNameForWorkerRole(request.Role); ok {
		parser.expectedTool = toolName
	}
	err = p.command.Run(ctx, commandSpec{
		Binary: p.binary, Args: args, Dir: request.WorkspacePath, Env: environment, Stdin: string(prompt),
	}, parser.consume)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, setupFailure(failure.CodeCLIMissing, fmt.Errorf("run pi cli: %w", err))
		}
		// pi 在网关失败时仍以 0 退出,但真出现非零退出时事件里的失败证据
		// 更权威,不能因为退出码丢掉分类。
		if parser.failureMessage != "" {
			code, message := classifyProviderError(parser.failureMessage)
			parser.emitFailure(code, message)
			return nil, executionFailure(code, message, err)
		}
		return nil, fmt.Errorf("run pi cli: %s", redactProviderError(err.Error(), request.Provider, request.ResultSubmission))
	}
	if parser.failureMessage != "" {
		code, message := classifyProviderError(parser.failureMessage)
		parser.emitFailure(code, message)
		return nil, executionFailure(code, message, nil)
	}
	if parser.aborted {
		parser.emitFailure(failure.CodeProviderFailed, "run aborted")
		return nil, executionFailure(failure.CodeProviderFailed, "run aborted", nil)
	}
	if normalized, ok := normalizeFinalResult([]byte(parser.finalText)); ok {
		if runner.IsDeliveryMarker(normalized) {
			// 内部交付标记只能由 Provider 在核验协调器回执后生成:Agent 仿写
			// 的标记一律不采信,即使确有真回执也重新生成,绝不把 Agent 文本
			// 透传成内部协议信号。
			if marker := parser.deliveryMarker(); marker != nil {
				return marker, nil
			}
			return nil, errors.New("pi final result spoofs the coordinator delivery marker")
		}
		return normalized, nil
	}
	if marker := parser.deliveryMarker(); marker != nil {
		// 主通道已被接受且终稿是散文:按交付标记收尾,执行记录正常终态化。
		return marker, nil
	}
	return nil, errors.New("pi final result is invalid JSON")
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

func allowedBaseEnvironment(current []string) []string {
	allowed := map[string]struct{}{
		"PATH": {}, "HOME": {}, "USER": {}, "LANG": {}, "LC_ALL": {}, "TMPDIR": {}, "SHELL": {},
		"AGENT_RUNNER_CACHE_ROOT": {}, "GOPROXY": {}, "MAVEN_OPTS": {},
		"JAVA_HOME": {}, "JAVA8_HOME": {}, "JAVA17_HOME": {}, "JAVA21_HOME": {},
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
