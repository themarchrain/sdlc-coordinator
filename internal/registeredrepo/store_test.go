package registeredrepo

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"sdlc-coordinator/internal/store"
)

func openRegisteredRepoTestDB(t *testing.T) *Store {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repoStore := NewStore(db)
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := repoStore.EnsureIdentityIndexes(); err != nil {
		t.Fatalf("ensure identity indexes: %v", err)
	}
	return repoStore
}

func enabledFixture(repositoryID, projectID, path string) *RegisteredRepository {
	return &RegisteredRepository{
		RepositoryID:      repositoryID,
		DisplayName:       path,
		Provider:          "gitlab",
		BaseURL:           "https://gitlab.example.com",
		CodeHostProjectID: projectID,
		PathWithNamespace: path,
		Token:             "token-a",
		Status:            StatusEnabled,
	}
}

func TestStoreRejectsDuplicateProjectIDAndPath(t *testing.T) {
	repoStore := openRegisteredRepoTestDB(t)
	ctx := context.Background()
	first := enabledFixture("repo_a", "24207", "group/repo-a")
	if err := repoStore.Create(ctx, first); err != nil {
		t.Fatalf("create first: %v", err)
	}
	dupProject := enabledFixture("repo_b", "24207", "group/repo-b")
	if err := repoStore.Create(ctx, dupProject); err == nil {
		t.Fatal("expected duplicate codehost_project_id to fail")
	}
	if err := repoStore.Create(ctx, dupProject); err != nil && strings.Contains(fmt.Sprintf("%v", err), "token-a") {
		t.Fatal("token leaked in duplicate project error")
	}
	dupPath := enabledFixture("repo_c", "24208", "group/repo-a")
	if err := repoStore.Create(ctx, dupPath); err == nil {
		t.Fatal("expected duplicate path_with_namespace to fail")
	}
}

func TestStoreCreateGeneratesRepositoryID(t *testing.T) {
	repoStore := openRegisteredRepoTestDB(t)
	repo := &RegisteredRepository{
		DisplayName:       "Coordinator",
		Provider:          "gitlab",
		CodeHostProjectID: "24207",
		PathWithNamespace: "group/coordinator",
		Token:             "token-a",
		Status:            StatusConfiguring,
	}
	if err := repoStore.Create(context.Background(), repo); err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.HasPrefix(repo.RepositoryID, "repo_") {
		t.Fatalf("repository_id=%q", repo.RepositoryID)
	}
	loaded, err := repoStore.GetByProjectID(context.Background(), "gitlab", "24207")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if loaded.PathWithNamespace != "group/coordinator" {
		t.Fatalf("path=%q", loaded.PathWithNamespace)
	}
}

func TestStoreRejectsInvalidStatus(t *testing.T) {
	repoStore := openRegisteredRepoTestDB(t)
	repo := enabledFixture("repo_a", "1", "group/a")
	repo.Status = "archived"
	err := repoStore.Create(context.Background(), repo)
	if err == nil {
		t.Fatal("expected invalid status to fail")
	}
	if strings.Contains(err.Error(), "token-a") {
		t.Fatal("token leaked in status error")
	}
}

func TestWebhookSecretByPathReturnsTrimmedSecret(t *testing.T) {
	repoStore := openRegisteredRepoTestDB(t)
	ctx := context.Background()
	repo := enabledFixture("repo_a", "24207", "group/repo-a")
	repo.WebhookSecret = "  hook-secret-a  "
	if err := repoStore.Create(ctx, repo); err != nil {
		t.Fatalf("create: %v", err)
	}
	secret, err := repoStore.WebhookSecretByPath(ctx, "gitlab", "group/repo-a", "")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if secret != "hook-secret-a" {
		t.Fatalf("secret=%q", secret)
	}
	if secret, err := repoStore.WebhookSecretByPath(ctx, "gitlab", "group/missing", ""); err != nil || secret != "" {
		t.Fatalf("unregistered path must be empty-without-error, got secret=%q err=%v", secret, err)
	}
	if secret, err := repoStore.WebhookSecretByPath(ctx, "gitea", "group/repo-a", ""); err != nil || secret != "" {
		t.Fatalf("provider scope must isolate path lookup, got secret=%q err=%v", secret, err)
	}
}

// TestNormalizeBaseURL 锁定平台地址归一化形态：注册摄入与 webhook 撞名
// 消歧共用此函数，同一输入两侧必须得到同一输出（含默认端口剥除——
// https 显式 :443 与 http 显式 :80 是同一实例的两种写法，不剥会导致
// 消歧「差一点=不中」）。
func TestNormalizeBaseURL(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "already normalized", raw: "https://gitlab.example.com", want: "https://gitlab.example.com"},
		{name: "upper scheme and host", raw: "HTTPS://GitLab.Example.COM", want: "https://gitlab.example.com"},
		{name: "trailing slash", raw: "https://gitlab.example.com/", want: "https://gitlab.example.com"},
		{name: "multiple trailing slashes", raw: "https://gitlab.example.com//", want: "https://gitlab.example.com"},
		{name: "https default port stripped", raw: "https://gitlab.example.com:443", want: "https://gitlab.example.com"},
		{name: "http default port stripped", raw: "http://gitlab.internal:80", want: "http://gitlab.internal"},
		{name: "leading and trailing spaces", raw: "  https://gitlab.example.com  ", want: "https://gitlab.example.com"},
		{name: "non-default port kept", raw: "https://gitlab.internal:8443", want: "https://gitlab.internal:8443"},
		{name: "default port with path", raw: "https://gitlab.example.com:443/sub", want: "https://gitlab.example.com/sub"},
		{name: "http scheme kept", raw: "http://gitlab.internal", want: "http://gitlab.internal"},
	}
	for _, tc := range cases {
		got, err := NormalizeBaseURL(tc.raw)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: NormalizeBaseURL(%q)=%q, want %q", tc.name, tc.raw, got, tc.want)
		}
	}
	for _, raw := range []string{
		"",
		"   ",
		"gitlab.example.com",
		"ftp://gitlab.example.com",
		"https://user@gitlab.example.com",
		"https://gitlab.example.com?x=1",
		"https://gitlab.example.com#frag",
		"https://:443",
	} {
		if got, err := NormalizeBaseURL(raw); err == nil {
			t.Fatalf("NormalizeBaseURL(%q) must be rejected, got %q", raw, got)
		}
	}
}
