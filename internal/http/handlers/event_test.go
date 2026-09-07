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

	"sdlc-coordinator/internal/registeredrepo"

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
}

type fakeSecretLookup struct {
	secrets map[secretLookupKey]string
	lookups []secretLookupKey
}

func (f *fakeSecretLookup) WebhookSecretByPath(_ context.Context, provider, path string) (string, error) {
	key := secretLookupKey{provider: provider, path: path}
	f.lookups = append(f.lookups, key)
	secret, ok := f.secrets[key]
	if !ok {
		return "", registeredrepo.ErrNotFound
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
	payload := `{"project":{"path_with_namespace":"group/repo-a"},"object_kind":"merge_request"}`

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

func TestGitLabWebhookRejectsUnregisteredRepo(t *testing.T) {
	router, lookup := newEventRouter(nil)
	payload := `{"project":{"path_with_namespace":"group/unknown"},"object_kind":"push"}`
	resp := performEvent(t, router, http.MethodPost, "/api/events/gitlab", payload, map[string]string{"X-Gitlab-Token": "anything"})
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("unregistered repo must be 401, got %d: %s", resp.Code, resp.Body.String())
	}
	if lookup.lookups[len(lookup.lookups)-1] != (secretLookupKey{provider: "gitlab", path: "group/unknown"}) {
		t.Fatalf("lookup=%+v", lookup.lookups[len(lookup.lookups)-1])
	}
}

// TestWebhookSecretLookupIsProviderScoped 锁定 provider 作用域:跨平台同 path
// 仓各持各的 secret,A 平台 endpoint 用 B 平台同 path 仓的 secret 必须拒绝。
func TestWebhookSecretLookupIsProviderScoped(t *testing.T) {
	router, _ := newEventRouter(nil)
	gitlabPayload := `{"project":{"path_with_namespace":"group/repo-a"},"object_kind":"merge_request"}`
	codeHostPayload := `{"action":"opened","repository":{"full_name":"group/repo-a"}}`

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
	payload := `{"project":{"path_with_namespace":"group/empty"},"object_kind":"push"}`
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
	payload := `{"action":"opened","repository":{"full_name":"group/repo-a"}}`

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
	unregisteredBody := `{"repository":{"full_name":"group/unknown"}}`
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
	payload := `{"action":"opened","repository":{"full_name":"group/repo-a"}}`

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
	emptyBody := `{"repository":{"full_name":"group/empty"}}`
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
// 与未注册、空 secret 一样一律拒绝,不得放行。
func TestWebhookSecretLookupFailureIsFailClosed(t *testing.T) {
	router, _ := newEventRouter(&failingSecretLookup{})
	resp := performEvent(t, router, http.MethodPost, "/api/events/gitlab", `{"project":{"path_with_namespace":"group/repo-a"}}`, map[string]string{"X-Gitlab-Token": "secret-a"})
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("lookup failure must be 401, got %d", resp.Code)
	}
}

type failingSecretLookup struct{}

func (failingSecretLookup) WebhookSecretByPath(context.Context, string, string) (string, error) {
	return "", errors.New("db down")
}
