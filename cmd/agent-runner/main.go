package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"sdlc-coordinator/internal/logging"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/runner/claudecli"
	"sdlc-coordinator/internal/runner/codexcli"
	"sdlc-coordinator/internal/workspaceproto"
)

const (
	defaultRunnerLogLevel = "info"
	shutdownBudget        = 5 * time.Second
)

type config struct {
	HTTPAddr      string
	DataRoot      string
	ClaudeBinary  string
	CodexBinary   string
	WorkspaceRoot string
	LogLevel      string
	LogDir        string
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
	logLevel := strings.TrimSpace(getenv("AGENT_RUNNER_LOG_LEVEL"))
	if logLevel == "" {
		logLevel = defaultRunnerLogLevel
	}
	logDir := strings.TrimSpace(getenv("AGENT_RUNNER_LOG_DIR"))
	if logDir == "" {
		logDir = logging.DefaultRunnerLogDir
	}
	return config{
		HTTPAddr:         httpAddr,
		DataRoot:         dataRoot,
		ClaudeBinary:     strings.TrimSpace(getenv("AGENT_RUNNER_CLAUDE_BINARY")),
		CodexBinary:      strings.TrimSpace(getenv("AGENT_RUNNER_CODEX_BINARY")),
		WorkspaceRoot:    workspaceRoot,
		InternalToken:    strings.TrimSpace(getenv("AGENT_RUNNER_INTERNAL_TOKEN")),
		AuthorizationURL: strings.TrimSpace(getenv("AGENT_RUNNER_AUTHORIZATION_URL")),
		LogLevel:         logLevel,
		LogDir:           logDir,
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, stop)
	stop()
	os.Exit(code)
}

// run 运行 runner 直到 ctx 取消或服务失败,返回进程退出码。stop 用于在收到关闭
// 信号后恢复默认信号处置(再按一次可以强退)。logger 的同步挂在返回前的 defer:
// 先停接客、再取消执行,最后刷日志,不让 os.Exit 跳过清理。
func run(ctx context.Context, stop func()) int {
	config := loadConfig(os.Getenv)
	logger, err := logging.NewRunner(config.LogLevel, config.LogDir)
	if err != nil {
		// logger 尚未可用的初始化失败边界:最小 stderr 提示,不保留标准库 log。
		fmt.Fprintf(os.Stderr, "initialize agent runner logger: %v\n", err)
		return 1
	}
	defer func() { _ = logger.Sync() }()

	workspaceRoot, err := filepath.Abs(config.WorkspaceRoot)
	if err != nil {
		logger.Error("resolve runner workspace root failed", zap.Error(err))
		return 1
	}
	config.WorkspaceRoot = workspaceRoot
	if err := os.Setenv("AGENT_RUNNER_WORKSPACE_ROOT", workspaceRoot); err != nil {
		logger.Error("set runner workspace root failed", zap.Error(err))
		return 1
	}

	service, err := newService(config, logger)
	if err != nil {
		logger.Error("initialize agent runner failed", zap.Error(err))
		return 1
	}
	var authorize func(context.Context, workspaceproto.AuthorizationRequest) (workspaceproto.AuthorizationDecision, error)
	if url := strings.TrimSpace(config.AuthorizationURL); url != "" {
		authorize = runner.NewHTTPAuthorizer(url, config.InternalToken, nil)
	}
	workspaceOps, err := runner.NewWorkspaceOps(runner.WorkspaceOpsConfig{
		DataRoot: config.DataRoot, WorkspaceRoot: config.WorkspaceRoot, Authorize: authorize,
	})
	if err != nil {
		logger.Error("initialize runner workspace operations failed", zap.Error(err))
		return 1
	}
	if authorize == nil {
		logger.Warn("agent runner authorization channel is not configured: commit/push/cleanup will be refused")
	}
	handler := runner.NewHTTPHandler(service, runner.WithWorkspaceOperations(workspaceOps, config.InternalToken))
	if config.InternalToken == "" {
		logger.Warn("agent runner internal workspace channel is not configured: /internal/* will answer 503")
	}
	server := newServer(config, logger, handler)

	// CLI 子进程在独立进程组中（便于取消时整树终止），不会随本进程的 SIGTERM
	// 退出；因此退出前必须主动 Shutdown：先停接客，再取消全部未终态执行并等待
	// 其终止，否则会遗留孤儿 CLI 进程继续消耗模型 API 与占用 workspace。
	logger.Info("agent runner listening", zap.String("http_addr", config.HTTPAddr))
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()
	exitCode := 0
	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("serve agent runner failed", zap.String("http_addr", config.HTTPAddr), zap.Error(err))
			exitCode = 1
		}
	case <-ctx.Done():
	}
	// 关闭窗口内恢复默认信号处置:此时再收到 SIGTERM/SIGINT 直接终止进程。
	stop()

	// 退出顺序：先停接新工作（HTTP 排空 + 工作区操作停止接纳），再取消并
	// 等待在途执行与在途工作区操作收尾（有限等待，未完成留可对账记录）。
	workspaceOps.StopAccepting()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownBudget)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Warn("drain agent runner http server failed", zap.Error(err))
	}
	workspaceOps.WaitIdle(shutdownCtx)
	service.Shutdown(shutdownCtx)
	return exitCode
}

// newServer 组装 HTTP 服务:ErrorLog 固定接入实例 logger 的局部适配,不重定向
// 标准库全局配置。
func newServer(config config, logger *zap.Logger, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              config.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          log.New(httpErrorLogWriter{logger: logger}, "", 0),
	}
}

// httpErrorLogWriter 把 http.Server 内部错误(默认走标准库 log)按 Error 级别
// 转写到实例 logger 并标注来源;这是 ErrorLog 的局部适配,不重定向标准库全局
// 配置,也不改变 HTTP panic 的原有恢复与连接行为。
type httpErrorLogWriter struct{ logger *zap.Logger }

func (w httpErrorLogWriter) Write(data []byte) (int, error) {
	w.logger.Error("http server error",
		zap.String("source", "http_server"),
		zap.String("message", strings.TrimSpace(string(data))))
	return len(data), nil
}

func newService(config config, logger *zap.Logger) (*runner.Service, error) {
	return runner.NewService(config.DataRoot, map[string]runner.Provider{
		"claude-cli": claudecli.New(config.ClaudeBinary),
		"codex-cli":  codexcli.New(config.CodexBinary),
	}, runner.WithLogger(logger))
}
