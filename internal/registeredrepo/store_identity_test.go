package registeredrepo

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/store"
)

// openRawTestDB 打开只做 AutoMigrate 的裸库:回填迁移测试需要控制
// EnsureIdentityIndexes 的时机(生产顺序为 Migrate → Ensure)。
func openRawTestDB(t *testing.T) *Store {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repoStore := NewStore(db)
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return repoStore
}

// TestStoreScopesIdentityByProvider 锁定身份作用域:跨平台同 path、同数字
// project id 是合法共存,查找必须按 provider 各归各的(防串仓)。
func TestStoreScopesIdentityByProvider(t *testing.T) {
	repoStore := openRegisteredRepoTestDB(t)
	ctx := context.Background()
	gitlab := enabledFixture("repo_gl", "555", "group/project")
	if err := repoStore.Create(ctx, gitlab); err != nil {
		t.Fatalf("create gitlab repo: %v", err)
	}
	gitea := enabledFixture("repo_gt", "555", "group/project")
	gitea.Provider, gitea.BaseURL = "gitea", "https://gitea.example.com"
	if err := repoStore.Create(ctx, gitea); err != nil {
		t.Fatalf("same path/project on another provider must coexist: %v", err)
	}
	byPath, err := repoStore.GetByPath(ctx, "gitea", "group/project")
	if err != nil || byPath.RepositoryID != "repo_gt" {
		t.Fatalf("gitea path lookup: %v %+v", err, byPath)
	}
	byPath, err = repoStore.GetByPath(ctx, "gitlab", "group/project")
	if err != nil || byPath.RepositoryID != "repo_gl" {
		t.Fatalf("gitlab path lookup: %v %+v", err, byPath)
	}
	byID, err := repoStore.GetByProjectID(ctx, "gitea", "555")
	if err != nil || byID.RepositoryID != "repo_gt" {
		t.Fatalf("gitea project lookup: %v %+v", err, byID)
	}
	byID, err = repoStore.GetByProjectID(ctx, "gitlab", "555")
	if err != nil || byID.RepositoryID != "repo_gl" {
		t.Fatalf("gitlab project lookup: %v %+v", err, byID)
	}
	if _, err := repoStore.GetByProjectID(ctx, "github", "555"); err == nil {
		t.Fatal("unrelated provider must not match")
	}
}

// TestGetByPathFallsBackToCaseInsensitive 锁定 R-P3-4 兜底:平台规范路径回写前
// 的窗口内,大小写不一致的投递按不区分大小写兜底命中,且兜底保持 provider 作用域。
func TestGetByPathFallsBackToCaseInsensitive(t *testing.T) {
	repoStore := openRegisteredRepoTestDB(t)
	ctx := context.Background()
	repo := enabledFixture("repo_a", "1", "Group/Project")
	if err := repoStore.Create(ctx, repo); err != nil {
		t.Fatalf("create: %v", err)
	}
	loaded, err := repoStore.GetByPath(ctx, "gitlab", "group/project")
	if err != nil || loaded.RepositoryID != "repo_a" {
		t.Fatalf("case-insensitive fallback: %v %+v", err, loaded)
	}
	if _, err := repoStore.GetByPath(ctx, "gitea", "group/project"); err == nil {
		t.Fatal("fallback must stay provider-scoped")
	}
}
