package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sdlc-coordinator/internal/configuration"

	"github.com/gin-gonic/gin"
)

func TestHealthReportsBootstrapModeWhileReadinessFails(t *testing.T) {
	service := configuration.NewService(nil, configuration.Resolution{
		Mode: configuration.ModeSetupPending,
	})
	handler := NewHealthHandler(service)
	router := gin.New()
	router.GET("/healthz", handler.Health)
	router.GET("/readyz", handler.Ready)

	health := httptest.NewRecorder()
	router.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK || !strings.Contains(health.Body.String(), `"mode":"setup_pending"`) {
		t.Fatalf("unexpected health response %d: %s", health.Code, health.Body.String())
	}
	ready := httptest.NewRecorder()
	router.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusServiceUnavailable || !strings.Contains(ready.Body.String(), "configuration_required") {
		t.Fatalf("unexpected readiness response %d: %s", ready.Code, ready.Body.String())
	}
}

func TestHealthDefaultsToReadyForLegacyRouterConstruction(t *testing.T) {
	handler := NewHealthHandler(nil)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)

	handler.Health(context)

	if response.Code != http.StatusOK || response.Body.String() != `{"status":"ok"}` {
		t.Fatalf("unexpected legacy health response %d: %s", response.Code, response.Body.String())
	}
}
