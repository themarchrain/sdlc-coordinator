package registeredrepo

import (
	"context"
	"slices"
	"sort"
	"testing"
	"time"

	"sdlc-coordinator/internal/store"
)

// legacyRegisteredRepository 复刻改名前的注册仓模型(旧列名 gitlab_*),用它让
// gorm 建出同形态的表。手写 DDL 的类型/约束会与 gorm 产物有差异,不能当迁移基线。
// 本结构是旧模型快照,禁止随现行模型演进。
type legacyRegisteredRepository struct {
	ID                     uint             `gorm:"primaryKey"`
	RepositoryID           string           `gorm:"column:repository_id;uniqueIndex;not null"`
	DisplayName            string           `gorm:"column:display_name;not null"`
	Provider               string           `gorm:"column:provider;index;not null;default:''"`
	BaseURL                string           `gorm:"column:base_url;type:text"`
	GitLabProjectID        string           `gorm:"column:gitlab_project_id"`
	PathWithNamespace      string           `gorm:"column:path_with_namespace;not null"`
	Token                  string           `gorm:"column:token;type:text"`
	WebhookSecret          string           `gorm:"column:webhook_secret;type:text"`
	TokenScopeAttested     bool             `gorm:"column:token_scope_attested;not null;default:false"`
	DefaultBranch          string           `gorm:"column:default_branch;type:text"`
	GitLabIssueLabels      string           `gorm:"column:gitlab_issue_labels;type:text"`
	ReviewerMentions       string           `gorm:"column:reviewer_mentions;type:text"`
	Status                 RepositoryStatus `gorm:"column:status;index;not null"`
	LastConfigurationError string           `gorm:"column:last_configuration_error;type:text"`
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func (legacyRegisteredRepository) TableName() string { return "registered_repositories" }

// openLegacySchemaTestDB 用旧列名建表,再建 provider 作用域唯一索引,并预置示例行。
func openLegacySchemaTestDB(t *testing.T) *Store {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&legacyRegisteredRepository{}); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	for _, ddl := range []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS uidx_registered_repos_path ON registered_repositories(provider, path_with_namespace) WHERE path_with_namespace <> ''`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uidx_registered_repos_provider_project ON registered_repositories(provider, gitlab_project_id) WHERE gitlab_project_id <> ''`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("create legacy identity indexes: %v", err)
		}
	}
	if err := db.Exec(`INSERT INTO registered_repositories
		(repository_id, display_name, provider, base_url, gitlab_project_id, path_with_namespace, token, webhook_secret, token_scope_attested, default_branch, gitlab_issue_labels, reviewer_mentions, status)
		VALUES
		('repo_legacy_a', 'Legacy A', 'gitlab', 'https://gitlab.example.com', '24207', 'group/a', 'tok-a', 'whsec-a', 1, 'main', '["developer-agent","hotfix"]', '', 'enabled'),
		('repo_legacy_b', 'Legacy B', 'gitea', 'https://gitea.example.com', '9', 'group/b', 'tok-b', 'whsec-b', 1, 'master', '', '', 'disabled')`).Error; err != nil {
		t.Fatalf("seed legacy rows: %v", err)
	}
	return NewStore(db)
}

func tableColumns(t *testing.T, repoStore *Store) []string {
	t.Helper()
	// 复用 Store.tableColumns,避免 pragma 扫描形态在测试与实现两处漂移。
	columns, err := repoStore.tableColumns()
	if err != nil {
		t.Fatalf("table columns: %v", err)
	}
	names := make([]string, 0, len(columns))
	for name := range columns {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func hasColumn(t *testing.T, repoStore *Store, name string) bool {
	t.Helper()
	for _, column := range tableColumns(t, repoStore) {
		if column == name {
			return true
		}
	}
	return false
}

// TestMigrateRenamesLegacyIdentityColumns 锁定列改名迁移:存量值逐行保持、
// 旧列消失、provider 作用域唯一索引随改名继续生效、查找按新列命中、二次迁移幂等。
func TestMigrateRenamesLegacyIdentityColumns(t *testing.T) {
	repoStore := openLegacySchemaTestDB(t)
	ctx := context.Background()
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}
	if hasColumn(t, repoStore, "gitlab_project_id") || hasColumn(t, repoStore, "gitlab_issue_labels") {
		t.Fatal("legacy columns must be renamed away")
	}
	if !hasColumn(t, repoStore, "codehost_project_id") || !hasColumn(t, repoStore, "codehost_issue_labels") {
		t.Fatal("renamed columns missing")
	}
	first, err := repoStore.Get(ctx, "repo_legacy_a")
	if err != nil {
		t.Fatalf("reload legacy row: %v", err)
	}
	if first.CodeHostProjectID != "24207" || first.DefaultBranch != "main" || first.Status != StatusEnabled {
		t.Fatalf("row values not preserved: %+v", first)
	}
	if labels := decodeLabels(first.CodeHostIssueLabels); len(labels) != 2 || labels[0] != "developer-agent" || labels[1] != "hotfix" {
		t.Fatalf("labels not preserved: %#v", labels)
	}
	// 旧形态预置的 provider 作用域索引随 RENAME 自动指向新列:同平台撞 project id 拒绝。
	dup := &RegisteredRepository{RepositoryID: "repo_dup", DisplayName: "dup", Provider: "gitlab",
		BaseURL: "https://gitlab.example.com", CodeHostProjectID: "24207", PathWithNamespace: "other/a",
		Token: "t", WebhookSecret: "w", TokenScopeAttested: true, Status: StatusConfiguring}
	if err := repoStore.Create(ctx, dup); err == nil {
		t.Fatal("duplicate project id within provider must be rejected after rename")
	}
	// 跨平台同 project id 合法共存(索引语义不变)。
	other := *dup
	other.RepositoryID, other.Provider, other.BaseURL = "repo_other", "gitea", "https://gitea.example.com"
	other.PathWithNamespace = "other/b"
	if err := repoStore.Create(ctx, &other); err != nil {
		t.Fatalf("cross-provider same project id must coexist: %v", err)
	}
	// 查找按新列名命中(数据驱动证明改名,而非空列假绿)。
	byID, err := repoStore.GetByProjectID(ctx, "gitlab", "24207")
	if err != nil || byID.RepositoryID != "repo_legacy_a" {
		t.Fatalf("GetByProjectID on renamed column: %v %+v", err, byID)
	}
	// 二次迁移幂等:零错误、列集合不变、行值不变。
	before := tableColumns(t, repoStore)
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("second migrate must be idempotent: %v", err)
	}
	after := tableColumns(t, repoStore)
	if !slices.Equal(before, after) {
		t.Fatalf("column set drifted: %v vs %v", before, after)
	}
	again, err := repoStore.Get(ctx, "repo_legacy_a")
	if err != nil || again.CodeHostProjectID != "24207" {
		t.Fatalf("row values drifted on second migrate: %v %+v", err, again)
	}
}

// preProviderRegisteredRepository 复刻更老的 vintage:仓平台身份分支之前建的表,
// 连 provider/base_url 列都没有,用于验证「改名 + AutoMigrate 补列」的组合路径。
type preProviderRegisteredRepository struct {
	ID                     uint             `gorm:"primaryKey"`
	RepositoryID           string           `gorm:"column:repository_id;uniqueIndex;not null"`
	DisplayName            string           `gorm:"column:display_name;not null"`
	GitLabProjectID        string           `gorm:"column:gitlab_project_id"`
	PathWithNamespace      string           `gorm:"column:path_with_namespace;not null"`
	Token                  string           `gorm:"column:token;type:text"`
	WebhookSecret          string           `gorm:"column:webhook_secret;type:text"`
	TokenScopeAttested     bool             `gorm:"column:token_scope_attested;not null;default:false"`
	DefaultBranch          string           `gorm:"column:default_branch;type:text"`
	GitLabIssueLabels      string           `gorm:"column:gitlab_issue_labels;type:text"`
	ReviewerMentions       string           `gorm:"column:reviewer_mentions;type:text"`
	Status                 RepositoryStatus `gorm:"column:status;index;not null"`
	LastConfigurationError string           `gorm:"column:last_configuration_error;type:text"`
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func (preProviderRegisteredRepository) TableName() string { return "registered_repositories" }

// TestMigrateRenamesOnPreProviderSchema 锁定最老 vintage:改名照常执行,缺失的
// provider/base_url 列由 AutoMigrate 补齐(空值;平台身份回填是装配方职责)。
func TestMigrateRenamesOnPreProviderSchema(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&preProviderRegisteredRepository{}); err != nil {
		t.Fatalf("create pre-provider schema: %v", err)
	}
	if err := db.Exec(`INSERT INTO registered_repositories
		(repository_id, display_name, gitlab_project_id, path_with_namespace, token, webhook_secret, token_scope_attested, status)
		VALUES ('repo_old', 'Old', '24207', 'group/old', 'tok', 'whsec', 1, 'enabled')`).Error; err != nil {
		t.Fatalf("seed row: %v", err)
	}
	repoStore := NewStore(db)
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate pre-provider schema: %v", err)
	}
	if hasColumn(t, repoStore, "gitlab_project_id") || !hasColumn(t, repoStore, "codehost_project_id") {
		t.Fatal("identity columns must be renamed on the pre-provider vintage")
	}
	if !hasColumn(t, repoStore, "provider") || !hasColumn(t, repoStore, "base_url") {
		t.Fatal("platform identity columns must be added by AutoMigrate")
	}
	loaded, err := repoStore.Get(context.Background(), "repo_old")
	if err != nil || loaded.CodeHostProjectID != "24207" {
		t.Fatalf("row values not preserved across vintage migration: %v %+v", err, loaded)
	}
	if loaded.Provider != "" || loaded.BaseURL != "" {
		t.Fatalf("platform identity must stay empty until backfill: %+v", loaded)
	}
}

// TestMigrateHealsPartiallyRenamedVintage 锁定中间态愈合:改名已做的库,迁移必须
// 安全跳过改名且保留行值。
func TestMigrateHealsPartiallyRenamedVintage(t *testing.T) {
	repoStore := openLegacySchemaTestDB(t)
	ctx := context.Background()
	for _, ddl := range []string{
		`ALTER TABLE registered_repositories RENAME COLUMN gitlab_project_id TO codehost_project_id`,
		`ALTER TABLE registered_repositories RENAME COLUMN gitlab_issue_labels TO codehost_issue_labels`,
	} {
		if err := repoStore.db.Exec(ddl).Error; err != nil {
			t.Fatalf("manual rename: %v", err)
		}
	}
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate partial vintage: %v", err)
	}
	kept, err := repoStore.Get(ctx, "repo_legacy_a")
	if err != nil || kept.CodeHostProjectID != "24207" {
		t.Fatalf("row must survive the partial vintage: %v %+v", err, kept)
	}
}

// TestMigrateFailsFastOnConflictingVintage 锁定双列并存拒绝:迁移序列之外的
// 干扰(新旧列同存)必须显式报错,而不是静默跳过把存量值滞留旧列。
func TestMigrateFailsFastOnConflictingVintage(t *testing.T) {
	repoStore := openLegacySchemaTestDB(t)
	if err := repoStore.db.Exec(`ALTER TABLE registered_repositories ADD COLUMN codehost_project_id text`).Error; err != nil {
		t.Fatalf("inject conflicting column: %v", err)
	}
	if err := repoStore.Migrate(); err == nil {
		t.Fatal("conflicting schema vintage must fail fast")
	}
}
