package registeredrepo

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/adapters/gitea"
	"sdlc-coordinator/internal/adapters/github"
	"sdlc-coordinator/internal/adapters/gitlab"
	"sdlc-coordinator/internal/model"
)

// PlatformClient 是三平台 client 的并集形状:写回链按 codehost.Backend 面
// 消费,需求同步按 requirements.ClientFactory 面消费;构造统一走 CodeHostClient,
// 平台分派单一来源。
type PlatformClient interface {
	codehost.Backend
	ReconcileWriteback(context.Context, codehost.ReconcileRequest) (codehost.ReconcileResult, error)
	GetIssueState(context.Context, string, int) (codehost.IssueState, error)
}

// CodeHostClient 按仓行平台身份构造对应平台 client;身份缺失或 provider 不明时
// fail-closed(不得静默回退 GitLab,回退会让跨平台投递写错平台)。
func CodeHostClient(provider, baseURL, token string, httpClient *http.Client) (PlatformClient, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("repository platform base_url is not configured")
	}
	switch provider {
	case "gitea":
		return gitea.NewClient(baseURL, token, gitea.WithHTTPClient(httpClient)), nil
	case "github":
		return github.NewClient(baseURL, token, github.WithHTTPClient(httpClient)), nil
	case "gitlab":
		return gitlab.NewClient(baseURL, token, gitlab.WithHTTPClient(httpClient)), nil
	default:
		return nil, fmt.Errorf("unsupported repository provider %q", provider)
	}
}

type CodeHostResolver struct {
	store      *Store
	httpClient *http.Client
}

// NewCodeHostResolver 构造按仓 resolver:任务所属仓行的 provider/base_url 是
// 唯一平台身份来源,构造期不再持有全局平台配置。
func NewCodeHostResolver(store *Store, httpClient *http.Client) *CodeHostResolver {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &CodeHostResolver{store: store, httpClient: httpClient}
}

func (resolver *CodeHostResolver) Resolve(ctx context.Context, task *model.Task, writeback *model.Writeback) (codehost.Backend, error) {
	if resolver == nil || resolver.store == nil {
		return nil, fmt.Errorf("repository store is required")
	}
	if task == nil || task.RepositoryID == nil || strings.TrimSpace(*task.RepositoryID) == "" {
		return nil, fmt.Errorf("task repository_id is required")
	}
	repo, err := resolver.store.Get(ctx, *task.RepositoryID)
	if err != nil {
		return nil, err
	}
	if repo.Status != StatusEnabled {
		return nil, fmt.Errorf("repository_not_enabled")
	}
	project := strings.TrimSpace(writeback.ProjectID)
	if project != repo.CodeHostProjectID && project != repo.PathWithNamespace {
		return nil, fmt.Errorf("writeback project does not belong to repository")
	}
	return CodeHostClient(repo.Provider, repo.BaseURL, repo.Token, resolver.httpClient)
}
