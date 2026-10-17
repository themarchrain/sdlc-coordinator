package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestGinRecoveryLogsOnceAndKeepsInternalServerError(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	defer gin.SetMode(previousMode)

	core, observed := observer.New(zapcore.DebugLevel)
	router := NewRouter(nil, WithRuntimeLogger(zap.New(core)))
	router.GET("/test-panic", func(*gin.Context) {
		panic("test panic")
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/test-panic", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, recorder.Code)
	}
	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("expected one recovery log entry, got %d", len(entries))
	}
	if entries[0].Level != zapcore.ErrorLevel {
		t.Fatalf("expected error-level recovery log, got %s", entries[0].Level)
	}
	if entries[0].ContextMap()["component"] != "gin_recovery" || !strings.Contains(entries[0].Message, "test panic") {
		t.Fatalf("unexpected recovery log: %+v", entries[0])
	}
}

func TestHTTPServerErrorLogUsesInjectedZapLogger(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	server := NewServer(":0", http.NotFoundHandler(), zap.New(core))
	server.server.ErrorLog.Printf("http: test diagnostic")

	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("expected one HTTP server log entry, got %d", len(entries))
	}
	if entries[0].Level != zapcore.ErrorLevel {
		t.Fatalf("expected error-level HTTP server log, got %s", entries[0].Level)
	}
	if entries[0].ContextMap()["component"] != "http_server" || !strings.Contains(entries[0].Message, "http: test diagnostic") {
		t.Fatalf("unexpected HTTP server log: %+v", entries[0])
	}
}

// 未注入 logger 时框架诊断回退 stderr 直写:gin panic 现场与 http.Server
// 连接错误在迁移前恒写 stderr,漏注入不得让诊断静默消失。
func TestGinRecoveryFallsBackToStderrWithoutLogger(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	defer gin.SetMode(previousMode)

	// 最小装配:不注入 logger,panic 现场必须仍可见。
	router := NewRouter(nil)
	router.GET("/test-panic-no-logger", func(*gin.Context) {
		panic("panic without logger")
	})

	captured := captureStderrDuring(t, func() {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/test-panic-no-logger", nil))
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, recorder.Code)
		}
	})
	if !strings.Contains(captured, "panic without logger") {
		t.Fatalf("panic without logger must fall back to stderr, got %q", captured)
	}
}

func TestHTTPServerErrorLogFallsBackToStderrWithoutLogger(t *testing.T) {
	server := NewServer(":0", http.NotFoundHandler(), nil)

	captured := captureStderrDuring(t, func() {
		server.server.ErrorLog.Printf("http: fallback diagnostic")
	})
	if !strings.Contains(captured, "http: fallback diagnostic") {
		t.Fatalf("server diagnostics must fall back to stderr, got %q", captured)
	}
}

// captureStderrDuring 在 run 执行期间把进程 stderr 替换为管道并返回捕获文本。
// 回退路径在 Write 时读取 os.Stderr;读取端并发排空,避免大载荷填满管道缓冲。
func captureStderrDuring(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	original := os.Stderr
	os.Stderr = writer
	defer func() {
		os.Stderr = original
		_ = writer.Close()
	}()

	var buf strings.Builder
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, reader)
		close(done)
	}()

	run()
	_ = writer.Close()
	<-done
	_ = reader.Close()
	return buf.String()
}
