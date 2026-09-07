package handlers

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
)

// 真机冒烟（默认跳过）：真实 codex CLI 直连真实 /mcp 端点完成工具提交。
// 运行方式（所有变量经环境注入，不落盘不回显）：
//
//	RUN_MCP_REAL_SMOKE=true \
//	MCP_SMOKE_CODEX_BIN=<codex 可执行文件路径> \
//	MCP_SMOKE_BASE_URL=<Responses 兼容端点根> \
//	MCP_SMOKE_MODEL=<模型名> \
//	MCP_SMOKE_API_KEY=<端点密钥> \
//	go test ./internal/http/handlers/ -run TestMCPRealCodexSmoke -v
func TestMCPRealCodexSmoke(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("RUN_MCP_REAL_SMOKE")), "true") {
		t.Skip("set RUN_MCP_REAL_SMOKE=true to execute the real codex smoke")
	}
	codexBin := strings.TrimSpace(os.Getenv("MCP_SMOKE_CODEX_BIN"))
	baseURL := strings.TrimSpace(os.Getenv("MCP_SMOKE_BASE_URL"))
	modelName := strings.TrimSpace(os.Getenv("MCP_SMOKE_MODEL"))
	apiKey := strings.TrimSpace(os.Getenv("MCP_SMOKE_API_KEY"))
	if codexBin == "" || baseURL == "" || modelName == "" || apiKey == "" {
		t.Fatal("MCP_SMOKE_CODEX_BIN / MCP_SMOKE_BASE_URL / MCP_SMOKE_MODEL / MCP_SMOKE_API_KEY are all required")
	}

	f := newMCPFixture(t)
	f.createDeveloperRun(t, "run-mcp-real-smoke")
	server := httptest.NewServer(f.router)
	defer server.Close()

	token := f.token(mcpTestEndpointTask, "run-mcp-real-smoke", "DeveloperWorker", f.now())
	workspace := t.TempDir()
	promptPath := filepath.Join(workspace, "prompt.md")
	prompt := "你是 DeveloperWorker。本次执行的修复失败：目标函数返回了错误的状态码。\n" +
		"不要修改任何文件，也不要运行测试。请立即通过 MCP 工具 " + results.SubmitDeveloperResultTool + " 提交结果：\n" +
		"task_id=" + mcpTestEndpointTask + "，run_id=run-mcp-real-smoke，outcome=fix_failed，" +
		"summary 说明失败原因，blocker_reason 说明根因。\n" +
		"提交返回 accepted 后立即停止。"
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	args := []string{
		"exec", "--json", "--ephemeral", "--ignore-user-config", "--ignore-rules",
		"--dangerously-bypass-approvals-and-sandbox", "--skip-git-repo-check", "--color", "never",
		"-c", "agents.enabled=false", "-c", `web_search="disabled"`,
		"-c", `model_provider="smoketest"`,
		"-c", `model_providers.smoketest.name="Smoke Provider"`,
		"-c", `model_providers.smoketest.base_url="` + baseURL + `"`,
		"-c", `model_providers.smoketest.env_key="SMOKE_API_KEY"`,
		"-c", `model_providers.smoketest.wire_api="responses"`,
		"-c", `model_providers.smoketest.requires_openai_auth=false`,
		"-m", modelName,
		"-c", `mcp_servers.coordinator.url="` + server.URL + `/mcp"`,
		"-c", `mcp_servers.coordinator.bearer_token_env_var="SMOKE_RUN_TOKEN"`,
		"-C", filepath.ToSlash(workspace), "-",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, codexBin, args...)
	command.Env = append(os.Environ(),
		"SMOKE_API_KEY="+apiKey,
		"SMOKE_RUN_TOKEN="+token,
		"CODEX_HOME="+t.TempDir(),
	)
	command.Dir = workspace
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	output := &strings.Builder{}
	command.Stdout = output
	command.Stderr = output
	if err := command.Start(); err != nil {
		t.Fatalf("start codex: %v", err)
	}
	if _, err := stdin.Write([]byte(prompt)); err != nil {
		t.Fatalf("write prompt to stdin: %v", err)
	}
	stdin.Close()
	waitErr := command.Wait()

	if waitErr != nil {
		t.Fatalf("codex smoke run failed: %v\noutput tail: %s", waitErr, tail(output.String(), 2000))
	}
	runRecord, err := f.runs.GetByRunID(context.Background(), "run-mcp-real-smoke")
	if err != nil {
		t.Fatalf("load run: %v", err)
	}
	if runRecord.Status != model.RunStatusFailed {
		t.Fatalf("real codex submission must terminalize the run as failed(fix_failed), got %s\noutput tail: %s", runRecord.Status, tail(output.String(), 2000))
	}
	task, _ := f.tasks.GetByTaskID(context.Background(), mcpTestEndpointTask)
	if task.Status == model.TaskStatusHumanRequired || task.Status == model.TaskStatusBlocked {
		t.Fatalf("task must not be escalated by the MCP path, got %s", task.Status)
	}
	t.Logf("real codex smoke ok: run=%s task=%s", runRecord.Status, task.Status)
}

func tail(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[len(text)-limit:]
}
