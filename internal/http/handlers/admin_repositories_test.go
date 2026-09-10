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

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/tenantdb"

	"github.com/gin-gonic/gin"
)

type repoProbeStub struct {
	ident registeredrepo.ProjectIdentity
}

func (stub repoProbeStub) GetProject(_ context.Context, _, _, path string) (registeredrepo.ProjectIdentity, error) {
	ident := stub.ident
	if ident.PathWithNamespace == "" {
		ident.PathWithNamespace = path
	}
	if ident.ID == "" {
		ident.ID = "24207"
	}
	if ident.DefaultBranch == "" {
		ident.DefaultBranch = "master"
	}
	return ident, nil
}

func newRepositoryRouter(t *testing.T, activity registeredrepo.ActivityChecker) *gin.Engine {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repoStore := registeredrepo.NewStore(db)
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := registeredrepo.NewService(repoStore, nil, activity)
	svc.WithProbeFactory(func(string) registeredrepo.ProjectProbe { return repoProbeStub{} })
	router := gin.New()
	NewAdminRepositoryHandler(svc).RegisterRoutes(router.Group("/api/admin"))
	return router
}

func performAdminRepositoryJSON(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestAdminRepositoriesCreateRequiresWebhookSecret(t *testing.T) {
	router := newRepositoryRouter(t, nil)
	resp := performAdminRepositoryJSON(t, router, http.MethodPost, "/api/admin/repositories", map[string]any{
		"display_name":         "Coordinator",
		"path_with_namespace":  "group/coordinator",
		"provider":             "gitlab",
		"base_url":             "https://gitlab.example.com",
		"token":                "repo-token-value",
		"token_scope_attested": true,
	})
	if resp.Code != http.StatusBadRequest || !strings.Contains(resp.Body.String(), "webhook_secret_required") {
		t.Fatalf("status %d body %s", resp.Code, resp.Body.String())
	}
}

func TestAdminRepositoriesCreateMasksWebhookSecret(t *testing.T) {
	router := newRepositoryRouter(t, nil)
	resp := performAdminRepositoryJSON(t, router, http.MethodPost, "/api/admin/repositories", map[string]any{
		"display_name":         "Coordinator",
		"path_with_namespace":  "group/coordinator",
		"provider":             "gitlab",
		"base_url":             "https://gitlab.example.com",
		"token":                "repo-token-value",
		"token_scope_attested": true,
		"webhook_secret":       "repo-hook-secret-value",
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", resp.Code, resp.Body.Bytes())
	}
	if strings.Contains(resp.Body.String(), "repo-hook-secret-value") {
		t.Fatal("webhook secret leaked")
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["webhook_secret_configured"] != true {
		t.Fatalf("webhook_secret_configured=%v", body["webhook_secret_configured"])
	}
}

func TestAdminRepositoriesPatchRotatesWebhookSecret(t *testing.T) {
	router := newRepositoryRouter(t, nil)
	created := performAdminRepositoryJSON(t, router, http.MethodPost, "/api/admin/repositories", map[string]any{
		"display_name": "A", "path_with_namespace": "group/a", "provider": "gitlab", "base_url": "https://gitlab.example.com", "token": "token-a",
		"token_scope_attested": true, "webhook_secret": "hook-secret-a",
	})
	var body struct {
		RepositoryID string `json:"repository_id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	patched := performAdminRepositoryJSON(t, router, http.MethodPatch, "/api/admin/repositories/"+body.RepositoryID, map[string]any{
		"webhook_secret": "hook-secret-b",
	})
	if patched.Code != http.StatusOK {
		t.Fatalf("patch %d %s", patched.Code, patched.Body.String())
	}
	if strings.Contains(patched.Body.String(), "hook-secret") {
		t.Fatal("webhook secret leaked in patch response")
	}
	var patchedBody map[string]any
	if err := json.Unmarshal(patched.Body.Bytes(), &patchedBody); err != nil {
		t.Fatal(err)
	}
	if patchedBody["webhook_secret_configured"] != true {
		t.Fatalf("webhook_secret_configured=%v", patchedBody["webhook_secret_configured"])
	}
}

func TestAdminRepositoriesCreateMasksToken(t *testing.T) {
	router := newRepositoryRouter(t, nil)
	resp := performAdminRepositoryJSON(t, router, http.MethodPost, "/api/admin/repositories", map[string]any{
		"display_name":         "Coordinator",
		"path_with_namespace":  "group/coordinator",
		"provider":             "gitlab",
		"base_url":             "https://gitlab.example.com",
		"token":                "repo-token-value",
		"token_scope_attested": true,
		"webhook_secret":       "repo-hook-secret",
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", resp.Code, resp.Body.Bytes())
	}
	if strings.Contains(resp.Body.String(), "repo-token-value") {
		t.Fatal("token leaked")
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["token_configured"] != true {
		t.Fatalf("token_configured=%v", body["token_configured"])
	}
	if body["status"] != "configuring" {
		t.Fatalf("status=%v", body["status"])
	}
	if body["codehost_project_id"] != "24207" {
		t.Fatalf("codehost_project_id=%v", body["codehost_project_id"])
	}
}

func TestAdminRepositoriesExposeIssueLabels(t *testing.T) {
	router := newRepositoryRouter(t, nil)
	created := performAdminRepositoryJSON(t, router, http.MethodPost, "/api/admin/repositories", map[string]any{
		"display_name":          "Coordinator",
		"path_with_namespace":   "group/coordinator",
		"provider":              "gitlab",
		"base_url":              "https://gitlab.example.com",
		"token":                 "repo-token-value",
		"token_scope_attested":  true,
		"webhook_secret":        "repo-hook-secret",
		"codehost_issue_labels": []string{"developer-agent"},
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create %d %s", created.Code, created.Body.String())
	}
	var repository struct {
		RepositoryID        string   `json:"repository_id"`
		CodeHostIssueLabels []string `json:"codehost_issue_labels"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &repository); err != nil {
		t.Fatal(err)
	}
	if len(repository.CodeHostIssueLabels) != 1 || repository.CodeHostIssueLabels[0] != "developer-agent" {
		t.Fatalf("labels=%#v", repository.CodeHostIssueLabels)
	}
}

func TestAdminRepositoryRetiredConnectorRoutesReturn404(t *testing.T) {
	router := newRepositoryRouter(t, nil)
	created := performAdminRepositoryJSON(t, router, http.MethodPost, "/api/admin/repositories", map[string]any{
		"display_name": "A", "path_with_namespace": "group/a", "provider": "gitlab", "base_url": "https://gitlab.example.com", "token": "token-a", "token_scope_attested": true, "webhook_secret": "hook-secret-a",
	})
	var body struct {
		RepositoryID string `json:"repository_id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	resp := performAdminRepositoryJSON(t, router, http.MethodGet, "/api/admin/repositories/"+body.RepositoryID+"/connector/runs", map[string]any{})
	if resp.Code != http.StatusNotFound {
		t.Fatalf("GET connector/runs expected 404, got %d %s", resp.Code, resp.Body.String())
	}
}

func TestAdminRepositoriesDisableBusyConflict(t *testing.T) {
	router := newRepositoryRouter(t, busyActivity{})
	created := performAdminRepositoryJSON(t, router, http.MethodPost, "/api/admin/repositories", map[string]any{
		"display_name": "A", "path_with_namespace": "group/a", "provider": "gitlab", "base_url": "https://gitlab.example.com", "token": "token-a", "token_scope_attested": true, "webhook_secret": "hook-secret-a",
	})
	var body struct {
		RepositoryID string `json:"repository_id"`
	}
	json.Unmarshal(created.Body.Bytes(), &body)
	disabled := performAdminRepositoryJSON(t, router, http.MethodPost, "/api/admin/repositories/"+body.RepositoryID+"/disable", nil)
	if disabled.Code != http.StatusConflict || !strings.Contains(disabled.Body.String(), "repository_busy") {
		t.Fatalf("status %d body %s", disabled.Code, disabled.Body.String())
	}
}

func TestAdminRepositoriesUnknownIDNotFound(t *testing.T) {
	router := newRepositoryRouter(t, nil)
	resp := performAdminRepositoryJSON(t, router, http.MethodGet, "/api/admin/repositories/repo_missing", nil)
	if resp.Code != http.StatusNotFound || !strings.Contains(resp.Body.String(), "repository_not_found") {
		t.Fatalf("status %d body %s", resp.Code, resp.Body.String())
	}
}

func TestAdminRepositoriesRejectUnattestedToken(t *testing.T) {
	router := newRepositoryRouter(t, nil)
	resp := performAdminRepositoryJSON(t, router, http.MethodPost, "/api/admin/repositories", map[string]any{
		"path_with_namespace": "group/a", "provider": "gitlab", "base_url": "https://gitlab.example.com", "token": "secret",
	})
	if resp.Code != http.StatusBadRequest || !strings.Contains(resp.Body.String(), "token_not_attested") {
		t.Fatalf("status %d body %s", resp.Code, resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), "secret") {
		t.Fatal("token leaked")
	}
}

type busyActivity struct{}

func (busyActivity) HasActiveWork(context.Context, string) (bool, error) {
	return true, nil
}

// TestAdminRepositoriesRejectRetiredGitLabIssueLabelsKey 锁定契约切换:
// DisallowUnknownFields 下改名前的旧键(gitlab_issue_labels)创建/更新必 400,
// 不做双读;旧管理页刷新加载新 bundle 即自愈。新键全链照常。
func TestAdminRepositoriesRejectRetiredGitLabIssueLabelsKey(t *testing.T) {
	router := newRepositoryRouter(t, nil)
	create := performAdminRepositoryJSON(t, router, http.MethodPost, "/api/admin/repositories", map[string]any{
		"display_name": "A", "path_with_namespace": "group/a", "provider": "gitlab",
		"base_url": "https://gitlab.example.com", "token": "repo-token-value", "token_scope_attested": true,
		"webhook_secret": "hook-secret-a", "gitlab_issue_labels": []string{"developer-agent"},
	})
	if create.Code != http.StatusBadRequest {
		t.Fatalf("retired create key must be rejected: %d %s", create.Code, create.Body.String())
	}
	patch := performAdminRepositoryJSON(t, router, http.MethodPatch, "/api/admin/repositories/repo-x", map[string]any{
		"gitlab_issue_labels": []string{"developer-agent"},
	})
	if patch.Code != http.StatusBadRequest {
		t.Fatalf("retired patch key must be rejected: %d %s", patch.Code, patch.Body.String())
	}
	accept := performAdminRepositoryJSON(t, router, http.MethodPost, "/api/admin/repositories", map[string]any{
		"display_name": "A", "path_with_namespace": "group/a", "provider": "gitlab",
		"base_url": "https://gitlab.example.com", "token": "repo-token-value", "token_scope_attested": true,
		"webhook_secret": "hook-secret-a", "codehost_issue_labels": []string{"developer-agent"},
	})
	if accept.Code != http.StatusCreated {
		t.Fatalf("new key must be accepted: %d %s", accept.Code, accept.Body.String())
	}
}

type claimingRouteIndexSync struct{}

func (claimingRouteIndexSync) Upsert(context.Context, uint, registeredrepo.RouteIndexEntry) error {
	return fmt.Errorf("%w: concurrent claim won", registeredrepo.ErrRouteIdentityClaimed)
}

func (claimingRouteIndexSync) Delete(context.Context, registeredrepo.RouteIndexEntry) error {
	return nil
}

func (claimingRouteIndexSync) Lookup(context.Context, registeredrepo.RouteIndexEntry) (uint, bool, error) {
	return 0, false, nil
}

// 注册竞态败者的 HTTP 链路：预检通过但写穿索引撞上并发占用（唯一索引
// 拒绝），必须以 409 + route_identity_claimed 收场，而不是建了一座永远
// 路由不到的仓却回 201 假成功。
func TestAdminRepositoriesCreateReturns409WhenRouteClaimLosesRace(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repoStore := registeredrepo.NewStore(db)
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := registeredrepo.NewService(repoStore, nil, nil)
	svc.WithProbeFactory(func(string) registeredrepo.ProjectProbe { return repoProbeStub{} })
	handler := NewAdminRepositoryHandler(svc)
	handler.SetRouteIndexSync(claimingRouteIndexSync{})

	router := gin.New()
	// 模拟租户会话绑定：reposFor 按会话租户把写穿端口挂到本次请求。
	router.Use(func(c *gin.Context) {
		setTenantStore(c, &tenantdb.Store{DB: db})
		c.Set(cpSessionKey, &controlplane.SessionInfo{EffectiveTenant: &controlplane.Tenant{ID: 2}})
	})
	handler.RegisterRoutes(router.Group("/api/admin"))

	resp := performAdminRepositoryJSON(t, router, http.MethodPost, "/api/admin/repositories", map[string]any{
		"display_name":         "Raced",
		"path_with_namespace":  "group/raced",
		"provider":             "gitlab",
		"base_url":             "https://gitlab.example.com",
		"token":                "repo-token-value",
		"token_scope_attested": true,
		"webhook_secret":       "repo-hook-secret-value",
	})
	if resp.Code != http.StatusConflict || !strings.Contains(resp.Body.String(), "route_identity_claimed") {
		t.Fatalf("race loser must get 409 route_identity_claimed, got %d: %s", resp.Code, resp.Body.String())
	}
	var count int64
	if err := db.Model(&registeredrepo.RegisteredRepository{}).Count(&count).Error; err != nil {
		t.Fatalf("count repositories: %v", err)
	}
	if count != 0 {
		t.Fatalf("race loser must leave no orphan row, got %d", count)
	}
}
