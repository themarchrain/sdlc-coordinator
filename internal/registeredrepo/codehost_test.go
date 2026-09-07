package registeredrepo

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/adapters/gitea"
	"sdlc-coordinator/internal/adapters/gitlab"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
)

func openResolverTestDB(t *testing.T) *Store {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	repoStore := NewStore(db)
	if err := repoStore.Migrate(); err != nil {
		t.Fatal(err)
	}
	return repoStore
}

func platformRepoFixture(id, provider, baseURL, projectID, path string) *RegisteredRepository {
	return &RegisteredRepository{
		RepositoryID:      id,
		DisplayName:       path,
		Provider:          provider,
		BaseURL:           baseURL,
		CodeHostProjectID: projectID,
		PathWithNamespace: path,
		Token:             "token-a",
		Status:            StatusEnabled,
	}
}

func TestCodeHostResolverRejectsForeignProject(t *testing.T) {
	repoStore := openResolverTestDB(t)
	ctx := context.Background()
	if err := repoStore.Create(ctx, platformRepoFixture("repo_a", "gitlab", "https://gitlab.example.com", "1", "group/a")); err != nil {
		t.Fatal(err)
	}
	resolver := NewCodeHostResolver(repoStore, nil)
	id := "repo_a"
	_, err := resolver.Resolve(context.Background(), &model.Task{TaskID: "t1", RepositoryID: &id}, &model.Writeback{ProjectID: "2"})
	if err == nil || err.Error() != "writeback project does not belong to repository" {
		t.Fatalf("got %v", err)
	}
	backend, err := resolver.Resolve(context.Background(), &model.Task{TaskID: "t1", RepositoryID: &id}, &model.Writeback{ProjectID: "1"})
	if err != nil || backend == nil {
		t.Fatalf("expected backend, got %v %v", backend, err)
	}
}

// TestResolveDispatchesPerRepositoryPlatform 锁定按仓分派:同 path/同 project id
// 的两平台仓,任务写回 client 各按仓行的 provider/base_url 构造。
func TestResolveDispatchesPerRepositoryPlatform(t *testing.T) {
	repoStore := openResolverTestDB(t)
	ctx := context.Background()
	if err := repoStore.Create(ctx, platformRepoFixture("repo_gl", "gitlab", "https://gitlab.example.com", "555", "group/project")); err != nil {
		t.Fatal(err)
	}
	if err := repoStore.Create(ctx, platformRepoFixture("repo_gt", "gitea", "https://gitea.example.com", "555", "group/project")); err != nil {
		t.Fatal(err)
	}
	resolver := NewCodeHostResolver(repoStore, nil)
	glID, gtID := "repo_gl", "repo_gt"

	gitlabBackend, err := resolver.Resolve(ctx, &model.Task{TaskID: "t1", RepositoryID: &glID}, &model.Writeback{ProjectID: "555"})
	if err != nil {
		t.Fatalf("resolve gitlab task: %v", err)
	}
	if _, ok := gitlabBackend.(*gitlab.Client); !ok {
		t.Fatalf("expected gitlab client for gitlab repo, got %T", gitlabBackend)
	}

	giteaBackend, err := resolver.Resolve(ctx, &model.Task{TaskID: "t2", RepositoryID: &gtID}, &model.Writeback{ProjectID: "group/project"})
	if err != nil {
		t.Fatalf("resolve gitea task: %v", err)
	}
	if _, ok := giteaBackend.(*gitea.Client); !ok {
		t.Fatalf("expected gitea client for gitea repo, got %T", giteaBackend)
	}
}

// TestResolveFailsClosedWhenPlatformIdentityMissing 锁定:仓行平台身份缺失
// (回填不可达的理论残余)时 fail-closed,不得用空 URL 构造 client。
func TestResolveFailsClosedWhenPlatformIdentityMissing(t *testing.T) {
	repoStore := openResolverTestDB(t)
	ctx := context.Background()
	repo := platformRepoFixture("repo_x", "gitlab", "", "1", "group/x")
	if err := repoStore.Create(ctx, repo); err != nil {
		t.Fatal(err)
	}
	resolver := NewCodeHostResolver(repoStore, nil)
	id := "repo_x"
	if _, err := resolver.Resolve(ctx, &model.Task{TaskID: "t", RepositoryID: &id}, &model.Writeback{ProjectID: "1"}); err == nil {
		t.Fatal("empty base_url must fail closed")
	}
}

// TestCodeHostClientRejectsUnknownProvider 锁定:未知 provider 在构造入口
// fail-closed,不得静默回退 GitLab(回退会让跨平台投递写错平台)。
func TestCodeHostClientRejectsUnknownProvider(t *testing.T) {
	if _, err := CodeHostClient("gitbucket", "https://gitbucket.example.com", "token-a", nil); err == nil {
		t.Fatal("unknown provider must fail closed")
	}
	if _, err := CodeHostClient("", "https://gitlab.example.com", "token-a", nil); err == nil {
		t.Fatal("empty provider must fail closed")
	}
	if _, err := CodeHostClient("gitlab", "", "token-a", nil); err == nil {
		t.Fatal("empty base_url must fail closed")
	}
}
