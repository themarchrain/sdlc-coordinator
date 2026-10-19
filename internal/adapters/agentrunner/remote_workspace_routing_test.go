package agentrunner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoutedRemoteWorkspaceDeletesExecutionOnRunEndpoint(t *testing.T) {
	defaultCalls := 0
	defaultRunner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defaultCalls++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer defaultRunner.Close()
	routedCalls := 0
	routedRunner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routedCalls++
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/executions/execution-1" {
			t.Errorf("unexpected delete request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer routedRunner.Close()

	workspace := NewRoutedRemoteWorkspace(defaultRunner.URL, "", 1, nil, func(_ context.Context, runID string) (string, error) {
		if runID != "run-1" {
			t.Errorf("unexpected run ID: %s", runID)
		}
		return routedRunner.URL, nil
	})
	if err := workspace.DeleteForRun(context.Background(), "run-1", "execution-1"); err != nil {
		t.Fatalf("delete execution: %v", err)
	}
	if routedCalls != 1 || defaultCalls != 0 {
		t.Fatalf("delete calls: routed=%d default=%d", routedCalls, defaultCalls)
	}
}
