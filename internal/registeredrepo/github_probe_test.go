package registeredrepo

import (
	"context"
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
