package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/requirements"
	"sdlc-coordinator/internal/store"

	"github.com/gin-gonic/gin"
)

type requirementClientStub struct {
	reconcile codehost.ReconcileResult
	issue     *codehost.Issue
	states    map[int]codehost.IssueState
}

func (stub *requirementClientStub) CreateIssue(context.Context, string, codehost.CreateIssueRequest) (*codehost.Issue, error) {
	return stub.issue, nil
}

func (stub *requirementClientStub) ReconcileWriteback(context.Context, codehost.ReconcileRequest) (codehost.ReconcileResult, error) {
	return stub.reconcile, nil
}

func (stub *requirementClientStub) GetIssueState(_ context.Context, _ string, iid int) (codehost.IssueState, error) {
	return stub.states[iid], nil
}

func newRequirementRouter(t *testing.T) *gin.Engine {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	requirementStore := requirements.NewStore(db)
	if err := requirementStore.Migrate(); err != nil {
		t.Fatalf("migrate requirements: %v", err)
	}
	repoStore := registeredrepo.NewStore(db)
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate repos: %v", err)
	}
	if err := repoStore.Create(context.Background(), &registeredrepo.RegisteredRepository{
		RepositoryID: "repo-1", DisplayName: "Demo", PathWithNamespace: "group/demo", Status: registeredrepo.StatusEnabled,
	}); err != nil {
		t.Fatalf("seed repo: %v", err)
	}
	service := requirements.NewService(requirementStore, repoStore, func(*registeredrepo.RegisteredRepository) requirements.ClientFactory {
		return &requirementClientStub{issue: &codehost.Issue{IID: 1, WebURL: "http://codehost/group/demo/issues/1"}, states: map[int]codehost.IssueState{}}
	})
	router := gin.New()
	NewAdminRequirementHandler(service).RegisterRoutes(router.Group("/api/admin"))
	return router
}

func performRequirementJSON(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
	}
	request := httptest.NewRequest(method, path, strings.NewReader(string(payload)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestAdminRequirementsCreateAndList(t *testing.T) {
	router := newRequirementRouter(t)
	created := performRequirementJSON(t, router, http.MethodPost, "/api/admin/requirements", map[string]any{
		"title": "支付回调幂等", "description": "防重", "repository_id": "repo-1",
		"labels": []string{"backend"}, "priority": "high",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create status %d body %s", created.Code, created.Body.Bytes())
	}
	var item map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if item["status"] != "queued" || item["requirement_id"] == "" {
		t.Fatalf("unexpected create payload: %v", item)
	}
	if labels, ok := item["labels"].([]any); !ok || len(labels) != 1 || labels[0] != "backend" {
		t.Fatalf("labels must round-trip: %v", item["labels"])
	}

	missing := performRequirementJSON(t, router, http.MethodPost, "/api/admin/requirements", map[string]any{"title": " ", "repository_id": "repo-1"})
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("empty title must 400, got %d", missing.Code)
	}

	listed := performRequirementJSON(t, router, http.MethodGet, "/api/admin/requirements?status=queued&repository_id=repo-1", nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("list status %d", listed.Code)
	}
	if !strings.Contains(listed.Body.String(), `"total":1`) {
		t.Fatalf("list must contain total 1: %s", listed.Body.String())
	}
	if !strings.Contains(listed.Body.String(), `"page":1`) {
		t.Fatalf("list must contain page: %s", listed.Body.String())
	}
}

func TestAdminRequirementsActionGuards(t *testing.T) {
	router := newRequirementRouter(t)
	created := performRequirementJSON(t, router, http.MethodPost, "/api/admin/requirements", map[string]any{"title": "x", "repository_id": "repo-1"})
	var item map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	requirementID := item["requirement_id"].(string)

	if resp := performRequirementJSON(t, router, http.MethodPost, "/api/admin/requirements/"+requirementID+"/complete", nil); resp.Code != http.StatusConflict {
		t.Fatalf("complete queued must 409, got %d", resp.Code)
	}
	if resp := performRequirementJSON(t, router, http.MethodPost, "/api/admin/requirements/"+requirementID+"/cancel", nil); resp.Code != http.StatusOK {
		t.Fatalf("cancel queued must 200, got %d", resp.Code)
	}
	if resp := performRequirementJSON(t, router, http.MethodPatch, "/api/admin/requirements/"+requirementID, map[string]any{"title": "y"}); resp.Code != http.StatusConflict {
		t.Fatalf("patch cancelled must 409, got %d", resp.Code)
	}
	if resp := performRequirementJSON(t, router, http.MethodPost, "/api/admin/requirements/req_missing/cancel", nil); resp.Code != http.StatusNotFound {
		t.Fatalf("missing requirement must 404, got %d", resp.Code)
	}
}

func TestAdminRequirementsSyncEndpoint(t *testing.T) {
	router := newRequirementRouter(t)
	if resp := performRequirementJSON(t, router, http.MethodPost, "/api/admin/requirements", map[string]any{"title": "x", "repository_id": "repo-1"}); resp.Code != http.StatusCreated {
		t.Fatalf("seed create failed: %d", resp.Code)
	}
	resp := performRequirementJSON(t, router, http.MethodPost, "/api/admin/requirements/sync", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("sync status %d body %s", resp.Code, resp.Body.Bytes())
	}
	if !strings.Contains(resp.Body.String(), `"created":1`) {
		t.Fatalf("sync must report created: %s", resp.Body.String())
	}
}
