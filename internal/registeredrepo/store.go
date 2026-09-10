package registeredrepo

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

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
	// 下列索引名是改列名前的字面名;SQLite RENAME COLUMN 不改索引名,须按旧名 drop。
	for _, name := range []string{
		"idx_registered_repositories_path_with_namespace",
		"idx_registered_repositories_gitlab_project_id",
		"uidx_registered_repos_project",
	} {
		if err := s.db.Exec("DROP INDEX IF EXISTS " + name).Error; err != nil {
			return fmt.Errorf("drop legacy index %s: %w", name, err)
		}
	}
	return nil
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
func (s *Store) tableColumns() (map[string]bool, error) {
	rows, err := s.db.Raw("pragma table_info(registered_repositories)").Rows()
	if err != nil {
		return nil, fmt.Errorf("inspect registered_repositories columns: %w", err)
	}
	defer rows.Close()
	columns := map[string]bool{}
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
// project 索引使用新名(provider 前缀语义),避免与 Migrate 会 drop 的旧
// 单列索引同名而被反复 drop/rebuild。
func (s *Store) EnsureIdentityIndexes() error {
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

// NormalizeBaseURL 归一化平台地址:小写 scheme/host、去尾斜杠、拒绝非 http(s)
// origin。注册摄入与存量回填共用,保证仓行 base_url 形态一致。
func NormalizeBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("base_url must be an http(s) origin")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
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
		return fmt.Errorf("create registered repository: %s", redactSecrets(err.Error(), repo.Token, repo.WebhookSecret))
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
		return fmt.Errorf("update registered repository: %s", redactSecrets(err.Error(), repo.Token, repo.WebhookSecret))
	}
	return nil
}

func (s *Store) Get(ctx context.Context, repositoryID string) (*RegisteredRepository, error) {
	return s.first(ctx, "repository_id = ?", strings.TrimSpace(repositoryID))
}

func (s *Store) GetByProjectID(ctx context.Context, provider, projectID string) (*RegisteredRepository, error) {
	return s.first(ctx, "provider = ? AND codehost_project_id = ?", strings.ToLower(strings.TrimSpace(provider)), strings.TrimSpace(projectID))
}

// GetByPath 精确匹配优先;未命中时按 COLLATE NOCASE 兜底一次(R-P3-4:平台规范
// 路径回写前的窗口内,投递大小写不一致仍可命中),兜底保持 provider 作用域。
func (s *Store) GetByPath(ctx context.Context, provider, path string) (*RegisteredRepository, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	path = strings.TrimSpace(path)
	repo, err := s.first(ctx, "provider = ? AND path_with_namespace = ?", provider, path)
	if err == nil || !errors.Is(err, gorm.ErrRecordNotFound) {
		return repo, err
	}
	return s.first(ctx, "provider = ? AND path_with_namespace = ? COLLATE NOCASE", provider, path)
}

// WebhookSecretByPath 返回注册仓的按仓 webhook secret(验签专用查找键为
// provider + path_with_namespace)。未注册返回 gorm.ErrRecordNotFound;调用方
// (handler 验签)对任何错误与空 secret 一律 fail-closed 拒绝。
func (s *Store) WebhookSecretByPath(ctx context.Context, provider, path string) (string, error) {
	repo, err := s.GetByPath(ctx, provider, path)
	if err != nil {
		return "", err
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
