package registeredrepo

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"sdlc-coordinator/internal/store"

	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Migrate() error {
	// 列改名必须先于 AutoMigrate:AutoMigrate 只加列不改名,先跑会按新列名
	// 建出空列,与旧列数据分家。
	if err := s.renameLegacyIdentityColumns(); err != nil {
		return err
	}
	if err := s.db.AutoMigrate(&RegisteredRepository{}); err != nil {
		return fmt.Errorf("migrate registered repositories: %w", err)
	}
	// 唯一性由 provider 作用域索引承担(EnsureIdentityIndexes)。
	for _, name := range []string{
		"idx_registered_repositories_path_with_namespace",
		"idx_registered_repositories_gitlab_project_id",
		"uidx_registered_repos_project",
	} {
		if err := s.dropIndexIfExists(name); err != nil {
			return fmt.Errorf("drop legacy index %s: %w", name, err)
		}
	}
	return nil
}

// dropIndexIfExists 按方言拆除遗留索引名（SQLite 直落 DDL;MySQL 先探测）。
func (s *Store) dropIndexIfExists(name string) error {
	if s.db.Dialector.Name() == "mysql" {
		if !s.db.Migrator().HasIndex(&RegisteredRepository{}, name) {
			return nil
		}
		return s.db.Migrator().DropIndex(&RegisteredRepository{}, name)
	}
	return s.db.Exec("DROP INDEX IF EXISTS " + name).Error
}

// renameLegacyIdentityColumns 把 GitLab 时代命名的身份列一次性改名(SQLite
// RENAME COLUMN,≥3.25;引用旧列的索引定义随之自动更新,索引名不变)。探测式
// 执行:旧列存在且新列不存在才动手,幂等可重入。
func (s *Store) renameLegacyIdentityColumns() error {
	columns, err := s.tableColumns()
	if err != nil {
		return err
	}
	for _, rename := range [][2]string{
		{"gitlab_project_id", "codehost_project_id"},
		{"gitlab_issue_labels", "codehost_issue_labels"},
	} {
		from, to := rename[0], rename[1]
		// 双列并存只可能是迁移序列之外的干扰(例如中途构建曾按新模型建过列):
		// 静默跳过会把存量值滞留旧列,查找失明且无告警——fail-fast 交运维处置。
		if columns[from] && columns[to] {
			return fmt.Errorf("conflicting schema vintage: registered_repositories has both %s and %s; resolve manually (drop the empty %s) and restart", from, to, to)
		}
		if !columns[from] {
			continue
		}
		if err := s.db.Exec("ALTER TABLE registered_repositories RENAME COLUMN " + from + " TO " + to).Error; err != nil {
			return fmt.Errorf("rename legacy column %s: %w", from, err)
		}
	}
	return nil
}

// tableColumns 返回 registered_repositories 现存列名集合(迁移探测用)。
// SQLite 走 pragma,MySQL 走 information_schema。
func (s *Store) tableColumns() (map[string]bool, error) {
	columns := map[string]bool{}
	if s.db.Dialector.Name() == "mysql" {
		var names []string
		if err := s.db.Raw("SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'registered_repositories'").Scan(&names).Error; err != nil {
			return nil, fmt.Errorf("inspect registered_repositories columns: %w", err)
		}
		for _, name := range names {
			columns[name] = true
		}
		return columns, nil
	}
	rows, err := s.db.Raw("pragma table_info(registered_repositories)").Rows()
	if err != nil {
		return nil, fmt.Errorf("inspect registered_repositories columns: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, pk int
		var name, kind string
		var dflt any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &dflt, &pk); err != nil {
			return nil, fmt.Errorf("scan table_info: %w", err)
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

// EnsureIdentityIndexes 建 provider 作用域唯一索引:path 与数字 project id 只在
// 同平台内唯一,跨平台同 path、同 project id 是合法共存(防串仓的结构前提)。
func (s *Store) EnsureIdentityIndexes() error {
	if s.db.Dialector.Name() == "mysql" {
		return s.ensureIdentityIndexesMySQL()
	}
	for _, ddl := range []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS uidx_registered_repos_path ON registered_repositories(provider, path_with_namespace) WHERE path_with_namespace <> ''",
		"CREATE UNIQUE INDEX IF NOT EXISTS uidx_registered_repos_provider_project ON registered_repositories(provider, codehost_project_id) WHERE codehost_project_id <> ''",
	} {
		if err := s.db.Exec(ddl).Error; err != nil {
			return fmt.Errorf("create provider-scoped identity index: %w", err)
		}
	}
	return nil
}

// ensureIdentityIndexesMySQL 用生成列等价实现空值不参与唯一性的两个身份索引。
func (s *Store) ensureIdentityIndexesMySQL() error {
	steps := []struct {
		keyColumn string
		keyDDL    string
		indexName string
		indexDDL  string
	}{
		{
			keyColumn: "identity_path_key",
			keyDDL:    "ALTER TABLE registered_repositories ADD COLUMN identity_path_key varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin GENERATED ALWAYS AS (NULLIF(path_with_namespace, '')) STORED",
			indexName: "uidx_registered_repos_path",
			indexDDL:  "CREATE UNIQUE INDEX uidx_registered_repos_path ON registered_repositories(provider, identity_path_key)",
		},
		{
			keyColumn: "identity_project_key",
			keyDDL:    "ALTER TABLE registered_repositories ADD COLUMN identity_project_key varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin GENERATED ALWAYS AS (NULLIF(codehost_project_id, '')) STORED",
			indexName: "uidx_registered_repos_provider_project",
			indexDDL:  "CREATE UNIQUE INDEX uidx_registered_repos_provider_project ON registered_repositories(provider, identity_project_key)",
		},
	}
	for _, step := range steps {
		if !s.db.Migrator().HasColumn(&RegisteredRepository{}, step.keyColumn) {
			if err := s.db.Exec(step.keyDDL).Error; err != nil {
				return fmt.Errorf("create identity key column %s: %w", step.keyColumn, err)
			}
		}
		if !s.db.Migrator().HasIndex(&RegisteredRepository{}, step.indexName) {
			if err := s.db.Exec(step.indexDDL).Error; err != nil {
				return fmt.Errorf("create provider-scoped identity index %s: %w", step.indexName, err)
			}
		}
	}
	return nil
}

// NormalizeBaseURL 归一化平台地址:小写 scheme/host、去尾斜杠、剥除与
// scheme 匹配的默认端口(https 的 :443、http 的 :80)、拒绝非 http(s)
// origin。注册摄入与存量回填共用,保证仓行 base_url 形态一致。
func NormalizeBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("base_url must be an http(s) origin")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	// 显式默认端口与省略写法是同一实例:注册侧与 webhook 消歧侧都过本函数,
	// 不剥会让两侧「差一点=不中」。
	if (parsed.Scheme == "https" && strings.HasSuffix(parsed.Host, ":443")) ||
		(parsed.Scheme == "http" && strings.HasSuffix(parsed.Host, ":80")) {
		parsed.Host = parsed.Host[:strings.LastIndex(parsed.Host, ":")]
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("base_url must be an http(s) origin")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func (s *Store) Create(ctx context.Context, repo *RegisteredRepository) error {
	if repo == nil {
		return fmt.Errorf("registered repository is required")
	}
	if repo.Status != "" && !ValidStatus(repo.Status) {
		return fmt.Errorf("invalid repository status")
	}
	if err := s.db.WithContext(ctx).Create(repo).Error; err != nil {
		// 脱敏展示但保留错误链：调用方按驱动错误码分类（重复键等）。
		return store.RedactError("create registered repository: "+err.Error(), err, repo.Token, repo.WebhookSecret)
	}
	return nil
}

func (s *Store) Update(ctx context.Context, repo *RegisteredRepository) error {
	if repo == nil {
		return fmt.Errorf("registered repository is required")
	}
	if repo.Status != "" && !ValidStatus(repo.Status) {
		return fmt.Errorf("invalid repository status")
	}
	if err := s.db.WithContext(ctx).Save(repo).Error; err != nil {
		return store.RedactError("update registered repository: "+err.Error(), err, repo.Token, repo.WebhookSecret)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, repositoryID string) (*RegisteredRepository, error) {
	return s.first(ctx, "repository_id = ?", strings.TrimSpace(repositoryID))
}

// Delete 按 repository_id 物理删除仓行（注册竞态败者的租户库回滚专用；
// 常规生命周期里仓行只切换状态，不物理删除）。
func (s *Store) Delete(ctx context.Context, repositoryID string) error {
	result := s.db.WithContext(ctx).Where("repository_id = ?", strings.TrimSpace(repositoryID)).Delete(&RegisteredRepository{})
	if result.Error != nil {
		return fmt.Errorf("delete registered repository: %w", result.Error)
	}
	return nil
}

func (s *Store) GetByProjectID(ctx context.Context, provider, projectID string) (*RegisteredRepository, error) {
	return s.first(ctx, "provider = ? AND codehost_project_id = ?", strings.ToLower(strings.TrimSpace(provider)), strings.TrimSpace(projectID))
}

func (s *Store) GetByPath(ctx context.Context, provider, path string) (*RegisteredRepository, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	path = strings.TrimSpace(path)
	repo, err := s.first(ctx, "provider = ? AND path_with_namespace = ?", provider, path)
	if err == nil || !errors.Is(err, gorm.ErrRecordNotFound) {
		return repo, err
	}
	return s.first(ctx, "provider = ? AND LOWER(path_with_namespace) = LOWER(?)", provider, path)
}

// WebhookSecretByPath 返回注册仓的按仓 webhook secret(验签专用查找键为
// provider + path_with_namespace;repoURL 是载荷自带完整链接,仅对齐
// handlers 的 WebhookSecretLookup 签名,平台库直查不消费)。未注册与仓储
// 错误一律折叠为空串(调用方按未注册 401 拒绝)；error 恒为 nil，签名仅为
// 对齐 handlers 的 WebhookSecretLookup 接口。
func (s *Store) WebhookSecretByPath(ctx context.Context, provider, path, repoURL string) (string, error) {
	repo, err := s.GetByPath(ctx, provider, path)
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(repo.WebhookSecret), nil
}

func (s *Store) List(ctx context.Context) ([]RegisteredRepository, error) {
	var items []RegisteredRepository
	if err := s.db.WithContext(ctx).Order("created_at DESC").Order("id DESC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list registered repositories: %w", err)
	}
	return items, nil
}

func (s *Store) first(ctx context.Context, query string, args ...any) (*RegisteredRepository, error) {
	var repo RegisteredRepository
	if err := s.db.WithContext(ctx).Where(query, args...).First(&repo).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, err
		}
		return nil, fmt.Errorf("get registered repository: %w", err)
	}
	return &repo, nil
}

func redactToken(message, token string) string {
	return redactSecrets(message, token)
}

// redactSecrets 把错误信息中出现的敏感值(token/webhook secret)统一替换为 ***,
// 防止驱动层错误回显配置值。
func redactSecrets(message string, secrets ...string) string {
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret == "" || !strings.Contains(message, secret) {
			continue
		}
		message = strings.ReplaceAll(message, secret, "***")
	}
	return message
}
