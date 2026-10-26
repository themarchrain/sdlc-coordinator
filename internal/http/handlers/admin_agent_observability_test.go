package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	agentobservability "sdlc-coordinator/internal/observability/agent"
	"sdlc-coordinator/internal/store"

	"github.com/gin-gonic/gin"
)

func TestAdminAgentObservabilityRoutesExposeSummaryListAndDetail(t *testing.T) {
	router := newAdminAgentObservabilityTestRouter(t)

	summary := performAdminAgentRequest(router, "/api/admin/agents/executions/summary")
	if summary.Code != http.StatusOK {
		t.Fatalf("summary status: %d %s", summary.Code, summary.Body.String())
	}
	var summaryBody map[string]any
	decodeAdminAgentJSON(t, summary, &summaryBody)
	// claude 的 cache_read 折入输入(子集口径):200+150;codex input 已含缓存。
	if summaryBody["total_executions"] != float64(2) || summaryBody["input_tokens"] != float64(392) || summaryBody["turn_count"] != float64(2) {
		t.Fatalf("unexpected summary without provider filter: %#v", summaryBody)
	}

	list := performAdminAgentRequest(router, "/api/admin/agents/executions/executions?page_size=10")
	if list.Code != http.StatusOK {
		t.Fatalf("list status: %d %s", list.Code, list.Body.String())
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int64            `json:"total"`
	}
	decodeAdminAgentJSON(t, list, &page)
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("unexpected unfiltered page: %#v", page)
	}

	codexList := performAdminAgentRequest(router, "/api/admin/agents/executions/executions?provider=codex-cli&page_size=10")
	decodeAdminAgentJSON(t, codexList, &page)
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0]["task_id"] != "task-codex-api" || page.Items[0]["model"] != "gpt-alpha" || page.Items[0]["provider"] != "codex-cli" {
		t.Fatalf("unexpected codex-filtered page: %#v", page)
	}

	claudeList := performAdminAgentRequest(router, "/api/admin/agents/executions/executions?provider=claude-cli&page_size=10")
	decodeAdminAgentJSON(t, claudeList, &page)
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0]["execution_id"] != "run-claude-api" || page.Items[0]["provider"] != "claude-cli" {
		t.Fatalf("unexpected claude-filtered page: %#v", page)
	}

	unknownList := performAdminAgentRequest(router, "/api/admin/agents/executions/executions?provider=some-other-cli&page_size=10")
	decodeAdminAgentJSON(t, unknownList, &page)
	if page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("unknown provider must yield an empty set: %#v", page)
	}

	detail := performAdminAgentRequest(router, "/api/admin/agents/executions/executions/run-claude-api")
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status: %d %s", detail.Code, detail.Body.String())
	}
	var detailBody map[string]any
	decodeAdminAgentJSON(t, detail, &detailBody)
	if detailBody["execution_id"] != "run-claude-api" || detailBody["provider"] != "claude-cli" || len(detailBody["events"].([]any)) != 1 {
		t.Fatalf("unexpected detail: %#v", detailBody)
	}
}

func TestAdminAgentObservabilityValidatesQueriesAndRetiresCodexRoutes(t *testing.T) {
	router := newAdminAgentObservabilityTestRouter(t)
	if response := performAdminAgentRequest(router, "/api/admin/agents/executions/executions?page=bad"); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid query status: %d %s", response.Code, response.Body.String())
	}
	if response := performAdminAgentRequest(router, "/api/admin/agents/executions/executions/missing"); response.Code != http.StatusNotFound {
		t.Fatalf("missing detail status: %d %s", response.Code, response.Body.String())
	}
	for _, retired := range []string{"/api/admin/agents/codex/summary", "/api/admin/agents/codex/executions", "/api/admin/agents/codex/executions/run-codex-api"} {
		if response := performAdminAgentRequest(router, retired); response.Code != http.StatusNotFound {
			t.Fatalf("retired route %s must be gone, got %d", retired, response.Code)
		}
	}
}

func newAdminAgentObservabilityTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.StopRequest{}, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.AgentExecutionEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	started := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)
	finished := started.Add(30 * time.Second)
	entities := []any{
		&model.Task{TaskID: "task-codex-api", DedupeKey: "task-codex-api", Type: "review_mr", Role: "ReviewerWorker", TargetRepo: "group/repo", Status: model.TaskStatusSucceeded},
		&model.Run{RunID: "run-codex-api", TaskID: "task-codex-api", WorkerRole: "ReviewerWorker", Status: model.RunStatusSucceeded},
		&model.AgentExecution{RunID: "run-codex-api", ExecutionID: "run-codex-api", Provider: "codex-cli", ProviderModel: "gpt-alpha", ProviderEffort: "high", Status: model.AgentExecutionStatusSucceeded, RequestHash: "hash", StartedAt: &started, FinishedAt: &finished},
		&model.AgentExecutionEvent{ExecutionID: "run-codex-api", RunID: "run-codex-api", Provider: "codex-cli", Seq: 1, Type: "codex_usage", OccurredAt: finished, PayloadJSON: `{"input_tokens":42,"cached_input_tokens":30,"output_tokens":8}`},
		&model.Task{TaskID: "task-claude-api", DedupeKey: "task-claude-api", Type: "develop_from_issue", Role: "DeveloperWorker", TargetRepo: "group/repo", Status: model.TaskStatusSucceeded},
		&model.Run{RunID: "run-claude-api", TaskID: "task-claude-api", WorkerRole: "DeveloperWorker", Status: model.RunStatusSucceeded},
		&model.AgentExecution{RunID: "run-claude-api", ExecutionID: "run-claude-api", Provider: "claude-cli", ProviderModel: "claude-alpha", Status: model.AgentExecutionStatusSucceeded, RequestHash: "hash-claude", StartedAt: &started, FinishedAt: &finished},
		&model.AgentExecutionEvent{ExecutionID: "run-claude-api", RunID: "run-claude-api", Provider: "claude-cli", Seq: 1, Type: "claude_usage", OccurredAt: finished, PayloadJSON: `{"model":"claude-alpha","usage":{"input_tokens":200,"cache_read_input_tokens":150,"output_tokens":40}}`},
	}
	for _, entity := range entities {
		if err := db.Create(entity).Error; err != nil {
			t.Fatalf("seed %T: %v", entity, err)
		}
	}
	router := gin.New()
	NewAdminAgentObservabilityHandler(agentobservability.NewQuery(db)).RegisterRoutes(router.Group("/api/admin"))
	return router
}

func performAdminAgentRequest(router http.Handler, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

func decodeAdminAgentJSON(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatalf("decode %q: %v", recorder.Body.String(), err)
	}
}
