package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sdlc-coordinator/internal/configuration"
	"sdlc-coordinator/internal/store"

	"github.com/gin-gonic/gin"
)

func TestAdminConfigurationGetMasksAllSecrets(t *testing.T) {
	handler, _ := newAdminConfigurationHandlerTest(t)
	router := gin.New()
	handler.RegisterRoutes(router.Group("/api/admin"))

	response := performAdminConfigurationRequest(t, router, http.MethodGet, "/api/admin/configuration", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, secret := range []string{"runner-key"} {
		if strings.Contains(body, secret) {
			t.Fatalf("configuration API leaked secret %q: %s", secret, body)
		}
	}
	for _, flag := range []string{
		`"agent_runner_api_key_configured":true`,
	} {
		if !strings.Contains(body, flag) {
			t.Fatalf("missing configured flag %s: %s", flag, body)
		}
	}
}

func TestAdminConfigurationPutRejectsUnknownFields(t *testing.T) {
	handler, _ := newAdminConfigurationHandlerTest(t)
	router := gin.New()
	handler.RegisterRoutes(router.Group("/api/admin"))

	response := performAdminConfigurationRequest(t, router, http.MethodPut, "/api/admin/configuration", map[string]any{
		"config":  configuration.EditableFromRuntime(configurationHandlerRuntimeConfig()),
		"secrets": map[string]any{},
		"unknown": true,
	})

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_request") {
		t.Fatalf("unexpected strict decode response %d: %s", response.Code, response.Body.String())
	}
}

func TestAdminConfigurationValidateReturnsFieldErrorsWithoutSaving(t *testing.T) {
	handler, service := newAdminConfigurationHandlerTest(t)
	router := gin.New()
	handler.RegisterRoutes(router.Group("/api/admin"))
	input := configuration.EditableFromRuntime(configurationHandlerRuntimeConfig())
	input.Scheduler.CallbackBaseURL = "invalid-url"

	response := performAdminConfigurationRequest(t, router, http.MethodPost, "/api/admin/configuration/validate", configuration.UpdateRequest{Config: input})

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "scheduler.callback_base_url") || !strings.Contains(response.Body.String(), `"valid":false`) {
		t.Fatalf("unexpected validation response %d: %s", response.Code, response.Body.String())
	}
	if service.Status().Revision != 1 || service.Status().Mode != configuration.ModeReady {
		t.Fatalf("validation persisted changes: %#v", service.Status())
	}
}

func TestAdminConfigurationRestartRespondsBeforeRequestingShutdown(t *testing.T) {
	handler, service := newAdminConfigurationHandlerTest(t)
	restart := &recordingShutdownRequester{called: make(chan struct{}, 1)}
	handler.shutdown = restart
	handler.restartDelay = 10 * time.Millisecond
	input := service.Get().Config
	input.Scheduler.Interval = "60s"
	if _, validationErrors, err := service.Update(context.Background(), configuration.UpdateRequest{Config: input}); err != nil || len(validationErrors) != 0 {
		t.Fatalf("prepare restart-required state: validation=%#v error=%v", validationErrors, err)
	}
	router := gin.New()
	handler.RegisterRoutes(router.Group("/api/admin"))

	response := performAdminConfigurationRequest(t, router, http.MethodPost, "/api/admin/configuration/restart", map[string]any{})

	if response.Code != http.StatusAccepted {
		t.Fatalf("unexpected restart response %d: %s", response.Code, response.Body.String())
	}
	if restart.count.Load() != 0 {
		t.Fatal("shutdown was requested before HTTP handler returned")
	}
	select {
	case <-restart.called:
	case <-time.After(time.Second):
		t.Fatal("shutdown was not requested after accepted response")
	}
}

type recordingShutdownRequester struct {
	count  atomic.Int32
	called chan struct{}
}

func (requester *recordingShutdownRequester) RequestShutdown() bool {
	requester.count.Add(1)
	select {
	case requester.called <- struct{}{}:
	default:
	}
	return true
}

func newAdminConfigurationHandlerTest(t *testing.T) (*AdminConfigurationHandler, *configuration.Service) {
	t.Helper()
	db, err := store.Open("file:" + strings.ReplaceAll(t.Name(), "/", "-") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repository := configuration.NewRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate configuration repository: %v", err)
	}
	runtimeConfig := configurationHandlerRuntimeConfig()
	snapshot, err := repository.Save(context.Background(), configuration.StatusReady, configuration.SourceUI, runtimeConfig)
	if err != nil {
		t.Fatalf("seed configuration: %v", err)
	}
	service := configuration.NewService(repository, configuration.Resolution{
		Mode: configuration.ModeReady, Prefill: runtimeConfig, Snapshot: snapshot,
	})
	return NewAdminConfigurationHandler(service, configuration.NewShutdownSignal()), service
}

func configurationHandlerRuntimeConfig() configuration.RuntimeConfig {
	return configuration.RuntimeConfig{
		AgentRunner: configuration.AgentRunnerConfig{
			ServiceURL: "http://agent-runner:8081", WorkspaceRoot: "/app/workspaces", Provider: "claude-cli", ProviderBaseURL: "https://gateway.example",
			APIKey: "runner-key", Model: "model-a", Effort: "high",
		},
		Scheduler: configuration.SchedulerConfig{
			Enabled: true, Interval: "30s",
			CallbackBaseURL: "https://coordinator.example.com", WorkspaceRoot: "/app/workspaces",
			DispatchLimit: 1, PollLimit: 5, ReconciliationLimit: 5, FinalizationLimit: 5, RecoveryLimit: 5, EffectLimit: 5,
		},
	}
}

func performAdminConfigurationRequest(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
