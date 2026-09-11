package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"sdlc-coordinator/internal/events"

	"github.com/gin-gonic/gin"
)

func hmacSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func githubSignatureHeader(secret string, body []byte) string {
	return "sha256=" + hmacSignature(secret, body)
}

type secretLookupKey struct {
	provider string
	path     string
	repoURL  string
}

type fakeSecretLookup struct {
	secrets map[secretLookupKey]string
	lookups []secretLookupKey
}

// WebhookSecretByPath 按 (provider, path) 命中 secret（repoURL 不参与，
// 与真实实现一致——仅多行路由时消费）；repoURL 随每次调用记录，供
// 贯穿断言核对 handler 是否把载荷完整链接传到了查找端。
func (f *fakeSecretLookup) WebhookSecretByPath(_ context.Context, provider, path, repoURL string) (string, error) {
	f.lookups = append(f.lookups, secretLookupKey{provider: provider, path: path, repoURL: repoURL})
	secret, ok := f.secrets[secretLookupKey{provider: provider, path: path}]
	if !ok {
		// 与真实实现对齐：未注册 = 空串无错误（401）；err 保留给基础设施抖动。
		return "", nil
	}
	return secret, nil
}

func newEventRouter(lookup WebhookSecretLookup) (*gin.Engine, *fakeSecretLookup) {
	fake := &fakeSecretLookup{secrets: map[secretLookupKey]string{
		{provider: "gitlab", path: "group/repo-a"}: "secret-a",
		{provider: "gitea", path: "group/repo-a"}:  "secret-gitea-a",
		{provider: "github", path: "group/repo-a"}: "secret-github-a",
		{provider: "gitlab", path: "group/repo-b"}: "secret-b",
		{provider: "gitlab", path: "group/empty"}:  "",
	}}
	if lookup == nil {
		lookup = fake
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewEventHandler(nil, WithWebhookSecretLookup(lookup))
	router.POST("/api/events/gitlab", handler.GitLab)
	router.POST("/api/events/gitea", handler.Gitea)
	router.POST("/api/events/github", handler.GitHub)
	return router, fake
}

func performEvent(t *testing.T, router http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestGitLabWebhookVerifiesAgainstRepoSecret(t *testing.T) {
	router, _ := newEventRouter(nil)
	payload := `{"project":{"path_with_namespace":"group/repo-a","web_url":"https://gitlab.example.com/group/repo-a"},"object_kind":"merge_request"}`

	// 本仓 secret:验签通过,到达业务层(processor 未配置 500),不被鉴权拦截。
	accepted := performEvent(t, router, http.MethodPost, "/api/events/gitlab", payload, map[string]string{"X-Gitlab-Token": "secret-a"})
	if accepted.Code != http.StatusInternalServerError {
		t.Fatalf("valid repo secret must reach business layer, got %d: %s", accepted.Code, accepted.Body.String())
	}

	// 他仓 secret:仓间隔离,拒绝。
	other := performEvent(t, router, http.MethodPost, "/api/events/gitlab", payload, map[string]string{"X-Gitlab-Token": "secret-b"})
	if other.Code != http.StatusUnauthorized {
		t.Fatalf("cross-repo secret must be 401, got %d", other.Code)
	}

	// 缺失 token:拒绝。
	missing := performEvent(t, router, http.MethodPost, "/api/events/gitlab", payload, nil)
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("missing token must be 401, got %d", missing.Code)
	}
}

func TestGitLabWebhookRejectsWhenEngineViewFactoryFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	failing := gin.New()
	handler := NewEventHandler(nil,
		WithWebhookSecretLookup(&fakeSecretLookup{secrets: map[secretLookupKey]string{
			{provider: "gitlab", path: "group/repo-a"}: "secret-a",
		}}),
		WithEngineViewFactory(func(context.Context, string, string, string) (*WebhookEngineView, error) {
			return nil, errors.New("tenant runtime config unavailable")
		}),
	)
	failing.POST("/api/events/gitlab", handler.GitLab)

	payload := `{"project":{"path_with_namespace":"group/repo-a"},"object_kind":"merge_request"}`
	response := performEvent(t, failing, http.MethodPost, "/api/events/gitlab", payload, map[string]string{"X-Gitlab-Token": "secret-a"})
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("cold config read failure must reject with 503, got %d: %s", response.Code, response.Body.String())
	}
}

func TestGitLabWebhookRejectsUnregisteredRepo(t *testing.T) {
	router, lookup := newEventRouter(nil)
	payload := `{"project":{"path_with_namespace":"group/unknown","web_url":"https://gitlab.example.com/group/unknown"},"object_kind":"push"}`
	resp := performEvent(t, router, http.MethodPost, "/api/events/gitlab", payload, map[string]string{"X-Gitlab-Token": "anything"})
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("unregistered repo must be 401, got %d: %s", resp.Code, resp.Body.String())
	}
	if lookup.lookups[len(lookup.lookups)-1] != (secretLookupKey{provider: "gitlab", path: "group/unknown", repoURL: "https://gitlab.example.com/group/unknown"}) {
		t.Fatalf("lookup=%+v", lookup.lookups[len(lookup.lookups)-1])
	}
}

// TestWebhookSecretLookupIsProviderScoped 锁定 provider 作用域:跨平台同 path
// 仓各持各的 secret,A 平台 endpoint 用 B 平台同 path 仓的 secret 必须拒绝。
func TestWebhookSecretLookupIsProviderScoped(t *testing.T) {
	router, _ := newEventRouter(nil)
	gitlabPayload := `{"project":{"path_with_namespace":"group/repo-a","web_url":"https://gitlab.example.com/group/repo-a"},"object_kind":"merge_request"}`
	codeHostPayload := `{"action":"opened","repository":{"full_name":"group/repo-a","html_url":"https://gitea.example.com/group/repo-a"}}`

	crossFromGitea := performEvent(t, router, http.MethodPost, "/api/events/gitlab", gitlabPayload, map[string]string{"X-Gitlab-Token": "secret-gitea-a"})
	if crossFromGitea.Code != http.StatusUnauthorized {
		t.Fatalf("gitlab endpoint with gitea-scoped secret must be 401, got %d", crossFromGitea.Code)
	}
	crossFromGitLab := performEvent(t, router, http.MethodPost, "/api/events/gitea", codeHostPayload, map[string]string{
		"X-Gitea-Signature": hmacSignature("secret-a", []byte(codeHostPayload)),
	})
	if crossFromGitLab.Code != http.StatusUnauthorized {
		t.Fatalf("gitea endpoint with gitlab-scoped secret must be 401, got %d", crossFromGitLab.Code)
	}
	ownGitea := performEvent(t, router, http.MethodPost, "/api/events/gitea", codeHostPayload, map[string]string{
		"X-Gitea-Signature": hmacSignature("secret-gitea-a", []byte(codeHostPayload)),
	})
	if ownGitea.Code != http.StatusInternalServerError {
		t.Fatalf("gitea endpoint with own scoped secret must reach business layer, got %d", ownGitea.Code)
	}
	ownGitHub := performEvent(t, router, http.MethodPost, "/api/events/github", codeHostPayload, map[string]string{
		"X-Hub-Signature-256": githubSignatureHeader("secret-github-a", []byte(codeHostPayload)),
	})
	if ownGitHub.Code != http.StatusInternalServerError {
		t.Fatalf("github endpoint with own scoped secret must reach business layer, got %d", ownGitHub.Code)
	}
}

func TestGitLabWebhookRejectsRepoWithoutSecret(t *testing.T) {
	router, _ := newEventRouter(nil)
	payload := `{"project":{"path_with_namespace":"group/empty","web_url":"https://gitlab.example.com/group/empty"},"object_kind":"push"}`
	resp := performEvent(t, router, http.MethodPost, "/api/events/gitlab", payload, map[string]string{"X-Gitlab-Token": "secret-a"})
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("repo without secret must be 401 (fail-closed), got %d", resp.Code)
	}
}

func TestGitLabWebhookRejectsMalformedPayload(t *testing.T) {
	router, _ := newEventRouter(nil)
	resp := performEvent(t, router, http.MethodPost, "/api/events/gitlab", `{"object_kind":"push"}`, map[string]string{"X-Gitlab-Token": "secret-a"})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("malformed payload must be 400, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestGiteaWebhookVerifiesAgainstRepoSecret(t *testing.T) {
	router, _ := newEventRouter(nil)
	payload := `{"action":"opened","repository":{"full_name":"group/repo-a","html_url":"https://gitea.example.com/group/repo-a"}}`

	accepted := performEvent(t, router, http.MethodPost, "/api/events/gitea", payload, map[string]string{
		"X-Gitea-Signature": hmacSignature("secret-gitea-a", []byte(payload)),
	})
	if accepted.Code != http.StatusInternalServerError {
		t.Fatalf("valid repo signature must reach business layer, got %d: %s", accepted.Code, accepted.Body.String())
	}

	// 他仓 secret 签名:拒绝(B 仓 secret 算出的签名不能通过 A 仓验签)。
	cross := performEvent(t, router, http.MethodPost, "/api/events/gitea", payload, map[string]string{
		"X-Gitea-Signature": hmacSignature("secret-b", []byte(payload)),
	})
	if cross.Code != http.StatusUnauthorized {
		t.Fatalf("cross-repo signature must be 401, got %d", cross.Code)
	}

	missing := performEvent(t, router, http.MethodPost, "/api/events/gitea", payload, map[string]string{
		"X-Gitea-Event": "pull_request", "X-Gitea-Delivery": "delivery-1",
	})
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("missing signature must be 401, got %d", missing.Code)
	}
}

func TestGiteaWebhookRejectsUnregisteredRepoAndMalformedPayload(t *testing.T) {
	router, _ := newEventRouter(nil)
	unregisteredBody := `{"repository":{"full_name":"group/unknown","html_url":"https://gitea.example.com/group/unknown"}}`
	unregistered := performEvent(t, router, http.MethodPost, "/api/events/gitea", unregisteredBody, map[string]string{
		"X-Gitea-Signature": hmacSignature("anything", []byte(unregisteredBody)),
	})
	if unregistered.Code != http.StatusUnauthorized {
		t.Fatalf("unregistered repo must be 401, got %d", unregistered.Code)
	}
	malformedBody := `{"action":"opened"}`
	malformed := performEvent(t, router, http.MethodPost, "/api/events/gitea", malformedBody, map[string]string{
		"X-Gitea-Signature": hmacSignature("anything", []byte(malformedBody)),
	})
	if malformed.Code != http.StatusBadRequest {
		t.Fatalf("malformed payload must be 400, got %d: %s", malformed.Code, malformed.Body.String())
	}
}

func TestGitHubWebhookVerifiesAgainstRepoSecret(t *testing.T) {
	router, _ := newEventRouter(nil)
	payload := `{"action":"opened","repository":{"full_name":"group/repo-a","html_url":"https://github.com/group/repo-a"}}`

	accepted := performEvent(t, router, http.MethodPost, "/api/events/github", payload, map[string]string{
		"X-Hub-Signature-256": githubSignatureHeader("secret-github-a", []byte(payload)),
	})
	if accepted.Code != http.StatusInternalServerError {
		t.Fatalf("valid repo signature must reach business layer, got %d: %s", accepted.Code, accepted.Body.String())
	}

	cross := performEvent(t, router, http.MethodPost, "/api/events/github", payload, map[string]string{
		"X-Hub-Signature-256": githubSignatureHeader("secret-b", []byte(payload)),
	})
	if cross.Code != http.StatusUnauthorized {
		t.Fatalf("cross-repo signature must be 401, got %d", cross.Code)
	}

	// secret 为空的仓:即使签名出自其他有效 secret 也拒绝。
	emptyBody := `{"repository":{"full_name":"group/empty","html_url":"https://github.com/group/empty"}}`
	empty := performEvent(t, router, http.MethodPost, "/api/events/github", emptyBody, map[string]string{
		"X-Hub-Signature-256": githubSignatureHeader("secret-a", []byte(emptyBody)),
	})
	if empty.Code != http.StatusUnauthorized {
		t.Fatalf("repo without secret must be 401 (fail-closed), got %d", empty.Code)
	}

	malformedBody := `{"action":"opened"}`
	malformed := performEvent(t, router, http.MethodPost, "/api/events/github", malformedBody, map[string]string{
		"X-Hub-Signature-256": githubSignatureHeader("secret-a", []byte(malformedBody)),
	})
	if malformed.Code != http.StatusBadRequest {
		t.Fatalf("malformed payload must be 400, got %d: %s", malformed.Code, malformed.Body.String())
	}
}

// TestWebhookSecretLookupFailureIsFailClosed 锁定枚举防护:lookup 出错(仓储抖动)
// 不得放行、不得落库；基础设施故障与「未注册」(401) 区分为 5xx，让代码
// 托管按可重试处理。
func TestWebhookSecretLookupFailureIsFailClosed(t *testing.T) {
	router, _ := newEventRouter(&failingSecretLookup{})
	resp := performEvent(t, router, http.MethodPost, "/api/events/gitlab", `{"project":{"path_with_namespace":"group/repo-a","web_url":"https://gitlab.example.com/group/repo-a"}}`, map[string]string{"X-Gitlab-Token": "secret-a"})
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("lookup failure must be 503 (retryable, still unauthorized), got %d", resp.Code)
	}
}

type failingSecretLookup struct{}

func (failingSecretLookup) WebhookSecretByPath(context.Context, string, string, string) (string, error) {
	return "", errors.New("db down")
}

// 工厂三态语义:命中用工厂产物;工厂零值(未注入工厂的装配,生产工厂已
// 不产生零值——未路由在验签层即被拒)回落构造期处理器;引擎不可用则
// 上抛错误。
func TestResolveProcessorDistinguishesMissFromUnavailable(t *testing.T) {
	platform := &events.Processor{}
	miss := NewEventHandler(platform, WithEngineViewFactory(func(context.Context, string, string, string) (*WebhookEngineView, error) {
		return nil, nil
	}))
	if view, err := miss.resolveEngineView(context.Background(), providerGitLab, "group/repo-a", ""); err != nil || view != miss.platformView {
		t.Fatalf("miss must fall back to the platform view, got view=%v err=%v", view, err)
	}
	down := NewEventHandler(platform, WithEngineViewFactory(func(context.Context, string, string, string) (*WebhookEngineView, error) {
		return nil, errors.New("tenant engine unavailable")
	}))
	if view, err := down.resolveEngineView(context.Background(), providerGitLab, "group/repo-a", ""); err == nil || view != nil {
		t.Fatalf("unavailable engine must surface an error without a view, got view=%v err=%v", view, err)
	}
	hit := &events.Processor{}
	targeted := NewEventHandler(platform, WithEngineViewFactory(func(context.Context, string, string, string) (*WebhookEngineView, error) {
		return &WebhookEngineView{Processor: hit}, nil
	}))
	if view, err := targeted.resolveEngineView(context.Background(), providerGitLab, "group/repo-a", ""); err != nil || view.Processor != hit {
		t.Fatalf("factory hit must win, got view=%v err=%v", view, err)
	}
}

// 已验签的 webhook 在租户引擎不可用时必须 503(代码托管会重试),即便
// 构造期存在平台处理器也不得回落写库。
func TestGiteaWebhookFailsClosedWhenTenantEngineUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewEventHandler(&events.Processor{},
		WithWebhookSecretLookup(&fakeSecretLookup{secrets: map[secretLookupKey]string{
			{provider: "gitea", path: "group/repo-a"}: "secret-gitea-a",
		}}),
		WithEngineViewFactory(func(context.Context, string, string, string) (*WebhookEngineView, error) {
			return nil, errors.New("tenant engine unavailable")
		}))
	router.POST("/api/events/gitea", handler.Gitea)

	body := `{"action":"opened","repository":{"full_name":"group/repo-a","html_url":"https://gitea.example.com/group/repo-a"}}`
	response := performEvent(t, router, http.MethodPost, "/api/events/gitea", body, map[string]string{
		"X-Gitea-Signature": hmacSignature("secret-gitea-a", []byte(body)),
	})
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("engine unavailable must fail closed with 503, got %d: %s", response.Code, response.Body.String())
	}
}

// TestDeriveInstanceBaseURL 锁定实例地址推导：完整链接必须是
// 实例地址 + "/" + 仓路径 的拼接形态，后缀不匹配或链接为空都推导失败
// （调用方维持歧义拒绝，不猜不兜底）。
func TestDeriveInstanceBaseURL(t *testing.T) {
	cases := []struct {
		name     string
		repoURL  string
		repoPath string
		want     string
		ok       bool
	}{
		{name: "plain suffix", repoURL: "https://gitlab.example.com/group/repo-a", repoPath: "group/repo-a", want: "https://gitlab.example.com", ok: true},
		{name: "nested group path", repoURL: "https://gitea.internal/team/sub/repo", repoPath: "team/sub/repo", want: "https://gitea.internal", ok: true},
		{name: "explicit default port", repoURL: "https://gitlab.example.com:443/group/repo-a", repoPath: "group/repo-a", want: "https://gitlab.example.com:443", ok: true},
		{name: "suffix mismatch", repoURL: "https://other.example.com/other/repo", repoPath: "group/repo-a", want: "", ok: false},
		{name: "empty url", repoURL: "", repoPath: "group/repo-a", want: "", ok: false},
		{name: "blank url", repoURL: "   ", repoPath: "group/repo-a", want: "", ok: false},
		{name: "url without path suffix", repoURL: "https://gitlab.example.com", repoPath: "group/repo-a", want: "", ok: false},
	}
	for _, tc := range cases {
		got, ok := DeriveInstanceBaseURL(tc.repoURL, tc.repoPath)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("%s: DeriveInstanceBaseURL(%q, %q)=(%q, %v), want (%q, %v)",
				tc.name, tc.repoURL, tc.repoPath, got, ok, tc.want, tc.ok)
		}
	}
}

// TestPayloadRepoURLParsing 锁定三个平台载荷的完整链接解析：真实载荷
// 必带该字段，缺失/载荷非法时返回空串（消歧侧据此维持 fail-closed）。
func TestPayloadRepoURLParsing(t *testing.T) {
	gitLab := gitLabRepoWebURL([]byte(`{"project":{"path_with_namespace":"group/repo-a","web_url":"https://gitlab.example.com/group/repo-a"}}`))
	if gitLab != "https://gitlab.example.com/group/repo-a" {
		t.Fatalf("gitlab web_url=%q", gitLab)
	}
	if got := gitLabRepoWebURL([]byte(`{"project":{"path_with_namespace":"group/repo-a"}}`)); got != "" {
		t.Fatalf("missing web_url must yield empty, got %q", got)
	}
	if got := gitLabRepoWebURL([]byte(`not json`)); got != "" {
		t.Fatalf("malformed payload must yield empty, got %q", got)
	}
	codeHost := codeHostRepoHTMLURL([]byte(`{"repository":{"full_name":"group/repo-a","html_url":"https://gitea.example.com/group/repo-a"}}`))
	if codeHost != "https://gitea.example.com/group/repo-a" {
		t.Fatalf("codehost html_url=%q", codeHost)
	}
	if got := codeHostRepoHTMLURL([]byte(`{"repository":{"full_name":"group/repo-a"}}`)); got != "" {
		t.Fatalf("missing html_url must yield empty, got %q", got)
	}
	if got := codeHostRepoHTMLURL([]byte(`not json`)); got != "" {
		t.Fatalf("malformed payload must yield empty, got %q", got)
	}
}

// TestWebhookForwardsRepoURLToFactory 锁定贯穿链：载荷里的仓库完整链接
// 必须原样到达处理器工厂（多行路由消歧的消费端），与 secret 查找端同参。
func TestWebhookForwardsRepoURLToFactory(t *testing.T) {
	var seen struct {
		provider string
		path     string
		repoURL  string
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewEventHandler(nil,
		WithWebhookSecretLookup(&fakeSecretLookup{secrets: map[secretLookupKey]string{
			{provider: "github", path: "group/repo-a"}: "secret-github-a",
		}}),
		WithEngineViewFactory(func(_ context.Context, provider, path, repoURL string) (*WebhookEngineView, error) {
			seen.provider, seen.path, seen.repoURL = provider, path, repoURL
			// 返回不可用错误：工厂已被调用即可断参，503 收尾避免依赖
			// 平台处理器的完整装配。
			return nil, errors.New("captured")
		}))
	router.POST("/api/events/github", handler.GitHub)

	body := `{"action":"opened","repository":{"full_name":"group/repo-a","html_url":"https://github.com/group/repo-a"}}`
	response := performEvent(t, router, http.MethodPost, "/api/events/github", body, map[string]string{
		"X-Hub-Signature-256": githubSignatureHeader("secret-github-a", []byte(body)),
	})
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("factory call must conclude with the unavailable status, got %d: %s", response.Code, response.Body.String())
	}
	if seen.provider != "github" || seen.path != "group/repo-a" || seen.repoURL != "https://github.com/group/repo-a" {
		t.Fatalf("factory must receive the payload repo url, got %+v", seen)
	}
}
