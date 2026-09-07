package registeredrepo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/adapters/gitea"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
)

func TestGiteaStatusCodeExtractsHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := gitea.NewClient(server.URL, "bad-token")
	_, err := client.CreateIssue(context.Background(), "group/repo", codehost.CreateIssueRequest{Title: "x"})
	status := gitea.StatusCode(err)
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401 from gitea StatusCode, got %d (%v)", status, err)
	}
}

func TestWritebackAuthHandlerDegradesOnGiteaCredentialError(t *testing.T) {
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
		RepositoryID: "repo-a", DisplayName: "A", PathWithNamespace: "group/a",
		Provider: "gitea", BaseURL: "https://gitea.example.com",
		Token: "token-a", TokenScopeAttested: true,
		Status: StatusEnabled, CodeHostProjectID: "42",
	}
	if err := repoStore.Create(context.Background(), repo); err != nil {
		t.Fatalf("seed repo: %v", err)
	}

	handler := NewWritebackAuthHandler(repoStore, nil)
	err = &gitea.StatusError{StatusCode: 401, Message: "unauthorized"}
	if !handler.HandleWritebackFailure(context.Background(), &model.Task{RepositoryID: &repo.RepositoryID}, err) {
		t.Fatal("expected gitea 401 to trigger degradation")
	}
	updated, _ := repoStore.Get(context.Background(), "repo-a")
	if updated.Status != StatusConfiguring {
		t.Fatalf("expected configuring status, got %s", updated.Status)
	}
}
