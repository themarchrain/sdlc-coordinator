package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sdlc-coordinator/internal/configuration"

	"github.com/gin-gonic/gin"
)

func TestConfigurationGateRejectsRuntimeRequestOutsideReadyStartup(t *testing.T) {
	called := false
	router := gin.New()
	router.POST("/runtime", ConfigurationGate(configuration.ModeSetupPending), func(c *gin.Context) {
		called = true
		c.Status(http.StatusNoContent)
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/runtime", nil))

	if called {
		t.Fatal("runtime handler executed in setup_pending mode")
	}
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "configuration_required") {
		t.Fatalf("unexpected gate response %d: %s", response.Code, response.Body.String())
	}
}

func TestConfigurationGateAllowsReadyStartup(t *testing.T) {
	router := gin.New()
	router.POST("/runtime", ConfigurationGate(configuration.ModeReady), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/runtime", nil))

	if response.Code != http.StatusNoContent {
		t.Fatalf("ready request was blocked: %d %s", response.Code, response.Body.String())
	}
}
