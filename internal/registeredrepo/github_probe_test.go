package registeredrepo

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sdlc-coordinator/internal/adapters/github"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
)

// 仓库注册链路的身份探针必须按 provider 分流(GitHub 与 GitLab/Gitea 各自的
// API 形状不同)。
func TestNewProviderProbeSelectsGitHubProbe(t *testing.T) {
	if _, ok := NewProviderProbe("github", nil).(GitHubHTTPProbe); !ok {
		t.Fatal("expected github provider to select GitHubHTTPProbe")
	}
}

func TestGitHubHTTPProbeReturnsProjectIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/org/repo" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer repo-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"id":72573381,"full_name":"org/repo","default_branch":"main","clone_url":"https://github.com/org/repo.git"}`))
	}))
	defer server.Close()

	probe := GitHubHTTPProbe{HTTP: server.Client()}
	ident, err := probe.GetProject(context.Background(), server.URL, "repo-token", "org/repo")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if ident.ID != "72573381" || ident.PathWithNamespace != "org/repo" || ident.DefaultBranch != "main" {
		t.Fatalf("unexpected identity: %#v", ident)
	}
}

func TestWritebackAuthHandlerDegradesOnGitHubCredentialError(t *testing.T) {
	dsn := strings.NewReplacer("/", "_", "\\", "_", " ", "_").Replace(t.Name())
	db, err := store.Open("file:" + dsn + "_" + model.NewID("db") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repoStore := NewStore(db)
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := &RegisteredRepository{
		RepositoryID: "repo-gh", DisplayName: "GH", PathWithNamespace: "org/repo",
		Provider: "github", BaseURL: "https://github.example.com",
		Token: "token-gh", TokenScopeAttested: true,
		Status: StatusEnabled, CodeHostProjectID: "72573381",
	}
	if err := repoStore.Create(context.Background(), repo); err != nil {
		t.Fatalf("seed repo: %v", err)
	}

	handler := NewWritebackAuthHandler(repoStore, nil)
	err = &github.StatusError{StatusCode: http.StatusUnauthorized, Message: "unauthorized"}
	if !handler.HandleWritebackFailure(context.Background(), &model.Task{RepositoryID: &repo.RepositoryID}, err) {
		t.Fatal("expected github 401 to trigger degradation")
	}
	updated, _ := repoStore.Get(context.Background(), "repo-gh")
	if updated.Status != StatusConfiguring {
		t.Fatalf("expected configuring status, got %s", updated.Status)
	}
}

// 注册行配 github.com 网页域时,探活请求自动派生到 REST API 域。
func TestGitHubHTTPProbeDerivesAPIHost(t *testing.T) {
	var seenHost string
	var seenPath string
	transport := roundTripStub(func(r *http.Request) (*http.Response, error) {
		seenHost = r.URL.Host
		seenPath = r.URL.Path
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":1,"full_name":"org/repo","default_branch":"main"}`)), Header: make(http.Header)}, nil
	})
	probe := GitHubHTTPProbe{HTTP: &http.Client{Transport: transport}}
	ident, err := probe.GetProject(context.Background(), "https://github.com", "repo-token", "org/repo")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if seenHost != "api.github.com" || seenPath != "/repos/org/repo" {
		t.Fatalf("unexpected target %s%s", seenHost, seenPath)
	}
	if ident.ID != "1" {
		t.Fatalf("unexpected identity: %#v", ident)
	}
}

// 非 github.com 的地址原样透传(GHE 与测试 server 不受派生影响)。
func TestGitHubHTTPProbePassesOtherHostsThrough(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/org/repo" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"id":2,"full_name":"org/repo","default_branch":"main"}`))
	}))
	defer server.Close()

	probe := GitHubHTTPProbe{HTTP: server.Client()}
	if _, err := probe.GetProject(context.Background(), server.URL, "t", "org/repo"); err != nil {
		t.Fatalf("probe: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func roundTripStub(fn roundTripFunc) http.RoundTripper { return fn }
