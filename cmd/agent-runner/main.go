package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/runner/claudecli"
	"sdlc-coordinator/internal/runner/codexcli"
)

type config struct {
	HTTPAddr     string
	DataRoot     string
	ClaudeBinary string
	CodexBinary  string
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
	return config{
		HTTPAddr:     httpAddr,
		DataRoot:     dataRoot,
		ClaudeBinary: strings.TrimSpace(getenv("AGENT_RUNNER_CLAUDE_BINARY")),
		CodexBinary:  strings.TrimSpace(getenv("AGENT_RUNNER_CODEX_BINARY")),
	}
}

func main() {
	config := loadConfig(os.Getenv)
	service, err := newService(config)
	if err != nil {
		log.Fatalf("initialize agent runner: %v", err)
	}
	server := &http.Server{
		Addr:              config.HTTPAddr,
		Handler:           runner.NewHTTPHandler(service),
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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("drain agent runner http server: %v", err)
	}
	service.Shutdown(shutdownCtx)
}

func newService(config config) (*runner.Service, error) {
	return runner.NewService(config.DataRoot, map[string]runner.Provider{
		"claude-cli": claudecli.New(config.ClaudeBinary),
		"codex-cli":  codexcli.New(config.CodexBinary),
	})
}
