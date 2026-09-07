package registeredrepo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

var (
	ErrTokenNotAttested  = errors.New("token_not_attested")
	ErrRepositoryBusy    = errors.New("repository_busy")
	ErrMustDisableFirst  = errors.New("repository_must_disable_first")
	ErrIdentityMismatch  = errors.New("repository_identity_mismatch")
	ErrNotFound          = errors.New("repository_not_found")
	ErrInvalidRepository = errors.New("invalid_repository")
	// ErrWebhookSecretRequired 表示按仓 webhook secret 缺失。secret 是该仓事件
	// 验签的唯一凭据(fail-closed),缺失时 enable 必须拒绝,把补配强制在配置面。
	ErrWebhookSecretRequired = errors.New("webhook_secret_required")
	// ErrPlatformIdentityRequired 表示仓行平台身份(provider/base_url)缺失——
	// 回填不可达的理论残余;启用前必须在配置面补齐。
	ErrPlatformIdentityRequired = errors.New("platform_identity_required")
	// ErrInvalidPlatformIdentity 表示注册/更新提交的平台身份不合法(provider
	// 枚举外或 base_url 非 http(s) origin)。
	ErrInvalidPlatformIdentity = errors.New("invalid_platform_identity")
)

// NormalizeProvider/ValidProvider 是 provider 的统一规范与枚举;仓行、事件
// Source、静态 endpoint 映射共用同一取值域(gitlab/gitea/github)。
func NormalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

func ValidProvider(provider string) bool {
	switch provider {
	case "gitlab", "gitea", "github":
		return true
	}
	return false
}

type CreateRequest struct {
	DisplayName         string
	PathWithNamespace   string
	Provider            string
	BaseURL             string
	Token               string
	TokenScopeAttested  bool
	WebhookSecret       string
	CodeHostIssueLabels []string
	ReviewerMentions    string
}

type PatchRequest struct {
	DisplayName         *string
	PathWithNamespace   *string
	Provider            *string
	BaseURL             *string
	Token               string
	TokenScopeAttested  *bool
	WebhookSecret       string
	CodeHostIssueLabels *[]string
	ReviewerMentions    *string
}

type View struct {
	RepositoryID            string
	DisplayName             string
	Provider                string
	BaseURL                 string
	CodeHostProjectID       string
	PathWithNamespace       string
	TokenConfigured         bool
	WebhookSecretConfigured bool
	DefaultBranch           string
	Status                  RepositoryStatus
	CodeHostIssueLabels     []string
	LastConfigurationError  string
}

func ViewFrom(repo *RegisteredRepository) View {
	if repo == nil {
		return View{}
	}
	return View{
		RepositoryID:            repo.RepositoryID,
		DisplayName:             repo.DisplayName,
		Provider:                repo.Provider,
		BaseURL:                 repo.BaseURL,
		CodeHostProjectID:       repo.CodeHostProjectID,
		PathWithNamespace:       repo.PathWithNamespace,
		TokenConfigured:         strings.TrimSpace(repo.Token) != "",
		WebhookSecretConfigured: strings.TrimSpace(repo.WebhookSecret) != "",
		DefaultBranch:           repo.DefaultBranch,
		Status:                  repo.Status,
		CodeHostIssueLabels:     decodeLabels(repo.CodeHostIssueLabels),
		LastConfigurationError:  repo.LastConfigurationError,
	}
}

type Service struct {
	store        *Store
	httpClient   *http.Client
	activity     ActivityChecker
	probeFactory func(provider string) ProjectProbe
}

// NewService 构造注册服务:探针按仓行的 provider/base_url 构造(平台身份按仓),
// 不再持有全局 provider/baseURL。
func NewService(store *Store, httpClient *http.Client, activity ActivityChecker) *Service {
	if activity == nil {
		activity = noopActivity{}
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Service{
		store:        store,
		httpClient:   httpClient,
		activity:     activity,
		probeFactory: func(provider string) ProjectProbe { return NewProviderProbe(provider, httpClient) },
	}
}

// WithProbeFactory 注入探针工厂(测试替身用),默认按 provider 分派三平台探针。
func (s *Service) WithProbeFactory(factory func(provider string) ProjectProbe) *Service {
	if s != nil {
		s.probeFactory = factory
	}
	return s
}

type noopActivity struct{}

func (noopActivity) HasActiveWork(context.Context, string) (bool, error) { return false, nil }

func (s *Service) Create(ctx context.Context, request CreateRequest) (*View, error) {
	if !request.TokenScopeAttested {
		return nil, ErrTokenNotAttested
	}
	path := strings.TrimSpace(request.PathWithNamespace)
	if path == "" || strings.TrimSpace(request.Token) == "" {
		return nil, fmt.Errorf("%w: path and token are required", ErrInvalidRepository)
	}
	if strings.TrimSpace(request.WebhookSecret) == "" {
		return nil, ErrWebhookSecretRequired
	}
	// 平台身份是仓行锚点:注册即必须给出合法 provider 与 base_url,消费链
	// (验签/写回/探针/人员/需求)全部按它分派。
	provider := NormalizeProvider(request.Provider)
	if !ValidProvider(provider) {
		return nil, fmt.Errorf("%w: provider must be gitlab, gitea or github", ErrInvalidPlatformIdentity)
	}
	baseURL, err := NormalizeBaseURL(request.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidPlatformIdentity, err)
	}
	// secret 摄入即去首尾空白:平台侧按原值计算签名,行内保存归一化值,
	// 避免配置面误带空白导致运行期验签恒败。
	repo := &RegisteredRepository{
		DisplayName:         strings.TrimSpace(request.DisplayName),
		PathWithNamespace:   path,
		Provider:            provider,
		BaseURL:             baseURL,
		Token:               request.Token,
		WebhookSecret:       strings.TrimSpace(request.WebhookSecret),
		TokenScopeAttested:  true,
		CodeHostIssueLabels: encodeLabels(request.CodeHostIssueLabels),
		ReviewerMentions:    request.ReviewerMentions,
		Status:              StatusConfiguring,
	}
	if repo.DisplayName == "" {
		repo.DisplayName = path
	}
	if err := s.store.Create(ctx, repo); err != nil {
		return nil, err
	}
	if err := s.refreshIdentity(ctx, repo); err != nil {
		view := ViewFrom(repo)
		return &view, err
	}
	view := ViewFrom(repo)
	return &view, nil
}

func (s *Service) List(ctx context.Context) ([]View, error) {
	items, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]View, 0, len(items))
	for i := range items {
		views = append(views, ViewFrom(&items[i]))
	}
	return views, nil
}

func (s *Service) Enable(ctx context.Context, repositoryID string) (*View, error) {
	repo, err := s.store.Get(ctx, repositoryID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if !repo.TokenScopeAttested || strings.TrimSpace(repo.Token) == "" {
		return nil, ErrTokenNotAttested
	}
	if strings.TrimSpace(repo.WebhookSecret) == "" {
		return nil, ErrWebhookSecretRequired
	}
	if NormalizeProvider(repo.Provider) == "" || strings.TrimSpace(repo.BaseURL) == "" {
		return nil, ErrPlatformIdentityRequired
	}
	if err := s.refreshIdentity(ctx, repo); err != nil {
		view := ViewFrom(repo)
		return &view, err
	}
	// 每仓 clone Token 保存在仓库行本身。身份探活通过即启用。
	repo.Status = StatusEnabled
	repo.LastConfigurationError = ""
	if err := s.store.Update(ctx, repo); err != nil {
		return nil, err
	}
	view := ViewFrom(repo)
	return &view, nil
}

func (s *Service) Get(ctx context.Context, repositoryID string) (*View, error) {
	repo, err := s.store.Get(ctx, repositoryID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	view := ViewFrom(repo)
	return &view, nil
}

func (s *Service) Disable(ctx context.Context, repositoryID string) error {
	repo, err := s.store.Get(ctx, repositoryID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	busy, err := s.activity.HasActiveWork(ctx, repo.RepositoryID)
	if err != nil {
		return err
	}
	if busy {
		return ErrRepositoryBusy
	}
	repo.Status = StatusDisabled
	return s.store.Update(ctx, repo)
}

func (s *Service) Patch(ctx context.Context, repositoryID string, request PatchRequest) (*View, error) {
	repo, err := s.store.Get(ctx, repositoryID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	tokenChanged := strings.TrimSpace(request.Token) != ""
	pathChanged := request.PathWithNamespace != nil && strings.TrimSpace(*request.PathWithNamespace) != "" && strings.TrimSpace(*request.PathWithNamespace) != repo.PathWithNamespace
	providerChanged := request.Provider != nil && strings.TrimSpace(*request.Provider) != "" && NormalizeProvider(*request.Provider) != repo.Provider
	// base_url 以归一化形态比较:仅大小写/尾斜杠差异不算变更,避免无意义的
	// 降级 reconfigure 与出网探测。
	baseURLChanged := false
	var normalizedBaseURL string
	if request.BaseURL != nil && strings.TrimSpace(*request.BaseURL) != "" {
		normalized, normErr := NormalizeBaseURL(*request.BaseURL)
		if normErr != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidPlatformIdentity, normErr)
		}
		baseURLChanged = normalized != repo.BaseURL
		normalizedBaseURL = normalized
	}
	if repo.Status == StatusEnabled && (tokenChanged || pathChanged || providerChanged || baseURLChanged) {
		return nil, ErrMustDisableFirst
	}
	if request.DisplayName != nil {
		repo.DisplayName = strings.TrimSpace(*request.DisplayName)
	}
	if pathChanged {
		repo.PathWithNamespace = strings.TrimSpace(*request.PathWithNamespace)
	}
	// 平台身份与 token/path 同级:变更会改写验签、写回与探测目标,enabled 仓
	// 必须先停用;空值表示保持(与 token 同语义)。
	if providerChanged {
		provider := NormalizeProvider(*request.Provider)
		if !ValidProvider(provider) {
			return nil, fmt.Errorf("%w: provider must be gitlab, gitea or github", ErrInvalidPlatformIdentity)
		}
		repo.Provider = provider
		// 换平台后旧平台的数字 project id 不再可比:清空锚点让 refreshIdentity
		// 按新平台重新获取身份,而不是误判 repository_identity_mismatch。
		repo.CodeHostProjectID = ""
	}
	if baseURLChanged {
		repo.BaseURL = normalizedBaseURL
	}
	if tokenChanged {
		if request.TokenScopeAttested == nil || !*request.TokenScopeAttested {
			return nil, ErrTokenNotAttested
		}
		repo.Token = request.Token
		repo.TokenScopeAttested = true
	}
	// webhook secret 轮换不影响平台身份与 git 凭据,空值表示保持(不支持清空)。
	if strings.TrimSpace(request.WebhookSecret) != "" {
		repo.WebhookSecret = strings.TrimSpace(request.WebhookSecret)
	}
	if request.CodeHostIssueLabels != nil {
		repo.CodeHostIssueLabels = encodeLabels(*request.CodeHostIssueLabels)
	}
	if request.ReviewerMentions != nil {
		repo.ReviewerMentions = *request.ReviewerMentions
	}
	repo.Status = StatusConfiguring
	if err := s.store.Update(ctx, repo); err != nil {
		return nil, err
	}
	if pathChanged || tokenChanged || providerChanged || baseURLChanged {
		if err := s.refreshIdentity(ctx, repo); err != nil {
			view := ViewFrom(repo)
			return &view, err
		}
	}
	view := ViewFrom(repo)
	return &view, nil
}

func encodeLabels(values []string) string {
	encoded, _ := json.Marshal(cleanLabels(values))
	return string(encoded)
}

func decodeLabels(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{"developer-agent"}
	}
	var values []string
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &values) != nil {
		return []string{}
	}
	return cleanLabels(values)
}

func cleanLabels(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (s *Service) refreshIdentity(ctx context.Context, repo *RegisteredRepository) error {
	if s.probeFactory == nil {
		return fmt.Errorf("project probe is not configured")
	}
	// 探针按仓行平台身份构造:provider 决定探测协议,base_url 决定探测目标。
	probe := s.probeFactory(repo.Provider)
	ident, err := probe.GetProject(ctx, repo.BaseURL, repo.Token, repo.PathWithNamespace)
	if err != nil {
		repo.LastConfigurationError = redactToken(err.Error(), repo.Token)
		_ = s.store.Update(ctx, repo)
		return fmt.Errorf("probe project: %s", redactToken(err.Error(), repo.Token))
	}
	if repo.CodeHostProjectID != "" && ident.ID != "" && ident.ID != repo.CodeHostProjectID {
		repo.LastConfigurationError = "repository_identity_mismatch"
		_ = s.store.Update(ctx, repo)
		return ErrIdentityMismatch
	}
	if ident.ID != "" {
		repo.CodeHostProjectID = ident.ID
	}
	if ident.PathWithNamespace != "" {
		repo.PathWithNamespace = ident.PathWithNamespace
	}
	if ident.DefaultBranch != "" {
		repo.DefaultBranch = ident.DefaultBranch
	}
	repo.LastConfigurationError = ""
	return s.store.Update(ctx, repo)
}

type SQLActivityChecker struct {
	db *gorm.DB
}

func NewSQLActivityChecker(db *gorm.DB) SQLActivityChecker {
	return SQLActivityChecker{db: db}
}

func (checker SQLActivityChecker) HasActiveWork(ctx context.Context, repositoryID string) (bool, error) {
	repositoryID = strings.TrimSpace(repositoryID)
	if repositoryID == "" {
		return false, nil
	}
	activeTasks := []model.TaskStatus{model.TaskStatusClaimed, model.TaskStatusRunning, model.TaskStatusFinalizing}
	var taskCount int64
	if err := checker.db.WithContext(ctx).Model(&model.Task{}).
		Where("repository_id = ? AND status IN ?", repositoryID, activeTasks).
		Count(&taskCount).Error; err != nil {
		return false, fmt.Errorf("count active tasks: %w", err)
	}
	if taskCount > 0 {
		return true, nil
	}
	activeRuns := []model.RunStatus{model.RunStatusRunning, model.RunStatusFinalizing, model.RunStatusPending}
	var runCount int64
	if err := checker.db.WithContext(ctx).Model(&model.Run{}).
		Joins("JOIN tasks ON tasks.task_id = runs.task_id").
		Where("tasks.repository_id = ? AND runs.status IN ?", repositoryID, activeRuns).
		Count(&runCount).Error; err != nil {
		return false, fmt.Errorf("count active runs: %w", err)
	}
	return runCount > 0, nil
}
