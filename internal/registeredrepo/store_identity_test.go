package registeredrepo

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/store"
)

// openRawTestDB 打开只做 AutoMigrate 的裸库:回填迁移测试需要控制
// EnsureIdentityIndexes 的时机(生产顺序为 Migrate → BackfillIdentity → Ensure)。
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

// TestBackfillIdentityFillsEmptyColumnsOnlyAndIsIdempotent 用全局配置填补空列,
// 规范化大小写与尾斜杠,不覆盖已有值,重入不再改行。
func TestBackfillIdentityFillsEmptyColumnsOnlyAndIsIdempotent(t *testing.T) {
	repoStore := openRawTestDB(t)
	ctx := context.Background()
	legacy := enabledFixture("repo_old", "1", "group/old")
	legacy.Provider, legacy.BaseURL = "", "" // 空列:尚无平台身份
	if err := repoStore.Create(ctx, legacy); err != nil {
		t.Fatalf("create legacy row: %v", err)
	}
	custom := enabledFixture("repo_new", "2", "group/new")
	custom.Provider, custom.BaseURL = "gitea", "https://gitea.example.com"
	if err := repoStore.Create(ctx, custom); err != nil {
		t.Fatalf("create custom row: %v", err)
	}
	backfilled, err := repoStore.BackfillIdentity(ctx, "GitLab", "https://GitLab.example.com/")
	if err != nil || backfilled != 2 {
		t.Fatalf("backfill: %v rows=%d", err, backfilled)
	}
	loaded, err := repoStore.Get(ctx, "repo_old")
	if err != nil {
		t.Fatalf("reload legacy row: %v", err)
	}
	if loaded.Provider != "gitlab" || loaded.BaseURL != "https://gitlab.example.com" {
		t.Fatalf("legacy row not normalized: %+v", loaded)
	}
	kept, err := repoStore.Get(ctx, "repo_new")
	if err != nil {
		t.Fatalf("reload custom row: %v", err)
	}
	if kept.Provider != "gitea" || kept.BaseURL != "https://gitea.example.com" {
		t.Fatalf("backfill must not overwrite configured values: %+v", kept)
	}
	again, err := repoStore.BackfillIdentity(ctx, "gitlab", "https://gitlab.example.com")
	if err != nil || again != 0 {
		t.Fatalf("backfill must be idempotent: %v rows=%d", err, again)
	}
}

// TestBackfillIdentitySkipsMissingDefaults 锁定:全局配置缺失时不报错、不填充
// (告警由装配方负责),启动不因缺省阻断。
func TestBackfillIdentitySkipsMissingDefaults(t *testing.T) {
	repoStore := openRawTestDB(t)
	ctx := context.Background()
	repo := enabledFixture("repo_old", "1", "group/old")
	repo.Provider, repo.BaseURL = "", ""
	if err := repoStore.Create(ctx, repo); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := repoStore.BackfillIdentity(ctx, "", ""); err != nil {
		t.Fatalf("missing defaults must not error: %v", err)
	}
	loaded, err := repoStore.Get(ctx, "repo_old")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if loaded.Provider != "" || loaded.BaseURL != "" {
		t.Fatalf("empty defaults must fill nothing: %+v", loaded)
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
