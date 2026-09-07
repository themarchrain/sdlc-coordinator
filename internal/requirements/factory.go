package requirements

import (
	"context"
	"net/http"
	"strings"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/registeredrepo"
)

// ClientFactory 是需求同步器需要的最小 codehost 能力集合,
// 三平台 client(gitlab/gitea/github)天然满足,不新增平台分支实现。
type ClientFactory interface {
	CreateIssue(ctx context.Context, projectID string, request codehost.CreateIssueRequest) (*codehost.Issue, error)
	ReconcileWriteback(ctx context.Context, request codehost.ReconcileRequest) (codehost.ReconcileResult, error)
	GetIssueState(ctx context.Context, projectID string, iid int) (codehost.IssueState, error)
}

// ClientFactoryFunc 按仓行构造一次性 client;client 无状态,构造成本可忽略。
// 平台身份取仓行的 provider/base_url(按仓,无全局回退);身份缺失时返回恒错
// client,由同步链 markFailure 落账,不 panic、不静默跳过。
type ClientFactoryFunc func(repo *registeredrepo.RegisteredRepository) ClientFactory

func NewProviderClientFactory(httpClient *http.Client) ClientFactoryFunc {
	return func(repo *registeredrepo.RegisteredRepository) ClientFactory {
		client, err := registeredrepo.CodeHostClient(repo.Provider, repo.BaseURL, repo.Token, httpClient)
		if err != nil {
			return failingClient{err: err}
		}
		return client
	}
}

type failingClient struct{ err error }

func (c failingClient) CreateIssue(context.Context, string, codehost.CreateIssueRequest) (*codehost.Issue, error) {
	return nil, c.err
}

func (c failingClient) ReconcileWriteback(context.Context, codehost.ReconcileRequest) (codehost.ReconcileResult, error) {
	return codehost.ReconcileResult{}, c.err
}

func (c failingClient) GetIssueState(context.Context, string, int) (codehost.IssueState, error) {
	return codehost.IssueState{}, c.err
}

// ResolveProjectID 与 writeback 链路语义对齐:gitlab 优先数字项目 ID,其余平台用完整路径。
func ResolveProjectID(repo *registeredrepo.RegisteredRepository) string {
	if strings.EqualFold(strings.TrimSpace(repo.Provider), "gitlab") && strings.TrimSpace(repo.CodeHostProjectID) != "" {
		return strings.TrimSpace(repo.CodeHostProjectID)
	}
	return repo.PathWithNamespace
}
