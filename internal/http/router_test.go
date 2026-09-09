package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"sdlc-coordinator/internal/admin"
	"sdlc-coordinator/internal/configuration"
	"sdlc-coordinator/internal/connectors"
	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/events"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	adminweb "sdlc-coordinator/web"

	"gorm.io/gorm"
)

func TestHealthz(t *testing.T) {
	router := NewRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %q", body["status"])
	}
}

func TestAdminConfigurationRoutesUseInjectedService(t *testing.T) {
	service := configuration.NewService(nil, configuration.Resolution{Mode: configuration.ModeSetupPending})
	router := NewRouter(nil, WithConfigurationService(service, configuration.NewShutdownSignal()))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/configuration/status", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"mode":"setup_pending"`) {
		t.Fatalf("unexpected configuration status response %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRouterProtectsAdminAPIWhenControlPlaneIsConfigured(t *testing.T) {
	db, err := store.Open("file:router-admin-auth?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	cpRepo := controlplane.NewRepository(db)
	if err := cpRepo.Migrate(); err != nil {
		t.Fatalf("migrate control plane: %v", err)
	}
	configurationService := configuration.NewService(nil, configuration.Resolution{Mode: configuration.ModeSetupPending})
	router := NewRouter(nil,
		WithConfigurationService(configurationService, configuration.NewShutdownSignal()),
		WithControlPlaneService(controlplane.NewService(cpRepo)),
	)

	adminRequest := httptest.NewRequest(http.MethodGet, "/api/admin/configuration/status", nil)
	adminResponse := httptest.NewRecorder()
	router.ServeHTTP(adminResponse, adminRequest)
	if adminResponse.Code != http.StatusUnauthorized {
		t.Fatalf("admin API status = %d, want %d", adminResponse.Code, http.StatusUnauthorized)
	}

	publicRequest := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	publicResponse := httptest.NewRecorder()
	router.ServeHTTP(publicResponse, publicRequest)
	if publicResponse.Code != http.StatusOK {
		t.Fatalf("public health status = %d, want %d", publicResponse.Code, http.StatusOK)
	}
}

func TestAdminConsoleServesSPAEntryAndClientRoutes(t *testing.T) {
	router := NewRouter(nil, WithAdminWebHandler(adminweb.NewHandler(fstest.MapFS{
		"index.html": {Data: []byte(`<!doctype html><div id="root"></div>`)},
	})))
	for _, requestPath := range []string{"/admin", "/admin/", "/admin/tasks", "/admin/tick"} {
		t.Run(requestPath, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, requestPath, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), `<div id="root"></div>`) {
				t.Fatalf("expected React SPA entry, got %q", rec.Body.String())
			}
			if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
				t.Fatalf("Cache-Control = %q, want no-cache", got)
			}
		})
	}
}

func TestLegacyAdminDefaultsRouteIsRemoved(t *testing.T) {
	router := NewRouter(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/tick-defaults", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNotFound, rec.Code, rec.Body.String())
	}
}

func TestLegacyRuntimeRoutesAreRemoved(t *testing.T) {
	router := NewRouter(nil)
	for _, fixture := range []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/mcp"},
		{method: http.MethodPost, path: "/api/tasks/task-1/dispatch"},
		{method: http.MethodPost, path: "/api/runs/run-1/poll"},
		{method: http.MethodPost, path: "/api/runs/run-1/callback"},
	} {
		req := httptest.NewRequest(fixture.method, fixture.path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected removed route %s %s to return 404, got %d: %s", fixture.method, fixture.path, rec.Code, rec.Body.String())
		}
	}
}

func TestAdminTickDefaultsUseInjectedStartupSnapshot(t *testing.T) {
	defaults := orchestrator.TickRequest{
		DispatchLimit:       2,
		PollLimit:           7,
		ReconciliationLimit: 8,
		FinalizationLimit:   9,
		RecoveryLimit:       10,
		EffectLimit:         1,
	}
	router := NewRouter(nil, WithTickDefaults(defaults))
	req := httptest.NewRequest(http.MethodGet, "/api/admin/scheduler/defaults", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var got map[string]any
	decodeJSON(t, rec, &got)
	if got["dispatch_limit"] != float64(2) || got["poll_limit"] != float64(7) || got["effect_limit"] != float64(1) {
		t.Fatalf("unexpected tick limits: %#v", got)
	}
}

func TestAdminQueryRoutesUseInjectedService(t *testing.T) {
	db, err := store.Open("file:router-admin-query?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Artifact{}, &model.Dependency{}, &model.Writeback{}, &model.Effect{}, &model.InboundEvent{}, &model.ResultSubmission{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&model.Task{
		TaskID: "task-router-admin", DedupeKey: "task-router-admin", Type: "test", Role: "TesterWorker", Status: model.TaskStatusPending,
	}).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	router := NewRouter(nil, WithAdminQueryService(admin.NewQueryService(db)))
	req := httptest.NewRequest(http.MethodGet, "/api/admin/tasks", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
}

func TestAdminConnectorRoutesUseInjectedService(t *testing.T) {
	db, err := store.Open("file:router-admin-connectors?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.ConnectorInstance{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	service := connectors.NewService(connectors.NewConnectorRepository(db), connectors.NewHTTPClient())
	router := NewRouter(nil, WithConnectorService(service))
	req := httptest.NewRequest(http.MethodGet, "/api/admin/connectors", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
}

func TestTickParksUnboundTask(t *testing.T) {
	_, orch, _, _, _ := newTestRouter(t)

	created, err := orch.CreateTask(context.Background(), &model.Task{
		DedupeKey:          "scheduler-tick-http-1",
		Type:               "bugfix",
		Role:               "TesterWorker",
		SourceRepo:         "test-automates",
		TargetRepo:         "framework",
		Objective:          "fix framework bug through scheduler tick",
		AcceptanceCriteria: "tests pass",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if _, err := orch.Tick(context.Background(), orchestrator.TickRequest{
		DispatchLimit: 5,
		PollLimit:     5,
		RecoveryLimit: 5,
	}); err != nil {
		t.Fatalf("tick: %v", err)
	}

	// 恒注册仓模式:未绑定注册仓的任务不得派发,被 park 为 blocked 等待 Rebind;
	// 不会再走到"不支持角色 → human_required"的分支。
	stored, err := orch.GetTask(context.Background(), created.TaskID)
	if err != nil {
		t.Fatalf("get parked task: %v", err)
	}
	if stored.Status != model.TaskStatusBlocked || !strings.Contains(stored.LastError, "repository not bound") {
		t.Fatalf("expected unbound task parked as blocked with repository error, got %#v", stored)
	}
}

func TestGitLabWebhookEndpointCreatesTask(t *testing.T) {
	router, taskRepo, _ := newTestRouterWithEventProcessor(t)

	resp := performRawJSONWithHeaders(t, router, http.MethodPost, "/api/events/gitlab", []byte(`{
	  "object_kind": "merge_request",
	  "event_type": "merge_request",
	  "project": {
	    "id": 24207,
	    "path_with_namespace": "group/project",
	    "git_ssh_url": "git@gitlab.example:group/project.git"
	  },
	  "object_attributes": {
	    "iid": 12,
	    "action": "open",
	    "state": "opened",
	    "title": "Improve service",
	    "description": "Review the service change",
	    "source_branch": "feature/demo",
	    "target_branch": "master",
	    "url": "https://gitlab.example/group/project/-/merge_requests/12",
	    "last_commit": {
	      "id": "abc123"
	    }
	  }
	}`), webhookAuthHeaders())
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected gitlab event status %d, got %d: %s", http.StatusAccepted, resp.Code, resp.Body.String())
	}

	var body struct {
		EventID     string           `json:"event_id"`
		EventStatus string           `json:"event_status"`
		Ignored     bool             `json:"ignored"`
		Task        taskHTTPResponse `json:"task"`
	}
	decodeJSON(t, resp, &body)
	if body.EventID == "" {
		t.Fatal("expected event id")
	}
	if body.EventStatus != string(model.InboundEventStatusTaskCreated) {
		t.Fatalf("expected event status %s, got %s", model.InboundEventStatusTaskCreated, body.EventStatus)
	}
	if body.Ignored {
		t.Fatal("expected event not to be ignored")
	}
	if body.Task.TaskID == "" {
		t.Fatal("expected task response")
	}
	if body.Task.Type != "review_mr" || body.Task.Role != "ReviewerWorker" {
		t.Fatalf("unexpected task type/role: %s/%s", body.Task.Type, body.Task.Role)
	}

	task, err := taskRepo.GetByDedupeKey(context.Background(), "gitlab:mr-review:24207:12:abc123")
	if err != nil {
		t.Fatalf("get created task: %v", err)
	}
	if task.TaskID != body.Task.TaskID {
		t.Fatalf("expected task id %s, got %s", body.Task.TaskID, task.TaskID)
	}
}

func TestGitLabWebhookEndpointOmitsTaskForIgnoredEvent(t *testing.T) {
	router, _, _ := newTestRouterWithEventProcessor(t)

	resp := performRawJSONWithHeaders(t, router, http.MethodPost, "/api/events/gitlab", readFixture(t, "../events/testdata/gitlab/merge_request_description_update.json"), webhookAuthHeaders())
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected accepted status %d, got %d: %s", http.StatusAccepted, resp.Code, resp.Body.String())
	}

	var raw map[string]any
	decodeJSON(t, resp, &raw)
	if _, ok := raw["task"]; ok {
		t.Fatalf("expected ignored webhook response to omit task, got %s", resp.Body.String())
	}
	if raw["ignored"] != true {
		t.Fatalf("expected ignored=true, got %#v", raw["ignored"])
	}
}

func TestGitLabWebhookEndpointRejectsMissingTokenWhenConfigured(t *testing.T) {
	router, taskRepo, _ := newTestRouterWithGitLabWebhookSecret(t, "secret-1")

	resp := performRawJSON(t, router, http.MethodPost, "/api/events/gitlab", readFixture(t, "../events/testdata/gitlab/merge_request_opened.json"))
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized status %d, got %d: %s", http.StatusUnauthorized, resp.Code, resp.Body.String())
	}
	assertNoGitLabReviewTask(t, taskRepo, "gitlab:mr-review:15594:2:de51000000000000000000000000000000000000")
}

func TestGitLabWebhookEndpointRejectsWrongTokenWhenConfigured(t *testing.T) {
	router, taskRepo, _ := newTestRouterWithGitLabWebhookSecret(t, "secret-1")

	resp := performRawJSONWithHeaders(t, router, http.MethodPost, "/api/events/gitlab", readFixture(t, "../events/testdata/gitlab/merge_request_opened.json"), map[string]string{
		"X-Gitlab-Token": "wrong-secret",
	})
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized status %d, got %d: %s", http.StatusUnauthorized, resp.Code, resp.Body.String())
	}
	assertNoGitLabReviewTask(t, taskRepo, "gitlab:mr-review:15594:2:de51000000000000000000000000000000000000")
}

func TestGitLabWebhookEndpointAcceptsMatchingTokenWhenConfigured(t *testing.T) {
	router, taskRepo, _ := newTestRouterWithGitLabWebhookSecret(t, "secret-1")

	resp := performRawJSONWithHeaders(t, router, http.MethodPost, "/api/events/gitlab", readFixture(t, "../events/testdata/gitlab/merge_request_opened.json"), map[string]string{
		"X-Gitlab-Token": "secret-1",
	})
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected accepted status %d, got %d: %s", http.StatusAccepted, resp.Code, resp.Body.String())
	}
	if _, err := taskRepo.GetByDedupeKey(context.Background(), "gitlab:mr-review:15594:2:de51000000000000000000000000000000000000"); err != nil {
		t.Fatalf("expected gitlab review task: %v", err)
	}
}

// TestGitLabWebhookEndpointRejectsCrossRepoSecret 锁定仓间隔离:A 仓的 payload
// 携带 B 仓 secret 签出的 token 不能通过 A 仓验签,且不落任何事件行。
func TestGitLabWebhookEndpointRejectsCrossRepoSecret(t *testing.T) {
	router, _, db := newWebhookTestRouter(t,
		seedWebhookRepo("repo_a", "15594", "demo-group/lite-list", "secret-a", registeredrepo.StatusEnabled),
		seedWebhookRepo("repo_b", "24207", "group/project", "secret-b", registeredrepo.StatusEnabled),
	)

	resp := performRawJSONWithHeaders(t, router, http.MethodPost, "/api/events/gitlab", readFixture(t, "../events/testdata/gitlab/merge_request_opened.json"), map[string]string{
		"X-Gitlab-Token": "secret-b",
	})
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("cross-repo secret must be 401, got %d: %s", resp.Code, resp.Body.String())
	}
	assertInboundEventCount(t, db, 0)

	accepted := performRawJSONWithHeaders(t, router, http.MethodPost, "/api/events/gitlab", readFixture(t, "../events/testdata/gitlab/merge_request_opened.json"), map[string]string{
		"X-Gitlab-Token": "secret-a",
	})
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("own repo secret must be accepted, got %d: %s", accepted.Code, accepted.Body.String())
	}
	assertInboundEventCount(t, db, 1)
}

// TestGitLabWebhookEndpointRejectsUnregisteredRepoWithoutPersistingEvent 锁定
// 「拒绝先于落库」:未注册仓的投递 401 且事件库零写入。
func TestGitLabWebhookEndpointRejectsUnregisteredRepoWithoutPersistingEvent(t *testing.T) {
	router, _, db := newWebhookTestRouter(t,
		seedWebhookRepo("repo_a", "15594", "demo-group/lite-list", "secret-1", registeredrepo.StatusEnabled),
	)

	payload := []byte(`{"object_kind":"push","project":{"id":99999,"path_with_namespace":"group/unknown"}}`)
	resp := performRawJSONWithHeaders(t, router, http.MethodPost, "/api/events/gitlab", payload, webhookAuthHeaders())
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("unregistered repo must be 401, got %d: %s", resp.Code, resp.Body.String())
	}
	assertInboundEventCount(t, db, 0)
}

// TestGitLabWebhookEndpointDefersDisabledRepoAndResumesOnReplay 锁定按仓验签
// 之后 not_enabled defer 与同 UUID 重放恢复语义:第一次投递验签通过但仓未启用
// → deferred;启用后平台重发同 UUID 事件 → 恢复评估并创建任务。
func TestGitLabWebhookEndpointDefersDisabledRepoAndResumesOnReplay(t *testing.T) {
	router, taskRepo, db := newWebhookTestRouter(t,
		seedWebhookRepo("repo_lite_list", "15594", "demo-group/lite-list", "secret-1", registeredrepo.StatusDisabled),
	)
	headers := map[string]string{
		"X-Gitlab-Token":        "secret-1",
		"X-Gitlab-Event":        "Merge Request Hook",
		"X-Gitlab-Event-UUID":   "defer-uuid-1",
		"X-Gitlab-Webhook-UUID": "webhook-defer-1",
		"X-Gitlab-Instance":     "https://gitlab.example",
	}

	first := performRawJSONWithHeaders(t, router, http.MethodPost, "/api/events/gitlab", readFixture(t, "../events/testdata/gitlab/merge_request_opened.json"), headers)
	if first.Code != http.StatusAccepted {
		t.Fatalf("expected accepted status %d, got %d: %s", http.StatusAccepted, first.Code, first.Body.String())
	}
	var firstBody struct {
		IgnoreReason string `json:"ignore_reason"`
		Ignored      bool   `json:"ignored"`
	}
	decodeJSON(t, first, &firstBody)
	if firstBody.Ignored || firstBody.IgnoreReason != "repository_not_enabled" {
		t.Fatalf("expected repository_not_enabled defer, got %+v", firstBody)
	}

	// 启用仓库后平台侧重发同 UUID 事件。
	result := db.Model(&registeredrepo.RegisteredRepository{}).Where("repository_id = ?", "repo_lite_list").Update("status", "enabled")
	if result.Error != nil {
		t.Fatalf("enable repo: %v", result.Error)
	}
	second := performRawJSONWithHeaders(t, router, http.MethodPost, "/api/events/gitlab", readFixture(t, "../events/testdata/gitlab/merge_request_opened.json"), headers)
	if second.Code != http.StatusAccepted {
		t.Fatalf("expected replay accepted %d, got %d: %s", http.StatusAccepted, second.Code, second.Body.String())
	}
	if _, err := taskRepo.GetByDedupeKey(context.Background(), "gitlab:mr-review:15594:2:de51000000000000000000000000000000000000"); err != nil {
		t.Fatalf("expected task after replay: %v", err)
	}
}

func TestGitLabWebhookEndpointDeduplicatesByEventUUID(t *testing.T) {
	router, taskRepo, _ := newTestRouterWithEventProcessor(t)
	headers := map[string]string{
		"X-Gitlab-Token":        "secret-1",
		"X-Gitlab-Event":        "Merge Request Hook",
		"X-Gitlab-Event-UUID":   "event-uuid-1",
		"X-Gitlab-Webhook-UUID": "webhook-delivery-1",
		"X-Gitlab-Instance":     "https://gitlab.example",
	}

	firstResp := performRawJSONWithHeaders(t, router, http.MethodPost, "/api/events/gitlab", readFixture(t, "../events/testdata/gitlab/merge_request_opened.json"), headers)
	if firstResp.Code != http.StatusAccepted {
		t.Fatalf("expected first webhook status %d, got %d: %s", http.StatusAccepted, firstResp.Code, firstResp.Body.String())
	}
	var firstBody struct {
		Duplicate bool              `json:"duplicate"`
		Task      *taskHTTPResponse `json:"task"`
	}
	decodeJSON(t, firstResp, &firstBody)
	if firstBody.Duplicate {
		t.Fatal("expected first webhook not to be duplicate")
	}
	if firstBody.Task == nil {
		t.Fatal("expected first webhook to create task")
	}

	secondPayload := strings.Replace(string(readFixture(t, "../events/testdata/gitlab/merge_request_opened.json")), "de51000000000000000000000000000000000000", "25b0360000000000000000000000000000000000", 1)
	secondResp := performRawJSONWithHeaders(t, router, http.MethodPost, "/api/events/gitlab", []byte(secondPayload), headers)
	if secondResp.Code != http.StatusAccepted {
		t.Fatalf("expected second webhook status %d, got %d: %s", http.StatusAccepted, secondResp.Code, secondResp.Body.String())
	}
	var secondBody struct {
		Duplicate bool              `json:"duplicate"`
		Task      *taskHTTPResponse `json:"task"`
	}
	decodeJSON(t, secondResp, &secondBody)
	if !secondBody.Duplicate {
		t.Fatal("expected second webhook with same event uuid to be duplicate")
	}
	if secondBody.Task != nil {
		t.Fatalf("expected duplicate webhook to omit task, got %#v", secondBody.Task)
	}
	assertNoGitLabReviewTask(t, taskRepo, "gitlab:mr-review:15594:2:25b0360000000000000000000000000000000000")
}

type taskHTTPResponse struct {
	TaskID       string `json:"task_id"`
	DedupeKey    string `json:"dedupe_key"`
	Type         string `json:"type"`
	Role         string `json:"role"`
	Status       string `json:"status"`
	CurrentRunID string `json:"current_run_id"`
}

type runHTTPResponse struct {
	RunID          string `json:"run_id"`
	TaskID         string `json:"task_id"`
	Status         string `json:"status"`
	ExternalSystem string `json:"external_system"`
	ExternalID     string `json:"external_id"`
	ExternalURL    string `json:"external_url"`
	OutputJSON     string `json:"output_json"`
}

type fakeHTTPEffectExecutor struct {
	calls []model.Effect
}

func (f *fakeHTTPEffectExecutor) ExecuteEffect(ctx context.Context, effect *model.Effect) (string, error) {
	f.calls = append(f.calls, *effect)
	return `{"delivered":true}`, nil
}

func newTestRouter(t *testing.T) (http.Handler, *orchestrator.Orchestrator, *repository.RunRepository, *repository.ArtifactRepository, *repository.DependencyRepository) {
	t.Helper()

	db := openHTTPTestDB(t)
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Artifact{}, &model.Dependency{}, &model.ResultSubmission{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	taskRepo := repository.NewTaskRepository(db)
	runRepo := repository.NewRunRepository(db)
	artifactRepo := repository.NewArtifactRepository(db)
	dependencyRepo := repository.NewDependencyRepository(db)
	resultSubmissionRepo := repository.NewResultSubmissionRepository(db)
	orch := orchestrator.New(
		taskRepo,
		runRepo,
		orchestrator.WithArtifactRepository(artifactRepo),
		orchestrator.WithDependencyRepository(dependencyRepo),
		orchestrator.WithResultSubmissionRepository(resultSubmissionRepo),
	)

	return NewRouter(orch), orch, runRepo, artifactRepo, dependencyRepo
}

func newTestRouterWithEffects(t *testing.T) (http.Handler, *repository.EffectRepository) {
	t.Helper()

	db := openHTTPTestDB(t)
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Effect{}, &model.ResultSubmission{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	taskRepo := repository.NewTaskRepository(db)
	runRepo := repository.NewRunRepository(db)
	effectRepo := repository.NewEffectRepository(db)
	resultSubmissionRepo := repository.NewResultSubmissionRepository(db)
	orch := orchestrator.New(
		taskRepo,
		runRepo,
		orchestrator.WithEffectRepository(effectRepo),
		orchestrator.WithResultSubmissionRepository(resultSubmissionRepo),
	)

	return NewRouter(orch), effectRepo
}

// seedWebhookRepo 构造按仓验签测试用的注册仓行:webhook secret 按仓持有,
// 验签凭据来自仓行本身,不存在全局回退。
func seedWebhookRepo(repositoryID, projectID, path, secret string, status registeredrepo.RepositoryStatus) *registeredrepo.RegisteredRepository {
	return &registeredrepo.RegisteredRepository{
		RepositoryID:      repositoryID,
		DisplayName:       path,
		Provider:          "gitlab",
		BaseURL:           "https://gitlab.example.com",
		CodeHostProjectID: projectID,
		PathWithNamespace: path,
		Token:             "token-a",
		WebhookSecret:     secret,
		Status:            status,
	}
}

func newWebhookTestRouter(t *testing.T, repos ...*registeredrepo.RegisteredRepository) (http.Handler, *repository.TaskRepository, *gorm.DB) {
	t.Helper()

	db := openHTTPTestDB(t)
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.InboundEvent{}, &model.ResultSubmission{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	repoStore := registeredrepo.NewStore(db)
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate registered repositories: %v", err)
	}
	for _, repo := range repos {
		if err := repoStore.Create(context.Background(), repo); err != nil {
			t.Fatalf("seed repo %s: %v", repo.PathWithNamespace, err)
		}
	}

	taskRepo := repository.NewTaskRepository(db)
	runRepo := repository.NewRunRepository(db)
	inboundRepo := repository.NewInboundEventRepository(db)
	orch := orchestrator.New(taskRepo, runRepo, orchestrator.WithResultSubmissionRepository(repository.NewResultSubmissionRepository(db)))
	processor := events.NewProcessor(inboundRepo, orch)
	processor.SetRepositoryLookup(repoStore)

	return NewRouter(orch, processor, WithWebhookSecretLookup(repoStore)), taskRepo, db
}

func newTestRouterWithEventProcessor(t *testing.T) (http.Handler, *repository.TaskRepository, *gorm.DB) {
	t.Helper()
	return newWebhookTestRouter(t,
		seedWebhookRepo("repo_project", "24207", "group/project", "secret-1", registeredrepo.StatusEnabled),
		seedWebhookRepo("repo_lite_list", "15594", "demo-group/lite-list", "secret-1", registeredrepo.StatusEnabled),
	)
}

func newTestRouterWithGitLabWebhookSecret(t *testing.T, secret string) (http.Handler, *repository.TaskRepository, *gorm.DB) {
	t.Helper()
	return newWebhookTestRouter(t,
		seedWebhookRepo("repo_lite_list", "15594", "demo-group/lite-list", secret, registeredrepo.StatusEnabled),
	)
}

func assertInboundEventCount(t *testing.T, db *gorm.DB, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.InboundEvent{}).Count(&count).Error; err != nil {
		t.Fatalf("count inbound events: %v", err)
	}
	if count != want {
		t.Fatalf("inbound events=%d, want %d", count, want)
	}
}

func assertStringSliceContains(t *testing.T, values []string, expected string) {
	t.Helper()
	for _, value := range values {
		if value == expected {
			return
		}
	}
	t.Fatalf("expected %q in %#v", expected, values)
}

func containsAll(value string, expected ...string) bool {
	for _, item := range expected {
		if !strings.Contains(value, item) {
			return false
		}
	}
	return true
}

func openHTTPTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	safeName := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_").Replace(t.Name())
	db, err := store.Open("file:" + safeName + "_" + model.NewID("testdb") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return db
}

func performJSON(t *testing.T, router http.Handler, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()

	var body *bytes.Reader
	if payload == nil {
		body = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		body = bytes.NewReader(raw)
	}

	req := httptest.NewRequest(method, path, body)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	return rec
}

func performRawJSON(t *testing.T, router http.Handler, method, path string, raw []byte) *httptest.ResponseRecorder {
	t.Helper()

	return performRawJSONWithHeaders(t, router, method, path, raw, nil)
}

func webhookAuthHeaders() map[string]string {
	return map[string]string{"X-Gitlab-Token": "secret-1"}
}

func performRawJSONWithHeaders(t *testing.T, router http.Handler, method, path string, raw []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	return rec
}

func assertNoGitLabReviewTask(t *testing.T, taskRepo *repository.TaskRepository, dedupeKey string) {
	t.Helper()

	_, err := taskRepo.GetByDedupeKey(context.Background(), dedupeKey)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected no task for %s, got err=%v", dedupeKey, err)
	}
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()

	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return raw
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()

	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode body: %v; body=%s", err, rec.Body.String())
	}
}
