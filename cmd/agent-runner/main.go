package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/runner/claudecli"
	"sdlc-coordinator/internal/runner/codexcli"
	"sdlc-coordinator/internal/workspaceproto"
)

type config struct {
	HTTPAddr      string
	DataRoot      string
	ClaudeBinary  string
	CodexBinary   string
	WorkspaceRoot string
	// InternalToken 是受控工作区操作面的部署级令牌；空值表示通道未配置
	//（/internal/* fail-closed 503，不静态放行）。
	InternalToken string
	// AuthorizationURL 是协调器的当前授权确认入口；空值表示状态改变类操作
	// 未配置确认通道（commit/push/cleanup 一律拒绝）。
	AuthorizationURL string
}

func loadConfig(getenv func(string) string) config {
	httpAddr := strings.TrimSpace(getenv("AGENT_RUNNER_HTTP_ADDR"))
	if httpAddr == "" {
		httpAddr = ":8081"
	}
	dataRoot := strings.TrimSpace(getenv("AGENT_RUNNER_DATA_ROOT"))
	if dataRoot == "" {
		dataRoot = "/app/runner-data"
	}
	workspaceRoot := strings.TrimSpace(getenv("AGENT_RUNNER_WORKSPACE_ROOT"))
	if workspaceRoot == "" {
		workspaceRoot = filepath.Join(dataRoot, "workspaces")
	}
	return config{
		HTTPAddr:         httpAddr,
		DataRoot:         dataRoot,
		ClaudeBinary:     strings.TrimSpace(getenv("AGENT_RUNNER_CLAUDE_BINARY")),
		CodexBinary:      strings.TrimSpace(getenv("AGENT_RUNNER_CODEX_BINARY")),
		WorkspaceRoot:    workspaceRoot,
		InternalToken:    strings.TrimSpace(getenv("AGENT_RUNNER_INTERNAL_TOKEN")),
		AuthorizationURL: strings.TrimSpace(getenv("AGENT_RUNNER_AUTHORIZATION_URL")),
	}
}

func main() {
	config := loadConfig(os.Getenv)
	workspaceRoot, err := filepath.Abs(config.WorkspaceRoot)
	if err != nil {
		log.Fatalf("resolve runner workspace root: %v", err)
	}
	config.WorkspaceRoot = workspaceRoot
	if err := os.Setenv("AGENT_RUNNER_WORKSPACE_ROOT", workspaceRoot); err != nil {
		log.Fatalf("set runner workspace root: %v", err)
	}
	service, err := newService(config)
	if err != nil {
		log.Fatalf("initialize agent runner: %v", err)
	}
	var authorize func(context.Context, workspaceproto.AuthorizationRequest) (workspaceproto.AuthorizationDecision, error)
	if url := strings.TrimSpace(config.AuthorizationURL); url != "" {
		authorize = runner.NewHTTPAuthorizer(url, config.InternalToken, nil)
	}
	workspaceOps, err := runner.NewWorkspaceOps(runner.WorkspaceOpsConfig{
		DataRoot: config.DataRoot, WorkspaceRoot: config.WorkspaceRoot, Authorize: authorize,
	})
	if err != nil {
		log.Fatalf("initialize runner workspace operations: %v", err)
	}
	if authorize == nil {
		log.Printf("agent runner authorization channel is not configured (AGENT_RUNNER_AUTHORIZATION_URL empty): commit/push/cleanup will be refused")
	}
	handler := runner.NewHTTPHandler(service, runner.WithWorkspaceOperations(workspaceOps, config.InternalToken))
	if config.InternalToken == "" {
		log.Printf("agent runner internal workspace channel is not configured (AGENT_RUNNER_INTERNAL_TOKEN empty): /internal/* will answer 503")
	}
	server := &http.Server{
		Addr:              config.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// CLI 子进程在独立进程组中（便于取消时整树终止），不会随本进程的 SIGTERM
	// 退出；因此退出前必须主动 Shutdown：先停接客，再取消全部未终态执行并等待
	// 其终止，否则会遗留孤儿 CLI 进程继续消耗模型 API 与占用 workspace。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("agent runner listening on %s", config.HTTPAddr)
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()
	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve agent runner: %v", err)
		}
	case <-ctx.Done():
	}
	stop()

	// 退出顺序：先停接新工作（HTTP 排空 + 工作区操作停止接纳），再取消并
	// 等待在途执行与在途工作区操作收尾（有限等待，未完成留可对账记录）。
	workspaceOps.StopAccepting()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("drain agent runner http server: %v", err)
	}
	workspaceOps.WaitIdle(shutdownCtx)
	service.Shutdown(shutdownCtx)
}

func newService(config config) (*runner.Service, error) {
	return runner.NewService(config.DataRoot, map[string]runner.Provider{
		"claude-cli": claudecli.New(config.ClaudeBinary),
		"codex-cli":  codexcli.New(config.CodexBinary),
	})
}
