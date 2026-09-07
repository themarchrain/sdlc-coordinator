package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPClientForwardsRepositoryIDOnRunsRecordsAndMetrics(t *testing.T) {
	var runQuery, recordQuery, metricsQuery string
	var triggerBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "POST /plugin/v1/runs":
			if err := json.NewDecoder(r.Body).Decode(&triggerBody); err != nil {
				t.Fatalf("decode trigger: %v", err)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"run_id":"sync-run-1","trigger":"manual","status":"running"}`))
		case "GET /plugin/v1/runs":
			runQuery = r.URL.RawQuery
			_, _ = w.Write([]byte(`{"items":[],"page":1,"page_size":10,"total":0}`))
		case "GET /plugin/v1/records":
			recordQuery = r.URL.RawQuery
			_, _ = w.Write([]byte(`{"items":[],"page":1,"page_size":10,"total":0}`))
		case "GET /plugin/v1/metrics":
			metricsQuery = r.URL.RawQuery
			_, _ = w.Write([]byte(`{"values":{"runs_total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewHTTPClient()
	ctx := context.Background()
	if _, err := client.TriggerRun(ctx, server.URL, "request-1", "repo_a"); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if triggerBody["request_id"] != "request-1" || triggerBody["repository_id"] != "repo_a" {
		t.Fatalf("trigger body %#v", triggerBody)
	}
	if _, err := client.ListRuns(ctx, server.URL, PageRequest{Page: 1, PageSize: 10, RepositoryID: "repo_a"}); err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if !strings.Contains(runQuery, "repository_id=repo_a") {
		t.Fatalf("runs query %q", runQuery)
	}
	if _, err := client.ListRecords(ctx, server.URL, PageRequest{Page: 1, PageSize: 10, RepositoryID: "repo_a"}); err != nil {
		t.Fatalf("list records: %v", err)
	}
	if !strings.Contains(recordQuery, "repository_id=repo_a") {
		t.Fatalf("records query %q", recordQuery)
	}
	if _, err := client.Metrics(ctx, server.URL, "repo_a"); err != nil {
		t.Fatalf("metrics: %v", err)
	}
	if metricsQuery != "repository_id=repo_a" {
		t.Fatalf("metrics query %q", metricsQuery)
	}
}

func TestHTTPClientUsesCanonicalConnectorContract(t *testing.T) {
	var triggerRequestID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /plugin/v1/manifest":
			_, _ = w.Write([]byte(`{"contract_version":"1.0","kind":"state_source_sync","name":"Task Sync","version":"0.1.0","capabilities":["manual_trigger","run_history"]}`))
		case "GET /plugin/v1/health":
			_, _ = w.Write([]byte(`{"status":"healthy","message":"ready"}`))
		case "POST /plugin/v1/runs":
			var request map[string]string
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode trigger request: %v", err)
			}
			triggerRequestID = request["request_id"]
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"run_id":"sync-run-1","trigger":"manual","status":"running"}`))
		case "GET /plugin/v1/runs":
			if r.URL.Query().Get("page") != "2" || r.URL.Query().Get("page_size") != "10" {
				t.Fatalf("unexpected run query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"items":[{"run_id":"sync-run-1","trigger":"manual","status":"succeeded"}],"page":2,"page_size":10,"total":1}`))
		case "GET /plugin/v1/runs/sync-run-1":
			_, _ = w.Write([]byte(`{"run_id":"sync-run-1","trigger":"manual","status":"succeeded"}`))
		case "GET /plugin/v1/records":
			_, _ = w.Write([]byte(`{"items":[{"record_id":"record-1","source_type":"source_task","source_id":"task-1","status":"synced"}],"page":1,"page_size":20,"total":1}`))
		case "GET /plugin/v1/records/record-1":
			_, _ = w.Write([]byte(`{"record_id":"record-1","source_type":"source_task","source_id":"task-1","status":"synced"}`))
		case "GET /plugin/v1/metrics":
			_, _ = w.Write([]byte(`{"values":{"runs_total":3,"records_total":10}}`))
		case "GET /plugin/v1/configuration":
			_, _ = w.Write([]byte(`{"mode":"setup_pending","version":0,"settings":{},"secrets":{}}`))
		case "PUT /plugin/v1/configuration":
			_, _ = w.Write([]byte(`{"mode":"ready","version":1,"restart_required":true,"settings":{"source_base_url":"https://tracker.example.test"},"secrets":{"source_password":true}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewHTTPClient()
	ctx := context.Background()
	manifest, err := client.Manifest(ctx, server.URL)
	if err != nil || manifest.ContractVersion != "1.0" || manifest.Kind != "state_source_sync" {
		t.Fatalf("manifest: %#v %v", manifest, err)
	}
	health, err := client.Health(ctx, server.URL)
	if err != nil || health.Status != "healthy" {
		t.Fatalf("health: %#v %v", health, err)
	}
	run, err := client.TriggerRun(ctx, server.URL, "request-1", "")
	if err != nil || run.RunID != "sync-run-1" || triggerRequestID != "request-1" {
		t.Fatalf("trigger: %#v request=%q err=%v", run, triggerRequestID, err)
	}
	runs, err := client.ListRuns(ctx, server.URL, PageRequest{Page: 2, PageSize: 10})
	if err != nil || runs.Total != 1 || runs.Items[0].RunID != "sync-run-1" {
		t.Fatalf("runs: %#v %v", runs, err)
	}
	if run, err := client.GetRun(ctx, server.URL, "sync-run-1"); err != nil || run.Status != "succeeded" {
		t.Fatalf("get run: %#v %v", run, err)
	}
	records, err := client.ListRecords(ctx, server.URL, PageRequest{})
	if err != nil || records.Total != 1 || records.Items[0].RecordID != "record-1" {
		t.Fatalf("records: %#v %v", records, err)
	}
	if record, err := client.GetRecord(ctx, server.URL, "record-1"); err != nil || record.Status != "synced" {
		t.Fatalf("get record: %#v %v", record, err)
	}
	metrics, err := client.Metrics(ctx, server.URL, "")
	if err != nil || metrics.Values["runs_total"] != 3 {
		t.Fatalf("metrics: %#v %v", metrics, err)
	}
	configuration, err := client.Configuration(ctx, server.URL)
	if err != nil || configuration.Mode != "setup_pending" {
		t.Fatalf("configuration: %#v %v", configuration, err)
	}
	saved, err := client.SaveConfiguration(ctx, server.URL, SaveConfigurationRequest{
		Settings:        map[string]any{"source_base_url": "https://tracker.example.test"},
		PreserveSecrets: []string{"source_password"},
	})
	if err != nil || !saved.RestartRequired || saved.Version != 1 {
		t.Fatalf("saved configuration: %#v %v", saved, err)
	}
	if saved.Settings["source_base_url"] != "https://tracker.example.test" {
		t.Fatalf("saved configuration settings mismatch: %#v", saved.Settings)
	}
}

func TestHTTPClientSaveConfigurationOpaqueSettingsRoundTrip(t *testing.T) {
	var receivedSettings map[string]any
	var receivedPreserve []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/plugin/v1/configuration" {
			http.NotFound(w, r)
			return
		}
		var body struct {
			Settings        map[string]any `json:"settings"`
			PreserveSecrets []string       `json:"preserve_secrets"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		receivedSettings = body.Settings
		receivedPreserve = body.PreserveSecrets
		_, _ = w.Write([]byte(`{"mode":"ready","version":3,"settings":{"custom_field":"v"},"secret_fields":["api_key"]}`))
	}))
	defer server.Close()

	client := NewHTTPClient()
	result, err := client.SaveConfiguration(context.Background(), server.URL, SaveConfigurationRequest{
		Settings:        map[string]any{"custom_field": "v", "nested": map[string]any{"deep": true}},
		PreserveSecrets: []string{"api_key"},
	})
	if err != nil {
		t.Fatalf("save configuration: %v", err)
	}
	if receivedSettings["custom_field"] != "v" {
		t.Fatalf("forwarded settings mismatch: %#v", receivedSettings)
	}
	if len(receivedPreserve) != 1 || receivedPreserve[0] != "api_key" {
		t.Fatalf("forwarded preserve_secrets mismatch: %#v", receivedPreserve)
	}
	if result.Settings["custom_field"] != "v" {
		t.Fatalf("result settings mismatch: %#v", result.Settings)
	}
	if len(result.SecretFields) != 1 || result.SecretFields[0] != "api_key" {
		t.Fatalf("result secret_fields mismatch: %#v", result.SecretFields)
	}
}

func TestHTTPClientNormalizesRemoteFailures(t *testing.T) {
	tests := []struct {
		name     string
		handler  http.Handler
		option   HTTPClientOption
		expected RemoteErrorKind
	}{
		{
			name: "invalid json",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"status":`))
			}),
			expected: RemoteErrorContract,
		},
		{
			name: "remote error",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "failed", http.StatusInternalServerError)
			}),
			expected: RemoteErrorRemote,
		},
		{
			name: "body too large",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(strings.Repeat("x", 128)))
			}),
			option:   WithResponseLimit(32),
			expected: RemoteErrorContract,
		},
		{
			name: "timeout",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				<-r.Context().Done()
			}),
			option:   WithRequestTimeouts(10*time.Millisecond, 10*time.Millisecond),
			expected: RemoteErrorTimeout,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(test.handler)
			defer server.Close()
			options := []HTTPClientOption{}
			if test.option != nil {
				options = append(options, test.option)
			}
			_, err := NewHTTPClient(options...).Health(context.Background(), server.URL)
			var remoteError *RemoteError
			if !errors.As(err, &remoteError) || remoteError.Kind != test.expected {
				t.Fatalf("expected %s, got %T %v", test.expected, err, err)
			}
		})
	}
}

func TestHTTPClientRejectsRedirects(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()

	_, err := NewHTTPClient().Health(context.Background(), redirect.URL)
	var remoteError *RemoteError
	if !errors.As(err, &remoteError) || remoteError.Kind != RemoteErrorRemote {
		t.Fatalf("expected redirect remote error, got %T %v", err, err)
	}
}

func TestNewHTTPClientUsesFiveMinuteTriggerTimeout(t *testing.T) {
	client := NewHTTPClient()
	if client.triggerTimeout != 5*time.Minute {
		t.Fatalf("default trigger timeout = %s, want 5m", client.triggerTimeout)
	}
}

func TestHTTPClientPutGetDeleteRepository(t *testing.T) {
	var putBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "PUT /plugin/v1/repositories/repo_a":
			if err := json.NewDecoder(r.Body).Decode(&putBody); err != nil {
				t.Fatalf("decode put: %v", err)
			}
			_, _ = w.Write([]byte(`{"repository_id":"repo_a","config_revision":3,"enabled":true,"settings":{"gitlab_project_id":"24207","path_with_namespace":"group/a"}}`))
		case "GET /plugin/v1/repositories/repo_a":
			_, _ = w.Write([]byte(`{"repository_id":"repo_a","config_revision":3,"enabled":true,"settings":{"gitlab_project_id":"24207","path_with_namespace":"group/a"}}`))
		case "DELETE /plugin/v1/repositories/repo_a":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewHTTPClient()
	ctx := context.Background()
	saved, err := client.PutRepository(ctx, server.URL, "repo_a", SaveRepositoryRequest{
		RepositoryID:   "repo_a",
		ConfigRevision: 3,
		Enabled:        true,
		Settings: map[string]any{
			"gitlab_project_id":   "24207",
			"path_with_namespace": "group/a",
			"token":               "super-secret-token",
		},
	})
	if err != nil || saved.ConfigRevision != 3 {
		t.Fatalf("put: %#v %v", saved, err)
	}
	if got, ok := saved.Settings["gitlab_project_id"]; !ok || got != "24207" {
		t.Fatalf("put settings mismatch: %#v", saved.Settings)
	}
	got, err := client.GetRepository(ctx, server.URL, "repo_a")
	if err != nil {
		t.Fatalf("get: %#v %v", got, err)
	}
	if err := client.DeleteRepository(ctx, server.URL, "repo_a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestHTTPClientPutRepositoryMapsConnectorErrorCodes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		expected error
	}{
		{"duplicate resource", http.StatusConflict, `{"error":{"code":"duplicate_resource","message":"resource already exists"}}`, ErrDuplicateResource},
		{"connector never configured", http.StatusConflict, `{"error":{"code":"configuration_required","message":"connector configuration is required"}}`, ErrConnectorNotConfigured},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			_, err := NewHTTPClient().PutRepository(context.Background(), server.URL, "repo_a", SaveRepositoryRequest{RepositoryID: "repo_a"})
			if !errors.Is(err, tc.expected) {
				t.Fatalf("got %v, want %v", err, tc.expected)
			}
			if strings.Contains(err.Error(), "connector instance has no") || strings.Contains(err.Error(), "configuration is required") {
				t.Fatalf("remote message leaked into error text: %v", err)
			}
		})
	}
}

func TestHTTPClientPutRepositoryMaps409(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"code":"duplicate_resource"}}`, http.StatusConflict)
	}))
	defer server.Close()

	_, err := NewHTTPClient().PutRepository(context.Background(), server.URL, "repo_a", SaveRepositoryRequest{
		RepositoryID: "repo_a",
	})
	if !errors.Is(err, ErrDuplicateResource) {
		t.Fatalf("expected duplicate_resource, got %v", err)
	}
}

func TestHTTPClientPutRepositoryRejectsBodyIDMismatch(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"repository_id":"other","gitlab_project_id":"1","path_with_namespace":"g/r","config_revision":1}`))
	}))
	defer server.Close()

	_, err := NewHTTPClient().PutRepository(context.Background(), server.URL, "repo_a", SaveRepositoryRequest{
		RepositoryID: "repo_other",
	})
	if !errors.Is(err, ErrRepositoryIDMismatch) {
		t.Fatalf("expected id mismatch, got %v", err)
	}
	if called {
		t.Fatal("mismatched body id must not be sent")
	}
}

func TestHTTPClientDoesNotExposeRemoteErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `database failed token="secret-value"`, http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := NewHTTPClient().Health(context.Background(), server.URL)
	var remoteError *RemoteError
	if !errors.As(err, &remoteError) || remoteError.Kind != RemoteErrorRemote {
		t.Fatalf("expected remote error, got %T %v", err, err)
	}
	if strings.Contains(err.Error(), "secret-value") || strings.Contains(err.Error(), "database failed") {
		t.Fatalf("remote response body leaked through error: %v", err)
	}
	if !strings.Contains(err.Error(), "Internal Server Error") {
		t.Fatalf("expected bounded HTTP status summary, got %v", err)
	}
}
