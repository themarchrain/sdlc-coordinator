package app

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
)

// 派发路由第一跳真实现：任务 → 注册仓行 → 控制面路由
// 索引 → 租户。任务未绑定仓、仓行缺失或索引 miss 一律 fail-closed 拒派发。
func TestIndexTaskTenantResolver(t *testing.T) {
	cpRepo := openControlPlaneRepository(t, testControlPlaneSettings(t))
	ctx := context.Background()
	platformStoreDB := openPlatformStoreDB(t)
	repoStore := registeredrepo.NewStore(platformStoreDB)
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate repo store: %v", err)
	}
	if err := repoStore.Create(ctx, &registeredrepo.RegisteredRepository{
		RepositoryID: "repo-x", DisplayName: "X", Provider: "gitlab",
		BaseURL: "https://gitlab.example.com", PathWithNamespace: "acme/x",
	}); err != nil {
		t.Fatalf("seed repo: %v", err)
	}
	if err := cpRepo.UpsertRepoRouteIndex(ctx, controlplane.RepoRouteIndex{
		Provider: "gitlab", BaseURL: "https://gitlab.example.com", PathWithNamespace: "acme/x",
		TenantID: 7, RepoFingerprint: "repo-x",
	}); err != nil {
		t.Fatalf("seed index: %v", err)
	}

	resolver := &indexTaskTenantResolver{index: cpRepo, repos: repoStore}
	repoID := "repo-x"
	tenant, err := resolver.ResolveTenant(ctx, &model.Task{TaskID: "task-1", RepositoryID: &repoID})
	if err != nil || tenant != 7 {
		t.Fatalf("expected tenant 7, got tenant=%d err=%v", tenant, err)
	}

	// 仓行存在但索引 miss = fail-closed 拒派发。
	if err := repoStore.Create(ctx, &registeredrepo.RegisteredRepository{
		RepositoryID: "repo-unrouted", DisplayName: "U", Provider: "gitlab",
		BaseURL: "https://gitlab.example.com", PathWithNamespace: "acme/unrouted",
	}); err != nil {
		t.Fatalf("seed unrouted repo: %v", err)
	}
	unroutedID := "repo-unrouted"
	if _, err := resolver.ResolveTenant(ctx, &model.Task{TaskID: "task-2", RepositoryID: &unroutedID}); err == nil {
		t.Fatalf("unrouted repo must fail closed")
	}
	// 任务未绑定仓 = fail-closed。
	unbound := &model.Task{TaskID: "task-3"}
	if _, err := resolver.ResolveTenant(ctx, unbound); err == nil {
		t.Fatalf("unbound task must fail closed")
	}
}
