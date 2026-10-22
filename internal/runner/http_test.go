package runner_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"sdlc-coordinator/internal/runner"
)

func TestHTTPHandlerCreatesAndQueriesExecution(t *testing.T) {
	t.Parallel()

	provider := &successfulProvider{result: json.RawMessage(`{"task_id":"task-http","run_id":"run-http","verdict":"approved"}`)}
	service, err := runner.NewService(t.TempDir(), map[string]runner.Provider{"fake": provider})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	server := httptest.NewServer(runner.NewHTTPHandler(service))
	defer server.Close()

	response, err := http.Get(server.URL + "/healthz")
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("health request: status=%v err=%v", statusCode(response), err)
	}
	_ = response.Body.Close()

	request := sampleRequest()
	request.ExecutionID = "run-http"
	request.TaskID = "task-http"
	raw, _ := json.Marshal(request)
	response, err = http.Post(server.URL+"/api/v1/executions", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", response.StatusCode)
	}
	_ = response.Body.Close()
	waitForStatus(t, service, "run-http", runner.StatusSucceeded)

	for _, path := range []string{
		"/api/v1/executions/run-http",
		"/api/v1/executions/run-http/events?after_seq=0",
		"/api/v1/executions/run-http/result",
	} {
		response, err = http.Get(server.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("GET %s returned %d", path, response.StatusCode)
		}
		_ = response.Body.Close()
	}

	missing, err := http.Get(server.URL + "/api/v1/executions/missing")
	if err != nil {
		t.Fatalf("missing execution request: %v", err)
	}
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", missing.StatusCode)
	}
	_ = missing.Body.Close()
}

func TestHTTPHandlerDeletesSucceededExecution(t *testing.T) {
	t.Parallel()
	service, err := runner.NewService(t.TempDir(), map[string]runner.Provider{"fake": &successfulProvider{result: json.RawMessage(`{"ok":true}`)}})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	server := httptest.NewServer(runner.NewHTTPHandler(service, runner.WithWorkspaceOperations(nil, "deployment")))
	defer server.Close()

	request := sampleRequest()
	request.ExecutionID = "run-http-delete"
	request.TaskID = "task-http-delete"
	raw, _ := json.Marshal(request)
	response, err := http.Post(server.URL+"/api/v1/executions", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", response.StatusCode)
	}
	_ = response.Body.Close()
	waitForStatus(t, service, "run-http-delete", runner.StatusSucceeded)

	deleteRequest, err := http.NewRequest(http.MethodDelete, server.URL+"/api/v1/executions/run-http-delete", nil)
	if err != nil {
		t.Fatal(err)
	}
	deleteRequest.Header.Set("Authorization", "Bearer deployment")
	response, err = http.DefaultClient.Do(deleteRequest)
	if err != nil || response.StatusCode != http.StatusNoContent {
		t.Fatalf("delete succeeded execution: status=%v err=%v", statusCode(response), err)
	}
	_ = response.Body.Close()
	response, err = http.DefaultClient.Do(deleteRequest)
	if err != nil || response.StatusCode != http.StatusNoContent {
		t.Fatalf("repeat delete: status=%v err=%v", statusCode(response), err)
	}
	_ = response.Body.Close()
}

func TestHTTPArtifactDeletionRequiresInternalToken(t *testing.T) {
	service, err := runner.NewService(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(runner.NewHTTPHandler(service, runner.WithWorkspaceOperations(nil, "deployment")))
	defer server.Close()
	for _, token := range []string{"", "wrong", "deployment"} {
		request, _ := http.NewRequest(http.MethodDelete, server.URL+"/api/v1/executions/absent", nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		want := http.StatusUnauthorized
		if token == "deployment" {
			want = http.StatusNoContent
		}
		response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("token=%q status=%v want=%v", token, response.StatusCode, want)
		}
	}
}

func statusCode(response *http.Response) int {
	if response == nil {
		return 0
	}
	return response.StatusCode
}
