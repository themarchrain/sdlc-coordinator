package app

import (
	"context"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store"
)

// 生产装配的 resolver 按任务绑定(RepositoryID)解析注册仓整行,
// 未绑定任务返回 repository_not_bound;URL 与凭据由同一行派生。
func TestRunnerRepositoryResolverResolvesBoundRow(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repoStore := registeredrepo.NewStore(db)
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := &registeredrepo.RegisteredRepository{
		RepositoryID: "repo-resolver", Provider: "gitea",
		BaseURL: "https://gitea.example.com:3000", PathWithNamespace: "group/project", Token: "row-token",
	}
	if err := repoStore.Create(ctx, repo); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	boundID := repo.RepositoryID
	resolver := runnerRepositoryResolver(repoStore)

	row, err := resolver(ctx, &model.Task{TaskID: "task-1", RepositoryID: &boundID})
	if err != nil {
		t.Fatalf("resolve bound task: %v", err)
	}
	if row.CloneURL() != "https://gitea.example.com:3000/group/project.git" {
		t.Fatalf("resolved row derived unexpected clone url: %q", row.CloneURL())
	}
	if row.Token != "row-token" {
		t.Fatalf("resolved row lost per-repository token: %q", row.Token)
	}
	if _, err := resolver(ctx, &model.Task{TaskID: "task-2"}); err == nil || !strings.Contains(err.Error(), orchestrator.ErrRepositoryNotBound.Error()) {
		t.Fatalf("unbound task should fail with repository_not_bound, got %v", err)
	}
}
