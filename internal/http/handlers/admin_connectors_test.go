package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sdlc-coordinator/internal/connectors"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"

	"github.com/gin-gonic/gin"
)

func TestAdminConnectorCRUDContract(t *testing.T) {
	router, _ := newAdminConnectorHandlerTestRouter(t, connectors.NewHTTPClient())

	createdResponse := performAdminConnectorJSON(t, router, http.MethodPost, "/api/admin/connectors", map[string]any{
		"kind": "state_source_sync", "display_name": "Task Sync", "base_url": "http://task-sync:8989/",
		"target_project_path": "delper-ai/delper-agent",
	})
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create status: %d %s", createdResponse.Code, createdResponse.Body.String())
	}
	var created map[string]any
	decodeAdminConnectorJSON(t, createdResponse, &created)
	connectorID, _ := created["connector_id"].(string)
	if connectorID == "" || created["base_url"] != "http://task-sync:8989" || created["enabled"] != true {
		t.Fatalf("unexpected create response: %#v", created)
	}

	list := performAdminConnectorJSON(t, router, http.MethodGet, "/api/admin/connectors?kind=state_source_sync&enabled=true", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list status: %d %s", list.Code, list.Body.String())
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int64            `json:"total"`
	}
	decodeAdminConnectorJSON(t, list, &page)
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0]["connector_id"] != connectorID {
		t.Fatalf("unexpected list: %#v", page)
	}

	patch := performAdminConnectorJSON(t, router, http.MethodPatch, "/api/admin/connectors/"+connectorID, map[string]any{
		"display_name": "Renamed Sync", "enabled": false,
	})
	if patch.Code != http.StatusOK {
		t.Fatalf("patch status: %d %s", patch.Code, patch.Body.String())
	}
	var patched map[string]any
	decodeAdminConnectorJSON(t, patch, &patched)
	if patched["display_name"] != "Renamed Sync" || patched["enabled"] != false {
		t.Fatalf("unexpected patch: %#v", patched)
	}

	detail := performAdminConnectorJSON(t, router, http.MethodGet, "/api/admin/connectors/"+connectorID, nil)
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status: %d %s", detail.Code, detail.Body.String())
	}

	deleted := performAdminConnectorJSON(t, router, http.MethodDelete, "/api/admin/connectors/"+connectorID, nil)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status: %d %s", deleted.Code, deleted.Body.String())
	}
	missing := performAdminConnectorJSON(t, router, http.MethodGet, "/api/admin/connectors/"+connectorID, nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status: %d %s", missing.Code, missing.Body.String())
	}
}

func TestAdminConnectorsAllowMultipleEnabledInstances(t *testing.T) {
	router, _ := newAdminConnectorHandlerTestRouter(t, connectors.NewHTTPClient())
	first := performAdminConnectorJSON(t, router, http.MethodPost, "/api/admin/connectors", map[string]any{
		"kind": "state_source_a", "display_name": "Source A", "base_url": "http://sync-a:8989",
	})
	if first.Code != http.StatusCreated {
		t.Fatalf("first %d %s", first.Code, first.Body.String())
	}
	second := performAdminConnectorJSON(t, router, http.MethodPost, "/api/admin/connectors", map[string]any{
		"kind": "state_source_a", "display_name": "Source A 2", "base_url": "http://sync-b:8989",
	})
	if second.Code != http.StatusCreated {
		t.Fatalf("second enabled instance must be allowed: %d %s", second.Code, second.Body.String())
	}
}

func TestAdminConnectorProxiesCanonicalRemoteOperations(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /plugin/v1/manifest":
			_, _ = w.Write([]byte(`{"contract_version":"1.0","kind":"state_source_sync","name":"Task Sync","version":"0.1.0","capabilities":["manual_trigger","run_history","record_query","metrics","deep_health"]}`))
		case "GET /plugin/v1/health":
			_, _ = w.Write([]byte(`{"status":"healthy","message":"ready"}`))
		case "POST /plugin/v1/runs":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"run_id":"remote-run-1","trigger":"manual","status":"running"}`))
		case "GET /plugin/v1/runs":
			_, _ = w.Write([]byte(`{"items":[{"run_id":"remote-run-1","trigger":"manual","status":"succeeded"}],"page":1,"page_size":20,"total":1}`))
		case "GET /plugin/v1/runs/remote-run-1":
			_, _ = w.Write([]byte(`{"run_id":"remote-run-1","trigger":"manual","status":"succeeded"}`))
		case "GET /plugin/v1/records":
			_, _ = w.Write([]byte(`{"items":[{"record_id":"record-1","source_type":"source_task","source_id":"task-1","status":"synced"}],"page":1,"page_size":20,"total":1}`))
		case "GET /plugin/v1/records/record-1":
			_, _ = w.Write([]byte(`{"record_id":"record-1","source_type":"source_task","source_id":"task-1","status":"synced"}`))
		case "GET /plugin/v1/metrics":
			_, _ = w.Write([]byte(`{"values":{"runs_total":1}}`))
		case "GET /plugin/v1/configuration":
			_, _ = w.Write([]byte(`{"mode":"setup_pending","version":0,"settings":{},"secrets":{}}`))
		case "PUT /plugin/v1/configuration":
			_, _ = w.Write([]byte(`{"mode":"ready","version":1,"restart_required":true,"settings":{},"secrets":{"source_password":true,"gitlab_token":true}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()

	router, service := newAdminConnectorHandlerTestRouter(t, connectors.NewHTTPClient())
	connector, err := service.Create(context.Background(), connectors.CreateConnectorRequest{
		Kind: "state_source_sync", DisplayName: "State Source", BaseURL: remote.URL, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create connector: %v", err)
	}
	basePath := "/api/admin/connectors/" + connector.ConnectorID

	tests := []struct {
		method string
		path   string
		status int
		key    string
	}{
		{method: http.MethodPost, path: basePath + "/probe", status: http.StatusOK, key: "manifest"},
		{method: http.MethodPost, path: basePath + "/runs", status: http.StatusAccepted, key: "request_id"},
		{method: http.MethodGet, path: basePath + "/runs", status: http.StatusOK, key: "items"},
		{method: http.MethodGet, path: basePath + "/runs/remote-run-1", status: http.StatusOK, key: "run_id"},
		{method: http.MethodGet, path: basePath + "/records", status: http.StatusOK, key: "items"},
		{method: http.MethodGet, path: basePath + "/records/record-1", status: http.StatusOK, key: "record_id"},
		{method: http.MethodGet, path: basePath + "/metrics", status: http.StatusOK, key: "values"},
		{method: http.MethodGet, path: basePath + "/configuration", status: http.StatusOK, key: "mode"},
		{method: http.MethodPut, path: basePath + "/configuration", status: http.StatusOK, key: "restart_required"},
	}
	for _, test := range tests {
		var payload any
		if test.method == http.MethodPut {
			payload = map[string]any{"settings": map[string]any{}, "preserve_secrets": []string{}}
		}
		response := performAdminConnectorJSON(t, router, test.method, test.path, payload)
		if response.Code != test.status {
			t.Fatalf("%s %s: %d %s", test.method, test.path, response.Code, response.Body.String())
		}
		var body map[string]any
		decodeAdminConnectorJSON(t, response, &body)
		if _, exists := body[test.key]; !exists {
			t.Fatalf("%s %s omitted %s: %#v", test.method, test.path, test.key, body)
		}
	}
}

func newAdminConnectorHandlerTestRouter(t *testing.T, remote connectors.RemoteClient) (*gin.Engine, *connectors.Service) {
	t.Helper()
	dsnName := strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(t.Name())
	db, err := store.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", dsnName))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.ConnectorInstance{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	service := connectors.NewService(connectors.NewConnectorRepository(db), remote)
	router := gin.New()
	NewAdminConnectorHandler(service).RegisterRoutes(router.Group("/api/admin"))
	return router, service
}

func performAdminConnectorJSON(t *testing.T, router http.Handler, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			t.Fatalf("encode request: %v", err)
		}
	}
	request := httptest.NewRequest(method, path, &body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func decodeAdminConnectorJSON(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatalf("decode %q: %v", recorder.Body.String(), err)
	}
}
