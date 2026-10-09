package api

import (
	"net/http"
	"net/http/httptest"
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
