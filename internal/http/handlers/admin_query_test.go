package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sdlc-coordinator/internal/admin"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestAdminTaskListAndDetailContract(t *testing.T) {
	router, db := newAdminQueryHandlerTestRouter(t)
	seedAdminQueryTask(t, db)

	list := performAdminQueryRequest(router, "/api/admin/tasks?page=1&page_size=10&status=running")
	if list.Code != http.StatusOK {
		t.Fatalf("list status: %d %s", list.Code, list.Body.String())
	}
	var page struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int64            `json:"total"`
	}
	decodeAdminQueryJSON(t, list, &page)
	if page.Page != 1 || page.PageSize != 10 || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("unexpected page: %#v", page)
	}
	if page.Items[0]["task_id"] != "task-admin" {
		t.Fatalf("missing snake_case task id: %#v", page.Items[0])
	}
	if _, exists := page.Items[0]["context_json"]; exists {
		t.Fatalf("list leaked context_json: %#v", page.Items[0])
	}

	detail := performAdminQueryRequest(router, "/api/admin/tasks/task-admin")
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status: %d %s", detail.Code, detail.Body.String())
	}
	var task map[string]any
	decodeAdminQueryJSON(t, detail, &task)
	if task["task_id"] != "task-admin" || task["context_json"] != `{"issue_iid":1}` {
		t.Fatalf("unexpected task detail: %#v", task)
	}
}

func TestAdminTaskQueryRejectsInvalidPageAndReturnsNotFound(t *testing.T) {
	router, _ := newAdminQueryHandlerTestRouter(t)

	invalid := performAdminQueryRequest(router, "/api/admin/tasks?page=invalid")
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid query status: %d %s", invalid.Code, invalid.Body.String())
	}
	var invalidBody map[string]any
	decodeAdminQueryJSON(t, invalid, &invalidBody)
	errorBody, _ := invalidBody["error"].(map[string]any)
	if errorBody["code"] != "invalid_query" {
		t.Fatalf("unexpected invalid query body: %#v", invalidBody)
	}

	missing := performAdminQueryRequest(router, "/api/admin/tasks/missing")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status: %d %s", missing.Code, missing.Body.String())
	}
}

func TestAdminTaskQueryRejectsInvalidTime(t *testing.T) {
	router, _ := newAdminQueryHandlerTestRouter(t)

	response := performAdminQueryRequest(router, "/api/admin/tasks?created_from=not-a-time")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid time status: %d %s", response.Code, response.Body.String())
	}
}

func TestAdminQueryExposesEveryPersistedEntity(t *testing.T) {
	router, db := newAdminQueryHandlerTestRouter(t)
	seedAdminQueryTask(t, db)
	seedAdminQueryEntities(t, db)

	tests := []struct {
		name           string
		listPath       string
		detailPath     string
		idKey          string
		idValue        string
		largeDetailKey string
	}{
		{name: "runs", listPath: "/api/admin/runs", detailPath: "/api/admin/runs/run-admin", idKey: "run_id", idValue: "run-admin", largeDetailKey: "input_json"},
		{name: "artifacts", listPath: "/api/admin/artifacts", detailPath: "/api/admin/artifacts/artifact-admin", idKey: "artifact_id", idValue: "artifact-admin", largeDetailKey: "metadata_json"},
		{name: "dependencies", listPath: "/api/admin/dependencies", detailPath: "/api/admin/dependencies/dependency-admin", idKey: "dependency_id", idValue: "dependency-admin", largeDetailKey: "metadata_json"},
		{name: "writebacks", listPath: "/api/admin/writebacks", detailPath: "/api/admin/writebacks/writeback-admin", idKey: "writeback_id", idValue: "writeback-admin", largeDetailKey: "request_json"},
		{name: "effects", listPath: "/api/admin/effects", detailPath: "/api/admin/effects/effect-admin", idKey: "effect_id", idValue: "effect-admin", largeDetailKey: "request_json"},
		{name: "inbound events", listPath: "/api/admin/inbound-events", detailPath: "/api/admin/inbound-events/event-admin", idKey: "event_id", idValue: "event-admin", largeDetailKey: "payload_json"},
		{name: "result submissions", listPath: "/api/admin/result-submissions", detailPath: "/api/admin/result-submissions/submission-admin", idKey: "submission_id", idValue: "submission-admin", largeDetailKey: "raw_payload_json"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			list := performAdminQueryRequest(router, test.listPath+"?page_size=5")
			if list.Code != http.StatusOK {
				t.Fatalf("list status: %d %s", list.Code, list.Body.String())
			}
			var page struct {
				Items []map[string]any `json:"items"`
				Total int64            `json:"total"`
			}
			decodeAdminQueryJSON(t, list, &page)
			if page.Total != 1 || len(page.Items) != 1 || page.Items[0][test.idKey] != test.idValue {
				t.Fatalf("unexpected list body: %#v", page)
			}
			if _, exists := page.Items[0][test.largeDetailKey]; exists {
				t.Fatalf("list leaked %s: %#v", test.largeDetailKey, page.Items[0])
			}

			detail := performAdminQueryRequest(router, test.detailPath)
			if detail.Code != http.StatusOK {
				t.Fatalf("detail status: %d %s", detail.Code, detail.Body.String())
			}
			var body map[string]any
			decodeAdminQueryJSON(t, detail, &body)
			if body[test.idKey] != test.idValue {
				t.Fatalf("unexpected detail id: %#v", body)
			}
			if _, exists := body[test.largeDetailKey]; !exists {
				t.Fatalf("detail omitted %s: %#v", test.largeDetailKey, body)
			}
		})
	}
}

func TestAdminQueryExposesOverviewAndTaskTimeline(t *testing.T) {
	router, db := newAdminQueryHandlerTestRouter(t)
	seedAdminQueryTask(t, db)
	seedAdminQueryEntities(t, db)

	overview := performAdminQueryRequest(router, "/api/admin/overview?window=all")
	if overview.Code != http.StatusOK {
		t.Fatalf("overview status: %d %s", overview.Code, overview.Body.String())
	}
	var overviewBody map[string]any
	decodeAdminQueryJSON(t, overview, &overviewBody)
	if _, exists := overviewBody["tasks_by_status"]; !exists {
		t.Fatalf("overview omitted task counts: %#v", overviewBody)
	}
	business, ok := overviewBody["business_summary"].(map[string]any)
	if !ok || business["window"] != "all" || business["merge_requests_created"] != float64(1) {
		t.Fatalf("overview omitted business summary: %#v", overviewBody)
	}

	timeline := performAdminQueryRequest(router, "/api/admin/tasks/task-admin/timeline")
	if timeline.Code != http.StatusOK {
		t.Fatalf("timeline status: %d %s", timeline.Code, timeline.Body.String())
	}
	var timelineBody struct {
		Items []map[string]any `json:"items"`
	}
	decodeAdminQueryJSON(t, timeline, &timelineBody)
	if len(timelineBody.Items) < 8 {
		t.Fatalf("timeline omitted related entities: %#v", timelineBody.Items)
	}
}

func TestAdminQueryMetricsUsesDefaultWindowAndValidatesInput(t *testing.T) {
	router, db := newAdminQueryHandlerTestRouter(t)
	now := time.Now().UTC().Add(-time.Minute)
	task := &model.Task{
		TaskID: "task-metrics", DedupeKey: "task-metrics", Type: "develop_from_issue",
		Role: "DeveloperWorker", Status: model.TaskStatusSucceeded, CreatedAt: now, UpdatedAt: now,
	}
	completedAt := now.Add(90 * time.Second)
	run := &model.Run{
		RunID: "run-metrics", TaskID: task.TaskID, WorkerRole: "DeveloperWorker", Status: model.RunStatusSucceeded,
		StartedAt: &now, CompletedAt: &completedAt, CreatedAt: now, UpdatedAt: completedAt,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("seed metrics task: %v", err)
	}
	if err := db.Create(run).Error; err != nil {
		t.Fatalf("seed metrics run: %v", err)
	}

	response := performAdminQueryRequest(router, "/api/admin/metrics")
	if response.Code != http.StatusOK {
		t.Fatalf("metrics status: %d %s", response.Code, response.Body.String())
	}
	var body map[string]any
	decodeAdminQueryJSON(t, response, &body)
	if body["window"] != "30d" || body["task_sample_count"] != float64(1) {
		t.Fatalf("unexpected metrics header: %#v", body)
	}
	current := body["current_success"].(map[string]any)
	if current["numerator"] != float64(1) || current["denominator"] != float64(1) || current["value"] != float64(1) {
		t.Fatalf("unexpected current success: %#v", current)
	}
	durations := body["successful_run_durations"].([]any)
	if len(durations) != 1 || durations[0].(map[string]any)["worker_role"] != "DeveloperWorker" {
		t.Fatalf("unexpected duration metrics: %#v", durations)
	}

	filtered := performAdminQueryRequest(router, "/api/admin/metrics?window=all&type=review_mr")
	if filtered.Code != http.StatusOK {
		t.Fatalf("filtered metrics status: %d %s", filtered.Code, filtered.Body.String())
	}
	decodeAdminQueryJSON(t, filtered, &body)
	if body["window"] != "all" || body["type"] != "review_mr" || body["task_sample_count"] != float64(0) {
		t.Fatalf("unexpected filtered metrics: %#v", body)
	}

	invalid := performAdminQueryRequest(router, "/api/admin/metrics?window=90d")
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid window status: %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestAdminQueryUnknownRepositoryIDReturns404(t *testing.T) {
	router, db := newAdminQueryHandlerTestRouter(t)
	if err := registeredrepo.NewStore(db).Migrate(); err != nil {
		t.Fatalf("migrate registered repositories: %v", err)
	}
	seedAdminQueryTask(t, db)

	for _, path := range []string{
		"/api/admin/tasks?repository_id=missing",
		"/api/admin/overview?repository_id=missing",
		"/api/admin/runs?repository_id=missing",
		"/api/admin/inbound-events?repository_id=missing",
	} {
		response := performAdminQueryRequest(router, path)
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s status: %d %s", path, response.Code, response.Body.String())
		}
		if !strings.Contains(response.Body.String(), "repository_not_found") {
			t.Fatalf("%s body: %s", path, response.Body.String())
		}
	}

	listed := performAdminQueryRequest(router, "/api/admin/tasks")
	if listed.Code != http.StatusOK {
		t.Fatalf("unscoped list status: %d %s", listed.Code, listed.Body.String())
	}
}

func newAdminQueryHandlerTestRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()

	dsnName := strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(t.Name())
	db, err := store.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", dsnName))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(
		db,
		&model.Task{},
		&model.Run{},
		&model.Artifact{},
		&model.Dependency{},
		&model.Writeback{},
		&model.Effect{},
		&model.InboundEvent{},
		&model.ResultSubmission{},
		&model.ResultSubmissionRejection{},
	); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	router := gin.New()
	handler := NewAdminQueryHandler(admin.NewQueryService(db))
	handler.RegisterRoutes(router.Group("/api/admin"))
	return router, db
}

func seedAdminQueryTask(t *testing.T, db *gorm.DB) {
	t.Helper()
	task := &model.Task{
		TaskID:             "task-admin",
		DedupeKey:          "task-admin",
		Type:               "develop_from_issue",
		Role:               "DeveloperWorker",
		SourceRepo:         "group/repo",
		TargetRepo:         "group/repo",
		Objective:          "fix issue",
		AcceptanceCriteria: "tests pass",
		Status:             model.TaskStatusRunning,
		ContextJSON:        `{"issue_iid":1}`,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("seed task: %v", err)
	}
}

func seedAdminQueryEntities(t *testing.T, db *gorm.DB) {
	t.Helper()
	entities := []any{
		&model.Run{
			RunID: "run-admin", TaskID: "task-admin", WorkerRole: "DeveloperWorker",
			Status: model.RunStatusRunning, InputJSON: `{"objective":"fix"}`,
		},
		&model.Artifact{
			ArtifactID: "artifact-admin", TaskID: "task-admin", RunID: "run-admin",
			Kind: "diff", Name: "change.patch", MetadataJSON: `{"files":1}`,
		},
		&model.Dependency{
			DependencyID: "dependency-admin", ParentTaskID: "task-admin", ChildTaskID: "task-child",
			Relation: "upstream_fix", Status: model.DependencyStatusResolved, MetadataJSON: `{"reason":"fixed"}`,
		},
		&model.Writeback{
			WritebackID: "writeback-admin", TaskID: "task-admin", RunID: "run-admin",
			DedupeKey: "writeback-admin", Kind: "create_merge_request", ProjectID: "group/repo",
			Status: model.WritebackStatusSucceeded, RequestJSON: `{"title":"MR"}`,
		},
		&model.Effect{
			EffectID: "effect-admin", TaskID: "task-admin", RunID: "run-admin",
			DedupeKey: "effect-admin", Kind: "send_im_message", Target: "default",
			Status: model.EffectStatusSucceeded, RequestJSON: `{"text":"done"}`,
		},
		&model.InboundEvent{
			EventID: "event-admin", DedupeKey: "event-admin", Source: "gitlab", EventType: "issue",
			Status: model.InboundEventStatusTaskCreated, PayloadJSON: `{"object_kind":"issue"}`,
			NormalizedJSON: `{"task_id":"task-admin"}`,
		},
		&model.ResultSubmission{
			SubmissionID: "submission-admin", TaskID: "task-admin", RunID: "run-admin", Source: "mcp",
			ToolName: "submit_developer_result", Role: "DeveloperWorker", PayloadHash: "hash-admin",
			RawPayloadJSON: `{"status":"succeeded"}`, CanonicalJSON: `{"status":"succeeded"}`,
			Status: model.ResultSubmissionStatusSucceeded,
		},
	}
	for _, entity := range entities {
		if err := db.Create(entity).Error; err != nil {
			t.Fatalf("seed entity %T: %v", entity, err)
		}
	}
}

func performAdminQueryRequest(router http.Handler, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	router.ServeHTTP(recorder, request)
	return recorder
}

func decodeAdminQueryJSON(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response %q: %v", recorder.Body.String(), err)
	}
}
