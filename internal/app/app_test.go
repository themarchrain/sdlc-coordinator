package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/configuration"
	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/events"
	"sdlc-coordinator/internal/http/handlers"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/people"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/tenantdb"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// seedAppRepository 注册一个 enabled 仓库行;恒注册仓模式下 webhook 准入、
// 回写解析与派发凭据都要求仓库行存在(全局 Token 老通道已退役)。
func seedAppRepository(t *testing.T, db *gorm.DB, repositoryID, projectID, path, baseURL string) {
	t.Helper()
	if err := registeredrepo.NewStore(db).Create(context.Background(), &registeredrepo.RegisteredRepository{
		RepositoryID: repositoryID, DisplayName: path, Provider: "gitlab", BaseURL: baseURL, CodeHostProjectID: projectID,
		PathWithNamespace: path, Token: "repo-token", TokenScopeAttested: true,
		WebhookSecret: "webhook-secret", Status: registeredrepo.StatusEnabled,
	}); err != nil {
		t.Fatalf("seed registered repository: %v", err)
	}
}

// seedTenantRoutedRepository 按真路径注册仓：经服务建组织（provisioner
// 同步开通租户库并迁移 schema）、仓行落租户库、路由索引指向该租户——
// webhook 准入与事件处理都路由到租户引擎（未路由仓一律验签 401，不存在
// 平台级仓行）。Windows 下租户库句柄不显式关闭会阻塞 TempDir 清理。
func seedTenantRoutedRepository(t *testing.T, application *App, slug, provider, repositoryID, projectID, path, baseURL, secret string) *controlplane.Tenant {
	t.Helper()
	ctx := context.Background()
	t.Cleanup(func() {
		application.pool.CloseTenantEngines()
		if resolver, ok := application.TenantStores.(*tenantdb.SqliteFileResolver); ok {
			resolver.Close()
		}
	})
	tenant, err := application.ControlPlane.CreateTenant(ctx, &controlplane.Actor{ID: 1, Username: "root"}, controlplane.TenantInput{
		Slug: slug, DisplayName: slug,
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	tenantStore, err := application.TenantStores.Handle(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("open tenant store: %v", err)
	}
	if err := registeredrepo.NewStore(tenantStore.DB).Create(ctx, &registeredrepo.RegisteredRepository{
		RepositoryID: repositoryID, DisplayName: path, Provider: provider, BaseURL: baseURL, CodeHostProjectID: projectID,
		PathWithNamespace: path, Token: "repo-token", TokenScopeAttested: true,
		WebhookSecret: secret, Status: registeredrepo.StatusEnabled,
	}); err != nil {
		t.Fatalf("seed tenant repository: %v", err)
	}
	if err := application.ControlPlane.Repository().UpsertRepoRouteIndex(ctx, controlplane.RepoRouteIndex{
		Provider: provider, BaseURL: baseURL, PathWithNamespace: path,
		TenantID: tenant.ID, RepoFingerprint: repositoryID,
	}); err != nil {
		t.Fatalf("seed repo route index: %v", err)
	}
	return tenant
}

func TestNewLeavesSchedulerDisabledByDefault(t *testing.T) {
	application, err := New(withTestControlPlane(t, config.Config{
		HTTPAddr: ":0",
	}), zap.NewNop(), openAppTestDB(t))
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	if application.Scheduler != nil {
		t.Fatal("expected internal scheduler to remain disabled")
	}
}

func TestNewBootstrapDoesNotAssembleRuntimeWorkersAndGatesWrites(t *testing.T) {
	db := openAppTestDB(t)
	repository := configuration.NewRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate configuration: %v", err)
	}
	service := configuration.NewService(repository, configuration.Resolution{Mode: configuration.ModeSetupPending})

	application, err := NewBootstrap(withTestBootstrap(t, config.BootstrapConfig{HTTPAddr: ":0"}), zap.NewNop(), db, service, configuration.NewShutdownSignal(), configuration.ModeSetupPending)
	if err != nil {
		t.Fatalf("new bootstrap app: %v", err)
	}
	if application.Orch != nil || application.Scheduler != nil {
		t.Fatalf("runtime workers assembled in bootstrap mode: %#v", application)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(`{"dedupe_key":"must-not-run"}`))
	request.Header.Set("Content-Type", "application/json")
	application.Router.ServeHTTP(response, request)
	// 未认证 runtime 写 API 已整体移除：bootstrap 模式下路由不存在（404），
	// 叠加运行时组件未装配，无认证写面不存在。
	if response.Code != http.StatusNotFound {
		t.Fatalf("unauthenticated runtime write API must not exist, got %d: %s", response.Code, response.Body.String())
	}
}

func TestNewBootstrapExposesPeopleDirectoryWithoutRuntimeWorkers(t *testing.T) {
	db := openAppTestDB(t)
	repository := configuration.NewRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate configuration: %v", err)
	}
	service := configuration.NewService(repository, configuration.Resolution{Mode: configuration.ModeSetupPending})
	application, err := NewBootstrap(withTestBootstrap(t, config.BootstrapConfig{HTTPAddr: ":0"}), zap.NewNop(), db, service, configuration.NewShutdownSignal(), configuration.ModeSetupPending)
	if err != nil {
		t.Fatalf("new bootstrap app: %v", err)
	}

	legacyAuth := httptest.NewRecorder()
	legacyReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"admin123"}`))
	legacyReq.Header.Set("Content-Type", "application/json")
	application.Router.ServeHTTP(legacyAuth, legacyReq)
	if legacyAuth.Code != http.StatusNotFound {
		t.Fatalf("legacy /api/auth/login must be gone, got %d: %s", legacyAuth.Code, legacyAuth.Body.String())
	}

	loginBody := strings.NewReader(`{"username":"root","password":"test-super-password"}`)
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/portal/auth/login", loginBody)
	loginRequest.Header.Set("Content-Type", "application/json")
	loginResponse := httptest.NewRecorder()
	application.Router.ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("unexpected bootstrap portal login %d: %s", loginResponse.Code, loginResponse.Body.String())
	}
	var portalCookie *http.Cookie
	for _, item := range loginResponse.Result().Cookies() {
		if item.Name == handlers.PortalSessionCookieName {
			portalCookie = item
		}
	}
	if portalCookie == nil {
		t.Fatal("missing portal session cookie")
	}

	// people 面收窄为 support_only——bootstrap 装配同语义，进入
	// 支持访问后仍可用（本测锁定的是 bootstrap 装配接线，非鉴权窗口）。
	createdTenant, err := application.ControlPlane.CreateTenant(context.Background(), nil, controlplane.TenantInput{Slug: "acme-it", DisplayName: "Acme IT"})
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if err := application.ControlPlane.EnterSupportAccess(context.Background(), portalCookie.Value, createdTenant.ID, "bootstrap wiring test"); err != nil {
		t.Fatalf("enter support access: %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/people", nil)
	request.AddCookie(portalCookie)
	application.Router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"items":[]`) {
		t.Fatalf("unexpected people response %d: %s", response.Code, response.Body.String())
	}
	if application.Orch != nil || application.Scheduler != nil {
		t.Fatalf("people directory assembled runtime workers in bootstrap mode: %#v", application)
	}
}

func TestNewWiresEnabledScheduler(t *testing.T) {
	cfg := config.Config{
		HTTPAddr: ":0",
	}
	cfg.Scheduler.Enabled = true
	cfg.Scheduler.Interval = 30 * time.Second
	cfg.Scheduler.TickRequest = orchestrator.TickRequest{DispatchLimit: 1, EffectLimit: 5}

	application, err := New(withTestControlPlane(t, cfg), zap.NewNop(), openAppTestDB(t))
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	if application.Scheduler == nil {
		t.Fatal("expected enabled internal scheduler")
	}
}

func TestNewWiresReviewerDispatchToConfiguredAgentRunner(t *testing.T) {
	reviewerBaseURL, headSHA := createAppReviewerOrigin(t)
	runnerService, err := runner.NewService(t.TempDir(), map[string]runner.Provider{"fake": appReviewerProvider{}})
	if err != nil {
		t.Fatalf("new runner service: %v", err)
	}
	runnerServer := httptest.NewServer(runner.NewHTTPHandler(runnerService))
	defer runnerServer.Close()
	cfg := config.Config{HTTPAddr: ":0"}
	cfg.AgentRunner = config.AgentRunnerSettings{
		ServiceURL: runnerServer.URL, WorkspaceRoot: t.TempDir(), Provider: "fake",
		Model: "model-a", Effort: "high", ProviderConfigRevision: 9,
	}
	application, err := New(withTestControlPlane(t, cfg), zap.NewNop(), openAppTestDB(t))
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	seedAppRepository(t, application.DB, "repo-app-reviewer", "1001", "group/project", reviewerBaseURL)
	repositoryID := "repo-app-reviewer"
	// 派发按真租户路由（fail-closed）：为仓登记路由索引指到默认租户。
	cpRepo := application.ControlPlane.Repository()
	tenant := &controlplane.Tenant{Slug: "acme-it", DisplayName: "Acme IT"}
	if err := cpRepo.CreateTenant(context.Background(), tenant); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if err := cpRepo.UpsertRepoRouteIndex(context.Background(), controlplane.RepoRouteIndex{
		Provider: "gitlab", BaseURL: reviewerBaseURL, PathWithNamespace: "group/project",
		TenantID: tenant.ID, RepoFingerprint: repositoryID,
	}); err != nil {
		t.Fatalf("seed repo route index: %v", err)
	}
	// 事件载荷 URL 固定为不可达的外部地址:若实现回退为事件 URL,真实 git 准备会
	// 网络失败,本测试即红——锁定「clone 以注册仓行为权威」。
	task, err := application.Orch.CreateTask(context.Background(), &model.Task{
		TaskID: "task-app-runner-reviewer", DedupeKey: "app-runner-reviewer", Type: "review_mr", Role: "ReviewerWorker",
		SourceRepo: "group/project", TargetRepo: "group/project", RepositoryID: &repositoryID, Objective: "review MR", AcceptanceCriteria: "return verdict",
		ContextJSON: `{"project":{"path":"group/project"},"merge_request":{"iid":21,"url":"https://gitlab.example/group/project/-/merge_requests/21","review_mode":"full","source_branch":"feature/review","target_branch":"master","head_sha":"` + headSHA + `"},"repos":[{"name":"project","url":"https://gitlab.example.com/group/project.git","branch":"feature/review","access":"readable"}]}`,
	})
	if err != nil {
		t.Fatalf("create reviewer task: %v", err)
	}
	dispatched, err := application.Orch.DispatchReviewerTask(context.Background(), task.TaskID)
	if err != nil {
		t.Fatalf("dispatch reviewer task: %v", err)
	}
	remote, err := runnerService.Get(dispatched.Run.RunID)
	if err != nil || remote.Provider != "fake" || remote.RequestHash == "" {
		t.Fatalf("configured runner did not receive execution: execution=%#v err=%v", remote, err)
	}
}

func TestNewAndBootstrapMigrateAgentExecutions(t *testing.T) {
	t.Run("ready", func(t *testing.T) {
		db := openAppTestDB(t)
		if _, err := New(withTestControlPlane(t, config.Config{HTTPAddr: ":0"}), zap.NewNop(), db); err != nil {
			t.Fatalf("new app: %v", err)
		}
		if !db.Migrator().HasTable(&model.AgentExecution{}) {
			t.Fatal("expected agent executions table in ready mode")
		}
	})

	t.Run("bootstrap", func(t *testing.T) {
		db := openAppTestDB(t)
		if _, err := NewBootstrap(withTestBootstrap(t, config.BootstrapConfig{HTTPAddr: ":0"}), zap.NewNop(), db, nil, nil, configuration.ModeSetupPending); err != nil {
			t.Fatalf("new bootstrap app: %v", err)
		}
		if !db.Migrator().HasTable(&model.AgentExecution{}) {
			t.Fatal("expected agent executions table in bootstrap mode")
		}
	})
}

func TestNewWiresAdminQueryRoutesToSharedDatabase(t *testing.T) {
	ctx := context.Background()
	db := openAppTestDB(t)
	application, err := New(withTestControlPlane(t, config.Config{
		HTTPAddr: ":0",
	}), zap.NewNop(), db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}

	// 真分库装配：经服务建租户（触发同事务数据面开通），任务种进租户库，
	// 租户作用域查询命中租户自己的库（不再是平台库）。
	if _, err := application.ControlPlane.CreateTenant(ctx, nil, controlplane.TenantInput{Slug: "acme-it", DisplayName: "Acme IT"}); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	tenant, err := application.ControlPlane.Repository().GetTenantBySlug(ctx, "acme-it")
	if err != nil || tenant == nil {
		t.Fatalf("lookup tenant: %v", err)
	}
	tenantStore, err := application.TenantStores.Handle(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("open tenant store: %v", err)
	}
	if err := tenantStore.DB.Create(&model.Task{
		TaskID: "task-app-admin", DedupeKey: "task-app-admin", Type: "test", Role: "TesterWorker", Status: model.TaskStatusPending,
	}).Error; err != nil {
		t.Fatalf("create task in tenant store: %v", err)
	}

	t.Cleanup(func() {
		if closer, ok := application.TenantStores.(interface{ Close() }); ok {
			closer.Close()
		}
	})

	recorder := httptest.NewRecorder()
	request := authenticatedAdminRequest(t, application, http.MethodGet, "/api/admin/tasks")
	application.Router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	var response struct {
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Total != 1 {
		t.Fatalf("expected one task, got %#v", response)
	}
}

func TestNewWiresConnectorAdminRoutes(t *testing.T) {
	application, err := New(withTestControlPlane(t, config.Config{
		HTTPAddr: ":0",
	}), zap.NewNop(), openAppTestDB(t))
	if err != nil {
		t.Fatalf("new app: %v", err)
	}

	t.Cleanup(func() {
		if closer, ok := application.TenantStores.(interface{ Close() }); ok {
			closer.Close()
		}
	})

	recorder := httptest.NewRecorder()
	request := authenticatedAdminRequest(t, application, http.MethodGet, "/api/admin/connectors")
	application.Router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
}

func openAppTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return db
}

func authenticatedAdminRequest(t *testing.T, application *App, method, path string) *http.Request {
	t.Helper()
	if application.ControlPlane == nil {
		t.Fatal("authenticatedAdminRequest requires a wired control plane")
	}
	ctx := context.Background()
	const username = "app-test-root"
	const password = "test-super-password"
	existing, err := application.ControlPlane.GetUserByUsername(ctx, username)
	if err != nil {
		t.Fatalf("lookup portal super admin: %v", err)
	}
	if existing == nil {
		if _, err := application.ControlPlane.CreateSuperAdminUser(ctx, username, password); err != nil {
			t.Fatalf("seed portal super admin: %v", err)
		}
	}
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/portal/auth/login", strings.NewReader(`{"username":"`+username+`","password":"`+password+`"}`))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginResponse := httptest.NewRecorder()
	application.Router.ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("portal login for request: status=%d body=%s", loginResponse.Code, loginResponse.Body.String())
	}
	var cookie *http.Cookie
	for _, item := range loginResponse.Result().Cookies() {
		if item.Name == handlers.PortalSessionCookieName {
			cookie = item
		}
	}
	if cookie == nil {
		t.Fatal("missing portal session cookie")
	}
	// 业务面对超管收窄为 support_only：helper 统一进入支持访问
	//（租户 lookup-or-create；经服务创建以触发数据面开通）。
	repository := application.ControlPlane.Repository()
	tenant, err := repository.GetTenantBySlug(ctx, "acme-it")
	if err != nil {
		t.Fatalf("lookup support tenant: %v", err)
	}
	if tenant == nil {
		if _, err := application.ControlPlane.CreateTenant(ctx, nil, controlplane.TenantInput{Slug: "acme-it", DisplayName: "Acme IT"}); err != nil {
			t.Fatalf("seed support tenant: %v", err)
		}
		if tenant, err = repository.GetTenantBySlug(ctx, "acme-it"); err != nil || tenant == nil {
			t.Fatalf("reload support tenant: %v", err)
		}
	}
	if err := application.ControlPlane.EnterSupportAccess(ctx, cookie.Value, tenant.ID, "wiring test"); err != nil {
		t.Fatalf("enter support access: %v", err)
	}
	request := httptest.NewRequest(method, path, nil)
	request.AddCookie(cookie)
	return request
}

type appReviewerProvider struct{}

func (appReviewerProvider) Execute(context.Context, runner.ProviderRequest, runner.EmitFunc) (json.RawMessage, error) {
	return json.RawMessage(`{"task_id":"task-app-runner-reviewer","run_id":"unused","verdict":"approved"}`), nil
}

// createAppReviewerOrigin 返回本地 bare origin 的 base_url 与 head:布局为
// <root>/group/project.git,使注册仓行派生的 CloneURL(root + path + .git)
// 与真实 origin 一致,派发即按仓行 URL 完成真实 git 准备。
func createAppReviewerOrigin(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	seed := filepath.Join(root, "seed")
	if err := os.MkdirAll(seed, 0o755); err != nil {
		t.Fatal(err)
	}
	appReviewerGit(t, seed, "init", "-b", "master")
	appReviewerGit(t, seed, "config", "user.name", "App Test")
	appReviewerGit(t, seed, "config", "user.email", "app@example.com")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	appReviewerGit(t, seed, "add", "README.md")
	appReviewerGit(t, seed, "commit", "-m", "base")
	appReviewerGit(t, seed, "checkout", "-b", "feature/review")
	if err := os.WriteFile(filepath.Join(seed, "feature.go"), []byte("package feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	appReviewerGit(t, seed, "add", "feature.go")
	appReviewerGit(t, seed, "commit", "-m", "feature")
	head := strings.TrimSpace(appReviewerGit(t, seed, "rev-parse", "HEAD"))
	origin := filepath.Join(root, "group", "project.git")
	if err := os.MkdirAll(filepath.Dir(origin), 0o755); err != nil {
		t.Fatal(err)
	}
	appReviewerGit(t, root, "init", "--bare", origin)
	appReviewerGit(t, seed, "remote", "add", "origin", origin)
	appReviewerGit(t, seed, "push", "origin", "master", "feature/review")
	return filepath.ToSlash(root), head
}

func appReviewerGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func TestNewMigratesWritebacks(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	_, err = New(withTestControlPlane(t, config.Config{
		HTTPAddr: ":0",
	}), zap.NewNop(), db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}

	if !db.Migrator().HasTable(&model.Writeback{}) {
		t.Fatal("expected writebacks table to be migrated")
	}
	if !db.Migrator().HasTable(&model.InboundEvent{}) {
		t.Fatal("expected inbound_events table to be migrated")
	}
	if !db.Migrator().HasTable(&model.Effect{}) {
		t.Fatal("expected effects table to be migrated")
	}
	if !db.Migrator().HasTable(&model.ResultSubmission{}) {
		t.Fatal("expected result_submissions table to be migrated")
	}
	if !db.Migrator().HasTable(&model.ConnectorInstance{}) {
		t.Fatal("expected connector_instances table to be migrated")
	}
}

func TestNewWiresRequirementsModule(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	application, err := New(withTestControlPlane(t, config.Config{
		HTTPAddr: ":0",
	}), zap.NewNop(), db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	if application.RequirementsSync == nil {
		t.Fatal("requirements sync runner must be wired")
	}
	var count int64
	if err := db.Table("requirements").Count(&count).Error; err != nil {
		t.Fatalf("requirements table must exist: %v", err)
	}
}

func TestNewCreatesDeveloperClarificationFollowupsFromAssigneeMap(t *testing.T) {
	ctx := context.Background()
	t.Setenv("GITLAB_ASSIGNEE_EMAIL_MAP", `{"7101":"owner@example.com"}`)

	gitLabCalls := 0
	gitLabServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gitLabCalls++
		if r.Method != http.MethodPost {
			t.Fatalf("expected GitLab note POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":101,"body":"created"}`))
	}))
	defer gitLabServer.Close()

	var imPayload map[string]any
	restoreTransport := replaceDefaultTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.telegram.org" {
			return http.DefaultTransport.RoundTrip(r)
		}
		if err := json.NewDecoder(r.Body).Decode(&imPayload); err != nil {
			t.Fatalf("decode IM payload: %v", err)
		}
		return jsonHTTPResponse(`{"ok":true,"result":{"message_id":7}}`), nil
	})
	defer restoreTransport()

	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	cfg := config.Load()
	cfg.HTTPAddr = ":0"
	if err := people.NewRepository(db).Migrate(); err != nil {
		t.Fatalf("migrate people: %v", err)
	}
	enabled := true
	if err := people.NewRepository(db).Create(ctx, &people.Person{
		PersonID: "person-owner", DisplayName: "负责人", Email: "owner@example.com", Enabled: enabled,
	}, []people.CodeHostIdentity{{
		IdentityID: "identity-owner", PersonID: "person-owner", Provider: "gitlab",
		CodeHostBaseURL: gitLabServer.URL, ExternalUserID: "7101", Username: "owner",
	}}); err != nil {
		t.Fatalf("seed person: %v", err)
	}
	application, err := New(withTestControlPlane(t, cfg), zap.NewNop(), db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	seedAppRepository(t, db, "repo-app-clarification", "15594", "demo-group/lite-list", "https://gitlab.example.com")
	// 写回目标按仓行 base_url 解析:指向测试后端,与人员身份同址。
	if seeded, err := registeredrepo.NewStore(db).Get(ctx, "repo-app-clarification"); err != nil {
		t.Fatalf("load seeded repository: %v", err)
	} else {
		seeded.BaseURL = gitLabServer.URL
		if err := registeredrepo.NewStore(db).Update(ctx, seeded); err != nil {
			t.Fatalf("point repository at test backend: %v", err)
		}
	}

	taskRepo := repository.NewTaskRepository(db)
	runRepo := repository.NewRunRepository(db)
	writebackRepo := repository.NewWritebackRepository(db)
	effectRepo := repository.NewEffectRepository(db)
	currentRunID := "run-developer-clarification"
	repositoryID := "repo-app-clarification"
	task := &model.Task{
		TaskID:       "task-developer-clarification",
		DedupeKey:    "gitlab:issue-develop:15594:2",
		Type:         "develop_from_issue",
		Role:         "DeveloperWorker",
		TargetRepo:   "demo-group/lite-list",
		RepositoryID: &repositoryID,
		Status:       model.TaskStatusRunning,
		CurrentRunID: &currentRunID,
		ContextJSON:  `{"project":{"id":"15594","path":"demo-group/lite-list"},"issue":{"iid":2,"assignee_ids":[7101]}}`,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := runRepo.Create(ctx, &model.Run{
		RunID: "run-developer-clarification", TaskID: task.TaskID, WorkerRole: "DeveloperWorker", Status: model.RunStatusRunning,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	reason := "请补充 status 字段的允许枚举值。"
	completedTask, _, err := application.Orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID: task.TaskID, RunID: currentRunID, Status: results.StatusHumanRequired,
		Summary: "缺少接口字段定义", BlockerReason: &reason,
	})
	if err != nil {
		t.Fatalf("apply developer human-required result: %v", err)
	}
	if completedTask.Status != model.TaskStatusHumanRequired {
		t.Fatalf("expected human_required task, got %s", completedTask.Status)
	}

	writeback, err := writebackRepo.GetByDedupeKey(ctx, "developer-clarification:task-developer-clarification:run-developer-clarification")
	if err != nil {
		t.Fatalf("expected coordinator clarification note: %v", err)
	}
	if writeback.Status != model.WritebackStatusSucceeded || !strings.Contains(writeback.RequestJSON, reason) || !strings.Contains(writeback.RequestJSON, "agent-coordinator-operation") {
		t.Fatalf("unexpected clarification writeback: %#v", writeback)
	}
	if gitLabCalls != 1 {
		t.Fatalf("expected one GitLab note call, got %d", gitLabCalls)
	}

	effect, err := effectRepo.GetByDedupeKey(ctx, "developer-clarification-im:task-developer-clarification:run-developer-clarification")
	if err != nil {
		t.Fatalf("expected coordinator IM effect: %v", err)
	}
	if effect.Status != model.EffectStatusPending {
		t.Fatalf("expected pending IM effect, got %s", effect.Status)
	}
	// IM 执行面已租户化:发送行为在组织段配置接线后由专门用例锁定。
	_ = imPayload
}

type appRoundTripFunc func(*http.Request) (*http.Response, error)

func (function appRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func replaceDefaultTransport(t *testing.T, function appRoundTripFunc) func() {
	t.Helper()
	original := http.DefaultClient.Transport
	http.DefaultClient.Transport = function
	return func() { http.DefaultClient.Transport = original }
}

func jsonHTTPResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestGitLabMRReworkWebhookCreatesOneManagedPendingTask(t *testing.T) {
	db := openAppTestDB(t)
	application, err := New(withTestControlPlane(t, config.Config{
		HTTPAddr: ":0",
	}), zap.NewNop(), db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	tenant := seedTenantRoutedRepository(t, application, "acme-rework", "gitlab", "repo-app-rework", "1001", "demo-group/demo-project", "https://gitlab.example.com", "webhook-secret")
	ctx := context.Background()
	tenantStore, err := application.TenantStores.Handle(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("open tenant store: %v", err)
	}
	tenantDB := tenantStore.DB
	origin := &model.Task{
		TaskID: "task-origin-developer", DedupeKey: "gitlab:issue:1001:19:open", Type: "develop_from_issue", Role: "DeveloperWorker",
		TargetRepo: "demo-group/demo-project", Status: model.TaskStatusSucceeded,
		Objective: "Implement Issue #19", AcceptanceCriteria: "Complete the requested change.",
		ContextJSON: `{"issue":{"iid":19,"url":"https://gitlab.example/demo-group/demo-project/-/issues/19","title":"Handle null status safely","description":"Normalize null values.","assignee_ids":[42]}}`,
	}
	if err := repository.NewTaskRepository(tenantDB).Create(ctx, origin); err != nil {
		t.Fatalf("create origin task: %v", err)
	}
	writeback := &model.Writeback{
		TaskID: origin.TaskID, RunID: "run-origin-developer", DedupeKey: "developer-origin-mr", Kind: orchestrator.CodeHostWritebackCreateMergeRequest,
		ProjectID: "demo-group/demo-project", Status: model.WritebackStatusSucceeded, ExternalID: "21", ExternalURL: "https://gitlab.example/demo-group/demo-project/-/merge_requests/21",
	}
	if err := repository.NewWritebackRepository(tenantDB).Create(ctx, writeback); err != nil {
		t.Fatalf("create managed MR writeback: %v", err)
	}
	raw := readAppFixture(t, "../events/testdata/gitlab/merge_request_resume_note_created.json")

	first := performAppRawJSONWithHeaders(t, application.Router, http.MethodPost, "/api/events/gitlab", raw, appWebhookAuthHeaders())
	if first.Code != http.StatusAccepted {
		t.Fatalf("expected accepted webhook, got %d: %s", first.Code, first.Body.String())
	}
	var firstResult events.ProcessResult
	if err := json.Unmarshal(first.Body.Bytes(), &firstResult); err != nil {
		t.Fatalf("decode first result: %v", err)
	}
	if firstResult.Ignored || firstResult.Task == nil || firstResult.Task.Type != model.TaskTypeReworkMergeRequest || firstResult.Task.Status != model.TaskStatusPending {
		t.Fatalf("expected pending rework task: %#v", firstResult)
	}
	storedTask, err := repository.NewTaskRepository(tenantDB).GetByDedupeKey(ctx, "gitlab:mr-rework:1001:21")
	if err != nil {
		t.Fatalf("get stored rework task: %v", err)
	}
	if storedTask.DeliveryBranch != "developer/issue-19-20260806" || !strings.Contains(storedTask.ContextJSON, `"expected_head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`) {
		t.Fatalf("unexpected rework branch/context: %#v", storedTask)
	}

	second := performAppRawJSONWithHeaders(t, application.Router, http.MethodPost, "/api/events/gitlab", raw, appWebhookAuthHeaders())
	var secondResult events.ProcessResult
	if err := json.Unmarshal(second.Body.Bytes(), &secondResult); err != nil {
		t.Fatalf("decode duplicate result: %v", err)
	}
	if second.Code != http.StatusAccepted || !secondResult.Duplicate {
		t.Fatalf("expected duplicate receipt, got %d: %#v", second.Code, secondResult)
	}
	var taskCount int64
	if err := tenantDB.Model(&model.Task{}).Where("type = ?", model.TaskTypeReworkMergeRequest).Count(&taskCount).Error; err != nil {
		t.Fatalf("count rework tasks: %v", err)
	}
	if taskCount != 1 {
		t.Fatalf("expected one stable rework task, got %d", taskCount)
	}
}

// TestNewRejectsWebhookForUnregisteredRepo 锁定按仓验签装配:无全局 secret 可用,
// 未注册仓(无 secret 可验)的投递一律 401。
func TestNewRejectsWebhookForUnregisteredRepo(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	application, err := New(withTestControlPlane(t, config.Config{
		HTTPAddr: ":0",
	}), zap.NewNop(), db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}

	resp := performAppRawJSON(t, application.Router, http.MethodPost, "/api/events/gitlab", readAppFixture(t, "../events/testdata/gitlab/merge_request_opened.json"))
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized status %d, got %d: %s", http.StatusUnauthorized, resp.Code, resp.Body.String())
	}
	var eventCount int64
	if err := db.Model(&model.InboundEvent{}).Count(&eventCount).Error; err != nil {
		t.Fatal(err)
	}
	if eventCount != 0 {
		t.Fatalf("unverified delivery must not persist events, got %d", eventCount)
	}
}

func TestNewWebhookAdmissionWithoutConfigurationService(t *testing.T) {
	db := openAppTestDB(t)
	application, err := New(withTestControlPlane(t, config.Config{HTTPAddr: ":0"}), zap.NewNop(), db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	seedTenantRoutedRepository(t, application, "acme-admit", "gitlab", "repo-app-webhook", "15594", "demo-group/lite-list", "https://gitlab.example.com", "webhook-secret")
	resp := performAppRawJSONWithHeaders(t, application.Router, http.MethodPost, "/api/events/gitlab", readAppFixture(t, "../events/testdata/gitlab/merge_request_opened.json"), appWebhookAuthHeaders())
	if resp.Code != http.StatusAccepted || strings.Contains(resp.Body.String(), `"ignored":true`) {
		t.Fatalf("app without configuration service must still admit webhooks: %d %s", resp.Code, resp.Body.String())
	}
}

// 按仓验签是唯一准入:未注册仓的 webhook 在验签处被 401 拒绝,零落库;
// 注册并补配 secret 后平台重发即可恢复(不再有 repository_not_registered deferred)。
func TestNewReadyRejectsUnregisteredWebhookWithoutPersisting(t *testing.T) {
	db := openAppTestDB(t)
	configRepo := configuration.NewRepository(db)
	if err := configRepo.Migrate(); err != nil {
		t.Fatalf("migrate configuration: %v", err)
	}
	runtimeConfig := configuration.RuntimeConfig{}
	saved, err := configRepo.Save(context.Background(), configuration.StatusReady, configuration.SourceUI, runtimeConfig)
	if err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	service := configuration.NewService(configRepo, configuration.Resolution{
		Mode: configuration.ModeReady, Prefill: runtimeConfig, Snapshot: saved,
	})
	application, err := NewReady(withTestControlPlane(t, config.Config{
		HTTPAddr: ":0",
	}), zap.NewNop(), db, service, nil)
	if err != nil {
		t.Fatalf("new ready app: %v", err)
	}

	resp := performAppRawJSONWithHeaders(t, application.Router, http.MethodPost, "/api/events/gitlab", readAppFixture(t, "../events/testdata/gitlab/merge_request_opened.json"), appWebhookAuthHeaders())
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized webhook, got %d: %s", resp.Code, resp.Body.String())
	}
	var eventCount int64
	if err := db.Model(&model.InboundEvent{}).Count(&eventCount).Error; err != nil {
		t.Fatal(err)
	}
	if eventCount != 0 {
		t.Fatalf("unverified delivery must not persist events, got %d", eventCount)
	}
}

func appWebhookAuthHeaders() map[string]string {
	return map[string]string{"X-Gitlab-Token": "webhook-secret"}
}

func performAppRawJSONWithHeaders(t *testing.T, router http.Handler, method, path string, raw []byte, headers map[string]string) *httptest.ResponseRecorder {
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

func performAppRawJSON(t *testing.T, router http.Handler, method, path string, raw []byte) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	return rec
}

func readAppFixture(t *testing.T, path string) []byte {
	t.Helper()

	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return raw
}
