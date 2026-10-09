package main

import (
	"context"
	"errors"
	stdlog "log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestLoadConfigUsesRunnerDefaults(t *testing.T) {
	config := loadConfig(func(string) string { return "" })
	if config.HTTPAddr != ":8081" || config.DataRoot != "/app/runner-data" {
		t.Fatalf("unexpected runner defaults: %#v", config)
	}
	if config.ClaudeBinary != "" || config.CodexBinary != "" {
		t.Fatalf("unexpected cli binary defaults: %#v", config)
	}
	if config.LogLevel != "info" || config.LogDir != "logs/agent-runner" {
		t.Fatalf("unexpected runner logging defaults: %#v", config)
	}
}

func TestLoadConfigReadsExplicitRunnerSettings(t *testing.T) {
	values := map[string]string{
		"AGENT_RUNNER_HTTP_ADDR":     "127.0.0.1:19081",
		"AGENT_RUNNER_DATA_ROOT":     "/tmp/runner-data",
		"AGENT_RUNNER_CLAUDE_BINARY": "/home/runner/.local/bin/claude",
		"AGENT_RUNNER_CODEX_BINARY":  "/home/runner/.local/bin/codex",
		"AGENT_RUNNER_LOG_LEVEL":     "warn",
		"AGENT_RUNNER_LOG_DIR":       "/var/log/agent-runner",
	}
	config := loadConfig(func(name string) string { return values[name] })
	if config.HTTPAddr != "127.0.0.1:19081" || config.DataRoot != "/tmp/runner-data" {
		t.Fatalf("unexpected custom runner config: %#v", config)
	}
	if config.ClaudeBinary != "/home/runner/.local/bin/claude" || config.CodexBinary != "/home/runner/.local/bin/codex" {
		t.Fatalf("unexpected cli binary paths: %#v", config)
	}
	if config.LogLevel != "warn" || config.LogDir != "/var/log/agent-runner" {
		t.Fatalf("unexpected runner logging settings: %#v", config)
	}
}

func TestLoadConfigTrimsAndDefaultsRunnerLoggingSettings(t *testing.T) {
	blank := loadConfig(func(name string) string {
		if strings.HasPrefix(name, "AGENT_RUNNER_LOG_") {
			return "   "
		}
		return ""
	})
	if blank.LogLevel != "info" || blank.LogDir != "logs/agent-runner" {
		t.Fatalf("blank logging settings must fall back to defaults: %#v", blank)
	}

	padded := loadConfig(func(name string) string {
		switch name {
		case "AGENT_RUNNER_LOG_LEVEL":
			return "  warn  "
		case "AGENT_RUNNER_LOG_DIR":
			return "  /var/log/agent-runner  "
		default:
			return ""
		}
	})
	if padded.LogLevel != "warn" || padded.LogDir != "/var/log/agent-runner" {
		t.Fatalf("padded logging settings must be trimmed: %#v", padded)
	}
}

func TestLoadConfigIgnoresCoordinatorLoggingSettings(t *testing.T) {
	values := map[string]string{
		"COORDINATOR_LOG_LEVEL": "debug",
		"COORDINATOR_LOG_DIR":   "/var/log/coordinator",
	}
	config := loadConfig(func(name string) string { return values[name] })
	if config.LogLevel != "info" || config.LogDir != "logs/agent-runner" {
		t.Fatalf("runner must not inherit coordinator logging settings: %#v", config)
	}
}

func TestRunReportsLoggerInitializationFailure(t *testing.T) {
	occupied := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(occupied, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_RUNNER_LOG_DIR", occupied)
	t.Setenv("AGENT_RUNNER_DATA_ROOT", t.TempDir())
	stderr := captureStderr(t)

	if code := run(context.Background()); code != 1 {
		t.Fatalf("logger initialization failure must exit non-zero, got %d", code)
	}
	if output := stderr(); !strings.Contains(output, "initialize agent runner logger") {
		t.Fatalf("expected logger initialization error on stderr, got %q", output)
	}
}

func TestRunReportsServeFailureOnBothChannels(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "logs")
	t.Setenv("AGENT_RUNNER_HTTP_ADDR", "127.0.0.1:-1")
	t.Setenv("AGENT_RUNNER_DATA_ROOT", t.TempDir())
	t.Setenv("AGENT_RUNNER_LOG_DIR", logDir)
	t.Setenv("AGENT_RUNNER_LOG_LEVEL", "info")
	stderr := captureStderr(t)

	if code := run(context.Background()); code != 1 {
		t.Fatalf("serve failure must exit non-zero, got %d", code)
	}
	output := stderr()
	if !strings.Contains(output, "serve agent runner failed") || !strings.Contains(output, "http_addr") {
		t.Fatalf("expected serve failure with http_addr on stderr, got %q", output)
	}
	fileLog := readSingleRunnerLog(t, logDir)
	if !strings.Contains(fileLog, "serve agent runner failed") || !strings.Contains(fileLog, "127.0.0.1:-1") {
		t.Fatalf("expected serve failure in runner daily log, got %q", fileLog)
	}
}

func TestRunServesHealthAndWarnsWhenDrainTimesOut(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release reserved port: %v", err)
	}

	logDir := filepath.Join(t.TempDir(), "logs")
	t.Setenv("AGENT_RUNNER_HTTP_ADDR", addr)
	t.Setenv("AGENT_RUNNER_DATA_ROOT", t.TempDir())
	t.Setenv("AGENT_RUNNER_LOG_DIR", logDir)
	t.Setenv("AGENT_RUNNER_LOG_LEVEL", "info")
	stderr := captureStderr(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	code := make(chan int, 1)
	go func() { code <- run(ctx) }()

	response := getWithRetry(t, "http://"+addr+"/healthz")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("health check must stay green, got %d", response.StatusCode)
	}
	_ = response.Body.Close()

	// 打开一个不完整的请求连接:优雅停止期限内仍有活动连接,必须走到 drain
	// 超时的 Warn 分支而不是静默退出。
	connection, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial runner: %v", err)
	}
	defer connection.Close()
	if _, err := connection.Write([]byte("GET /healthz HTTP/1.1\r\nHost: runner\r\n")); err != nil {
		t.Fatalf("write partial request: %v", err)
	}
	// 给 Accept 循环留出接管连接的窗口:Shutdown 只把已被 serve 的连接算作活动。
	time.Sleep(200 * time.Millisecond)

	cancel()
	select {
	case exitCode := <-code:
		if exitCode != 0 {
			t.Fatalf("graceful stop must exit zero, got %d", exitCode)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("run did not return after context cancellation")
	}

	output := stderr()
	if !strings.Contains(output, "agent runner listening") || !strings.Contains(output, addr) {
		t.Fatalf("expected startup info with http_addr, got %q", output)
	}
	if !strings.Contains(output, "drain agent runner http server failed") {
		t.Fatalf("expected drain timeout warning, got %q", output)
	}
	fileLog := readSingleRunnerLog(t, logDir)
	for _, expected := range []string{"agent runner listening", "drain agent runner http server failed"} {
		if !strings.Contains(fileLog, expected) {
			t.Fatalf("expected %q in runner daily log, got %q", expected, fileLog)
		}
	}
}

func TestHTTPErrorLogAdapterRecordsSourceAndMessage(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	writer := httpErrorLogWriter{logger: zap.New(core)}
	stdlog.New(writer, "", 0).Print("http: TLS handshake error from 10.0.0.1:40000")

	entries := logs.FilterLevelExact(zapcore.ErrorLevel).All()
	if len(entries) != 1 || entries[0].Message != "http server error" {
		t.Fatalf("expected one http server error entry, got %#v", logs.All())
	}
	context := entries[0].ContextMap()
	if context["source"] != "http_server" {
		t.Fatalf("unexpected source field: %#v", context)
	}
	if context["message"] != "http: TLS handshake error from 10.0.0.1:40000" {
		t.Fatalf("unexpected message field: %#v", context)
	}
}

func TestAgentRunnerProcessHelper(t *testing.T) {
	if os.Getenv("AGENT_RUNNER_TEST_HELPER") != "1" {
		t.Skip("subprocess helper; driven by TestRunnerProcessExitCodes")
	}
	main()
}

func TestRunnerProcessExitCodes(t *testing.T) {
	occupied := filepath.Join(t.TempDir(), "occupied.log")
	if err := os.WriteFile(occupied, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name       string
		env        map[string]string
		wantStderr string
	}{
		{
			name:       "logger initialization failure",
			env:        map[string]string{"AGENT_RUNNER_LOG_DIR": occupied},
			wantStderr: "initialize agent runner logger",
		},
		{
			name: "serve failure",
			env: map[string]string{
				"AGENT_RUNNER_LOG_DIR":   filepath.Join(t.TempDir(), "logs"),
				"AGENT_RUNNER_HTTP_ADDR": "127.0.0.1:-1",
			},
			wantStderr: "serve agent runner failed",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			overrides := map[string]string{
				"AGENT_RUNNER_TEST_HELPER": "1",
				"AGENT_RUNNER_DATA_ROOT":   t.TempDir(),
			}
			for name, value := range testCase.env {
				overrides[name] = value
			}
			command := exec.Command(os.Args[0], "-test.run=TestAgentRunnerProcessHelper$")
			command.Env = envWith(overrides)
			output, err := command.CombinedOutput()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
				t.Fatalf("expected exit code 1, got err=%v output=%s", err, output)
			}
			if !strings.Contains(string(output), testCase.wantStderr) {
				t.Fatalf("expected %q in process output, got %s", testCase.wantStderr, output)
			}
		})
	}
}

func TestNewServiceRegistersCodeLevelCLIProviders(t *testing.T) {
	service, err := newService(config{DataRoot: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("new runner service: %v", err)
	}
	if names := service.ProviderNames(); !reflect.DeepEqual(names, []string{"claude-cli", "codex-cli"}) {
		t.Fatalf("unexpected provider registry: %#v", names)
	}
}

// captureStderr 把进程 stderr 指向临时文件:runner logger 的终端 core 与
// http.ErrorLog 都在构造时捕获 os.Stderr,因此替换必须发生在被测流程开始前。
func captureStderr(t *testing.T) func() string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stderr.log")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create stderr capture: %v", err)
	}
	original := os.Stderr
	os.Stderr = file
	t.Cleanup(func() {
		os.Stderr = original
		_ = file.Close()
	})
	return func() string {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read stderr capture: %v", err)
		}
		return string(data)
	}
}

func readSingleRunnerLog(t *testing.T, dir string) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "agent-runner-*.log"))
	if err != nil || len(files) != 1 {
		t.Fatalf("expected exactly one runner daily log in %s, got %v err=%v", dir, files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read runner daily log: %v", err)
	}
	return string(data)
}

func getWithRetry(t *testing.T, url string) *http.Response {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(url)
		if err == nil {
			return response
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("runner did not serve %s", url)
	return nil
}

func envWith(overrides map[string]string) []string {
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if _, replaced := overrides[name]; replaced {
			continue
		}
		env = append(env, entry)
	}
	for name, value := range overrides {
		env = append(env, name+"="+value)
	}
	return env
}
