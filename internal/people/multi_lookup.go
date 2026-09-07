package people

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// multiProviderLookup 按请求的 provider 分派到对应平台 client;人员查询的作用域
// (provider/base_url)由调用方按仓行传入,装配层不再单选平台。
type multiProviderLookup struct {
	gitlab, gitea, github IdentityLookup
}

// NewMultiProviderLookupClient 构造三平台全量的分派 lookup:单套部署可同时服务
// 多平台注册仓,lookup 目标由每个请求自带的作用域决定。
func NewMultiProviderLookupClient(httpClient *http.Client) IdentityLookup {
	return multiProviderLookup{
		gitlab: NewGitLabLookupClient(httpClient),
		gitea:  NewGiteaLookupClient(httpClient),
		github: NewGitHubLookupClient(httpClient),
	}
}

func newMultiProviderLookup(gitlab, gitea, github IdentityLookup) IdentityLookup {
	return multiProviderLookup{gitlab: gitlab, gitea: gitea, github: github}
}

func (m multiProviderLookup) Lookup(ctx context.Context, request LookupRequest) (LookupResult, error) {
	switch strings.ToLower(strings.TrimSpace(request.Provider)) {
	case "gitlab":
		return m.gitlab.Lookup(ctx, request)
	case "gitea":
		return m.gitea.Lookup(ctx, request)
	case "github":
		return m.github.Lookup(ctx, request)
	default:
		return LookupResult{}, fmt.Errorf("%w: provider %q", ErrInvalidBatchLookup, request.Provider)
	}
}
