package registeredrepo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	storerepo "sdlc-coordinator/internal/store/repository"
)

type fakeProbe struct {
	calls   int
	ident   ProjectIdentity
	err     error
	token   string
	baseURL string
}

func (f *fakeProbe) GetProject(_ context.Context, baseURL, token, path string) (ProjectIdentity, error) {
	f.calls++
	f.token = token
	f.baseURL = baseURL
	if f.err != nil {
		return ProjectIdentity{}, f.err
	}
	if f.ident.PathWithNamespace == "" {
		f.ident.PathWithNamespace = path
	}
	return f.ident, nil
}

type fakeActivity struct {
	busy bool
}

func (f fakeActivity) HasActiveWork(context.Context, string) (bool, error) {
	return f.busy, nil
}

func newServiceWithProbe(t *testing.T) (*Service, *fakeProbe) {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repoStore := NewStore(db)
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	probe := &fakeProbe{ident: ProjectIdentity{ID: "24207", PathWithNamespace: "group/a", DefaultBranch: "master"}}
	svc := NewService(repoStore, nil, fakeActivity{})
	svc.WithProbeFactory(func(string) ProjectProbe { return probe })
	return svc, probe
}

func TestCreateRejectsUnattestedTokenAndDoesNotEchoSecret(t *testing.T) {
	svc, probe := newServiceWithProbe(t)
	_, err := svc.Create(context.Background(), CreateRequest{
		DisplayName:       "A",
		PathWithNamespace: "group/a",
		Token:             "super-secret-token",
	})
	if !errors.Is(err, ErrTokenNotAttested) {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), "super-secret-token") {
		t.Fatal("token leaked in error")
	}
	if probe.calls != 0 {
		t.Fatal("must not probe before attestation")
	}
}

func TestCreateRequiresWebhookSecret(t *testing.T) {
	svc, probe := newServiceWithProbe(t)
	_, err := svc.Create(context.Background(), CreateRequest{
		DisplayName:       "A",
		PathWithNamespace: "group/a",
		Provider:          "gitlab",
		BaseURL:           "https://gitlab.example.com", Token: "token-a",
		TokenScopeAttested: true,
	})
	if !errors.Is(err, ErrWebhookSecretRequired) {
		t.Fatalf("got %v", err)
	}
	if probe.calls != 0 {
		t.Fatal("must not probe before webhook secret is configured")
	}
}

// TestCreateValidatesPlatformIdentity 锁定注册面校验:provider 必须是受支持
// 平台(规范化小写),base_url 必须是 http(s) origin 并归一化存储。
func TestCreateValidatesPlatformIdentity(t *testing.T) {
	svc, probe := newServiceWithProbe(t)
	base := CreateRequest{
		DisplayName:        "A",
		PathWithNamespace:  "group/a",
		Token:              "token-a",
		TokenScopeAttested: true,
		WebhookSecret:      "hook-secret-a",
	}
	if _, err := svc.Create(context.Background(), cloneRequest(base, "gitbucket", "https://gitlab.example.com")); !errors.Is(err, ErrInvalidPlatformIdentity) {
		t.Fatalf("invalid provider: %v", err)
	}
	if _, err := svc.Create(context.Background(), cloneRequest(base, "gitlab", "not-a-url")); !errors.Is(err, ErrInvalidPlatformIdentity) {
		t.Fatalf("invalid base_url: %v", err)
	}
	if _, err := svc.Create(context.Background(), cloneRequest(base, "gitlab", "ftp://gitlab.example.com")); !errors.Is(err, ErrInvalidPlatformIdentity) {
		t.Fatalf("non-http base_url: %v", err)
	}
	if probe.calls != 0 {
		t.Fatal("must not probe before platform identity is valid")
	}
	view, err := svc.Create(context.Background(), cloneRequest(base, "GitLab", "https://GitLab.example.com/"))
	if err != nil {
		t.Fatalf("normalized create: %v", err)
	}
	if view.Provider != "gitlab" || view.BaseURL != "https://gitlab.example.com" {
		t.Fatalf("identity not normalized: %+v", view)
	}
	if probe.baseURL != "https://gitlab.example.com" {
		t.Fatalf("probe must target repository base_url, got %q", probe.baseURL)
	}
}

func cloneRequest(base CreateRequest, provider, baseURL string) CreateRequest {
	request := base
	request.Provider, request.BaseURL = provider, baseURL
	return request
}

// TestEnableRequiresPlatformIdentity 缺少 provider/base_url 的仓行不能启用。
func TestEnableRequiresPlatformIdentity(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	legacy := &RegisteredRepository{
		DisplayName: "legacy", PathWithNamespace: "group/legacy",
		Token: "token-a", TokenScopeAttested: true, WebhookSecret: "hook-secret",
	}
	if err := svc.store.Create(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Enable(context.Background(), legacy.RepositoryID); !errors.Is(err, ErrPlatformIdentityRequired) {
		t.Fatalf("got %v", err)
	}
	got, err := svc.Get(context.Background(), legacy.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status == StatusEnabled {
		t.Fatal("must not enable without platform identity")
	}
}

// TestPatchPlatformChangeReacquiresIdentity 锁定平台迁移路径:disabled 仓换
// 平台后,旧平台的数字 project id 不再可比,refreshIdentity 按新平台重新获取
// 身份(清空锚点重探),不得误判 repository_identity_mismatch 卡死迁移。
func TestPatchPlatformChangeReacquiresIdentity(t *testing.T) {
	svc, gitlabProbe := newServiceWithProbe(t)
	view, err := svc.Create(context.Background(), CreateRequest{
		DisplayName:        "A",
		PathWithNamespace:  "group/a",
		Provider:           "gitlab",
		BaseURL:            "https://gitlab.example.com",
		Token:              "token-a",
		TokenScopeAttested: true,
		WebhookSecret:      "hook-secret-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.CodeHostProjectID != "24207" {
		t.Fatalf("expected gitlab identity acquired, got %q", view.CodeHostProjectID)
	}
	stored, err := svc.store.Get(context.Background(), view.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	stored.Status = StatusDisabled
	if err := svc.store.Update(context.Background(), stored); err != nil {
		t.Fatal(err)
	}

	giteaProbe := &fakeProbe{ident: ProjectIdentity{ID: "9876", PathWithNamespace: "group/a", DefaultBranch: "main"}}
	svc.WithProbeFactory(func(provider string) ProjectProbe {
		if provider == "gitea" {
			return giteaProbe
		}
		return gitlabProbe
	})
	giteaProvider := "gitea"
	newURL := "https://gitea.example.com/"
	patched, err := svc.Patch(context.Background(), view.RepositoryID, PatchRequest{Provider: &giteaProvider, BaseURL: &newURL})
	if err != nil {
		t.Fatalf("platform change must succeed: %v", err)
	}
	if patched.Provider != "gitea" || patched.BaseURL != "https://gitea.example.com" {
		t.Fatalf("platform identity not migrated: %+v", patched)
	}
	reloaded, err := svc.store.Get(context.Background(), view.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.CodeHostProjectID != "9876" {
		t.Fatalf("identity must be reacquired on the new platform, got %q", reloaded.CodeHostProjectID)
	}
}

// TestPatchPlatformIdentityOnEnabledRequiresDisable 锁定:enabled 仓改平台身份
// 与改 token/path 同级,必须先停用(平台身份变更会打断运行中任务的写回目标)。
func TestPatchPlatformIdentityOnEnabledRequiresDisable(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	view, err := svc.Create(context.Background(), CreateRequest{
		DisplayName:        "A",
		PathWithNamespace:  "group/a",
		Provider:           "gitlab",
		BaseURL:            "https://gitlab.example.com",
		Token:              "token-a",
		TokenScopeAttested: true,
		WebhookSecret:      "hook-secret-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := svc.store.Get(context.Background(), view.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	stored.Status = StatusEnabled
	if err := svc.store.Update(context.Background(), stored); err != nil {
		t.Fatal(err)
	}
	giteaProvider := "gitea"
	if _, err := svc.Patch(context.Background(), view.RepositoryID, PatchRequest{Provider: &giteaProvider}); !errors.Is(err, ErrMustDisableFirst) {
		t.Fatalf("provider change: %v", err)
	}
	newURL := "https://gitea.example.com"
	if _, err := svc.Patch(context.Background(), view.RepositoryID, PatchRequest{BaseURL: &newURL}); !errors.Is(err, ErrMustDisableFirst) {
		t.Fatalf("base_url change: %v", err)
	}
}

// TestEnableRequiresWebhookSecret 缺少 webhook secret 的仓行不能启用。
func TestEnableRequiresWebhookSecret(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	legacy := &RegisteredRepository{
		DisplayName: "legacy", PathWithNamespace: "group/legacy",
		Token: "token-a", TokenScopeAttested: true,
	}
	if err := svc.store.Create(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Enable(context.Background(), legacy.RepositoryID); !errors.Is(err, ErrWebhookSecretRequired) {
		t.Fatalf("got %v", err)
	}
	got, err := svc.Get(context.Background(), legacy.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status == StatusEnabled {
		t.Fatal("must not enable without webhook secret")
	}
}

func TestPatchRotatesWebhookSecret(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	view, err := svc.Create(context.Background(), CreateRequest{
		DisplayName:       "A",
		PathWithNamespace: "group/a",
		Provider:          "gitlab",
		BaseURL:           "https://gitlab.example.com", Token: "token-a",
		TokenScopeAttested: true,
		WebhookSecret:      "hook-secret-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	patched, err := svc.Patch(context.Background(), view.RepositoryID, PatchRequest{WebhookSecret: "hook-secret-b"})
	if err != nil {
		t.Fatal(err)
	}
	if !patched.WebhookSecretConfigured {
		t.Fatal("expected webhook_secret_configured after rotation")
	}
	stored, err := svc.store.Get(context.Background(), view.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.WebhookSecret != "hook-secret-b" {
		t.Fatalf("stored secret=%q", stored.WebhookSecret)
	}
	// 空值表示保持不变(与 token 同语义),不支持清空。
	name := "renamed"
	if _, err := svc.Patch(context.Background(), view.RepositoryID, PatchRequest{DisplayName: &name}); err != nil {
		t.Fatal(err)
	}
	stored, err = svc.store.Get(context.Background(), view.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.WebhookSecret != "hook-secret-b" {
		t.Fatalf("blank patch must keep secret, got %q", stored.WebhookSecret)
	}
}

func TestViewDoesNotEchoWebhookSecret(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	view, err := svc.Create(context.Background(), CreateRequest{
		DisplayName:       "A",
		PathWithNamespace: "group/a",
		Provider:          "gitlab",
		BaseURL:           "https://gitlab.example.com", Token: "token-a",
		TokenScopeAttested: true,
		WebhookSecret:      "hook-secret-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !view.WebhookSecretConfigured {
		t.Fatal("expected webhook_secret_configured")
	}
	if strings.Contains(fmt.Sprintf("%+v", view), "hook-secret-a") {
		t.Fatal("view leaked webhook secret")
	}
}

func TestCreateStaysConfiguringAfterSuccessfulProbe(t *testing.T) {
	svc, probe := newServiceWithProbe(t)
	view, err := svc.Create(context.Background(), CreateRequest{
		DisplayName:       "A",
		PathWithNamespace: "group/a",
		Provider:          "gitlab",
		BaseURL:           "https://gitlab.example.com", Token: "super-secret-token",
		TokenScopeAttested: true,
		WebhookSecret:      "hook-secret-a",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if view.Status != StatusConfiguring {
		t.Fatalf("status=%s", view.Status)
	}
	encoded := fmt.Sprintf("%+v", view)
	if strings.Contains(encoded, "super-secret-token") {
		t.Fatal("view leaked token")
	}
	if !view.TokenConfigured {
		t.Fatal("expected token_configured")
	}
	if view.CodeHostProjectID != "24207" || view.DefaultBranch != "master" {
		t.Fatalf("identity %+v", view)
	}
	if probe.calls != 1 {
		t.Fatalf("probe calls=%d", probe.calls)
	}
}

func TestCreatePersistsIssueLabels(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	view, err := svc.Create(context.Background(), CreateRequest{
		DisplayName:       "A",
		PathWithNamespace: "group/a",
		Provider:          "gitlab",
		BaseURL:           "https://gitlab.example.com", Token: "token-a",
		TokenScopeAttested:  true,
		WebhookSecret:       "hook-secret-a",
		CodeHostIssueLabels: []string{" developer-agent ", "bugfix", "developer-agent"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(view.CodeHostIssueLabels) != 2 || view.CodeHostIssueLabels[0] != "developer-agent" || view.CodeHostIssueLabels[1] != "bugfix" {
		t.Fatalf("labels=%#v", view.CodeHostIssueLabels)
	}
	stored, err := svc.store.Get(context.Background(), view.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.CodeHostIssueLabels != `["developer-agent","bugfix"]` {
		t.Fatalf("stored labels=%q", stored.CodeHostIssueLabels)
	}
}

func TestPatchCanExplicitlyClearIssueLabels(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	view, err := svc.Create(context.Background(), CreateRequest{
		DisplayName:       "A",
		PathWithNamespace: "group/a",
		Provider:          "gitlab",
		BaseURL:           "https://gitlab.example.com", Token: "token-a",
		TokenScopeAttested:  true,
		WebhookSecret:       "hook-secret-a",
		CodeHostIssueLabels: []string{"developer-agent"},
	})
	if err != nil {
		t.Fatal(err)
	}
	empty := []string{}
	updated, err := svc.Patch(context.Background(), view.RepositoryID, PatchRequest{CodeHostIssueLabels: &empty})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.CodeHostIssueLabels) != 0 {
		t.Fatalf("labels=%#v", updated.CodeHostIssueLabels)
	}
	stored, err := svc.store.Get(context.Background(), view.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.CodeHostIssueLabels != "[]" {
		t.Fatalf("stored labels=%q", stored.CodeHostIssueLabels)
	}
}

func TestCreateProbeFailureKeepsConfiguringWithoutLeakingToken(t *testing.T) {
	svc, probe := newServiceWithProbe(t)
	probe.err = errors.New("gitlab 401 token=super-secret-token")
	view, err := svc.Create(context.Background(), CreateRequest{
		DisplayName:       "A",
		PathWithNamespace: "group/a",
		Provider:          "gitlab",
		BaseURL:           "https://gitlab.example.com", Token: "super-secret-token",
		TokenScopeAttested: true,
		WebhookSecret:      "hook-secret-a",
	})
	if err == nil {
		t.Fatal("expected probe error")
	}
	if strings.Contains(err.Error(), "super-secret-token") {
		t.Fatal("token leaked in error")
	}
	if view == nil {
		t.Fatal("expected persisted view")
	}
	if view.Status != StatusConfiguring {
		t.Fatalf("status=%s", view.Status)
	}
	if strings.Contains(view.LastConfigurationError, "super-secret-token") {
		t.Fatal("token leaked in last_configuration_error")
	}
}

func TestPatchTokenOnEnabledRequiresDisable(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	view, err := svc.Create(context.Background(), CreateRequest{
		DisplayName:       "A",
		PathWithNamespace: "group/a",
		Provider:          "gitlab",
		BaseURL:           "https://gitlab.example.com", Token: "token-a",
		TokenScopeAttested: true,
		WebhookSecret:      "hook-secret-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := svc.store.Get(context.Background(), view.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	stored.Status = StatusEnabled
	if err := svc.store.Update(context.Background(), stored); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Patch(context.Background(), view.RepositoryID, PatchRequest{Token: "token-b"})
	if !errors.Is(err, ErrMustDisableFirst) {
		t.Fatalf("got %v", err)
	}
}

func TestDisableReturnsBusyWhenRunIsActive(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	svc.activity = fakeActivity{busy: true}
	view, err := svc.Create(context.Background(), CreateRequest{
		DisplayName:       "A",
		PathWithNamespace: "group/a",
		Provider:          "gitlab",
		BaseURL:           "https://gitlab.example.com", Token: "token-a",
		TokenScopeAttested: true,
		WebhookSecret:      "hook-secret-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := svc.store.Get(context.Background(), view.RepositoryID)
	stored.Status = StatusEnabled
	_ = svc.store.Update(context.Background(), stored)
	err = svc.Disable(context.Background(), view.RepositoryID)
	if !errors.Is(err, ErrRepositoryBusy) {
		t.Fatalf("got %v", err)
	}
	got, _ := svc.Get(context.Background(), view.RepositoryID)
	if got.Status != StatusEnabled {
		t.Fatalf("status changed to %s", got.Status)
	}
}

func TestDisableSucceedsWhenIdle(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	view, err := svc.Create(context.Background(), CreateRequest{
		DisplayName:       "A",
		PathWithNamespace: "group/a",
		Provider:          "gitlab",
		BaseURL:           "https://gitlab.example.com", Token: "token-a",
		TokenScopeAttested: true,
		WebhookSecret:      "hook-secret-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := svc.store.Get(context.Background(), view.RepositoryID)
	stored.Status = StatusEnabled
	_ = svc.store.Update(context.Background(), stored)
	if err := svc.Disable(context.Background(), view.RepositoryID); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(context.Background(), view.RepositoryID)
	if got.Status != StatusDisabled {
		t.Fatalf("status=%s", got.Status)
	}
}

func TestPatchPathRejectsProjectRedirect(t *testing.T) {
	svc, probe := newServiceWithProbe(t)
	view, err := svc.Create(context.Background(), CreateRequest{
		DisplayName:       "A",
		PathWithNamespace: "group/a",
		Provider:          "gitlab",
		BaseURL:           "https://gitlab.example.com", Token: "token-a",
		TokenScopeAttested: true,
		WebhookSecret:      "hook-secret-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := svc.store.Get(context.Background(), view.RepositoryID)
	stored.Status = StatusDisabled
	_ = svc.store.Update(context.Background(), stored)
	probe.ident = ProjectIdentity{ID: "99999", PathWithNamespace: "group/renamed", DefaultBranch: "master"}
	path := "group/renamed"
	_, err = svc.Patch(context.Background(), view.RepositoryID, PatchRequest{PathWithNamespace: &path})
	if !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("got %v", err)
	}
}

func TestSQLActivityCheckerDetectsRunningTask(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}); err != nil {
		t.Fatal(err)
	}
	repoID := "repo_busy"
	task := &model.Task{
		DedupeKey:    "busy",
		Type:         "develop_from_issue",
		Role:         "DeveloperWorker",
		Status:       model.TaskStatusRunning,
		RepositoryID: &repoID,
	}
	if err := storerepo.NewTaskRepository(db).Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	checker := NewSQLActivityChecker(db)
	busy, err := checker.HasActiveWork(context.Background(), repoID)
	if err != nil {
		t.Fatal(err)
	}
	if !busy {
		t.Fatal("expected busy")
	}
	idle, err := checker.HasActiveWork(context.Background(), "repo_other")
	if err != nil {
		t.Fatal(err)
	}
	if idle {
		t.Fatal("other repo must not be busy")
	}
}

// TestEnableCompletesWhenIdentityProbePasses Enable 在身份探活通过后将仓库置为 enabled。
func TestEnableCompletesWhenIdentityProbePasses(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	created, err := svc.Create(context.Background(), CreateRequest{
		DisplayName:        "A",
		PathWithNamespace:  "group/a",
		Provider:           "gitlab",
		BaseURL:            "https://gitlab.example.com",
		Token:              "super-secret-token",
		TokenScopeAttested: true,
		WebhookSecret:      "hook-secret-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.Enable(context.Background(), created.RepositoryID)
	if err != nil {
		t.Fatalf("enable without secrets must succeed: %v", err)
	}
	if view.Status != StatusEnabled || view.LastConfigurationError != "" {
		t.Fatalf("unexpected enabled view: %+v", view)
	}
}

type fakeRouteIndexSync struct {
	ownerTenantID uint
	found         bool
	lookupErr     error
}

func (f *fakeRouteIndexSync) Upsert(context.Context, uint, RouteIndexEntry) error { return nil }
func (f *fakeRouteIndexSync) Delete(context.Context, RouteIndexEntry) error       { return nil }
func (f *fakeRouteIndexSync) Lookup(context.Context, RouteIndexEntry) (uint, bool, error) {
	return f.ownerTenantID, f.found, f.lookupErr
}

func validCreateRequest() CreateRequest {
	return CreateRequest{
		PathWithNamespace:  "group/a",
		Provider:           "gitlab",
		BaseURL:            "https://gitlab.example.com",
		Token:              "token-a",
		TokenScopeAttested: true,
		WebhookSecret:      "hook-secret-a",
	}
}

// 路由身份被其他租户占用时注册必须显式拒绝，且拒绝发生在租户库写入之前。
func TestCreateRejectsRouteIdentityClaimedByAnotherTenant(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	svc.SetRouteIndexSync(&fakeRouteIndexSync{ownerTenantID: 7, found: true}, 2)

	_, err := svc.Create(context.Background(), validCreateRequest())
	if !errors.Is(err, ErrRouteIdentityClaimed) {
		t.Fatalf("expected route identity claimed error, got %v", err)
	}
	repos, listErr := svc.List(context.Background())
	if listErr != nil || len(repos) != 0 {
		t.Fatalf("rejected create must not write the tenant row: repos=%d err=%v", len(repos), listErr)
	}
}

// 本租户重复认领同一身份（改 secret 等再注册场景）必须放行。
func TestCreateAllowsReclaimingOwnRouteIdentity(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	svc.SetRouteIndexSync(&fakeRouteIndexSync{ownerTenantID: 2, found: true}, 2)

	view, err := svc.Create(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatalf("same-tenant identity must pass: %v", err)
	}
	if view.PathWithNamespace != "group/a" {
		t.Fatalf("unexpected view: %#v", view)
	}
}

// 改仓换身份同样预检：占用方为其他租户时拒绝，不写租户库。
func TestPatchRejectsRouteIdentityClaimedByAnotherTenant(t *testing.T) {
	svc, _ := newServiceWithProbe(t)
	view, err := svc.Create(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	svc.SetRouteIndexSync(&fakeRouteIndexSync{ownerTenantID: 7, found: true}, 2)

	newPath := "group/other"
	_, err = svc.Patch(context.Background(), view.RepositoryID, PatchRequest{PathWithNamespace: &newPath})
	if !errors.Is(err, ErrRouteIdentityClaimed) {
		t.Fatalf("expected route identity claimed error, got %v", err)
	}
}
