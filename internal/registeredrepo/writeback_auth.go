package registeredrepo

import (
	"context"
	"net/http"
	"strings"

	"sdlc-coordinator/internal/adapters/gitea"
	"sdlc-coordinator/internal/adapters/github"
	"sdlc-coordinator/internal/adapters/gitlab"
	"sdlc-coordinator/internal/model"
)

// WritebackAuthHandler 在写回遇 401/403 时复核凭据并降级仓状态;探测目标按任务
// 所属仓行的 provider/base_url 解析(平台身份按仓,无全局回退)。
type WritebackAuthHandler struct {
	store        *Store
	httpClient   *http.Client
	probeFactory func(provider string) ProjectProbe
}

func NewWritebackAuthHandler(store *Store, httpClient *http.Client) *WritebackAuthHandler {
	return &WritebackAuthHandler{
		store:      store,
		httpClient: httpClient,
		probeFactory: func(provider string) ProjectProbe {
			return NewProviderProbe(provider, httpClient)
		},
	}
}

// WithProbeFactory 注入探针工厂(测试替身用),默认按 provider 分派三平台探针。
func (handler *WritebackAuthHandler) WithProbeFactory(factory func(provider string) ProjectProbe) *WritebackAuthHandler {
	if handler != nil {
		handler.probeFactory = factory
	}
	return handler
}

func (handler *WritebackAuthHandler) HandleWritebackFailure(ctx context.Context, task *model.Task, writebackErr error) bool {
	if handler == nil || handler.store == nil || task == nil || task.RepositoryID == nil || strings.TrimSpace(*task.RepositoryID) == "" {
		return false
	}
	status := gitlab.StatusCode(writebackErr)
	if status == 0 {
		status = gitea.StatusCode(writebackErr)
	}
	if status == 0 {
		status = github.StatusCode(writebackErr)
	}
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		return false
	}
	repo, err := handler.store.Get(ctx, *task.RepositoryID)
	if err != nil || repo == nil {
		return false
	}
	tokenFailed := status == http.StatusUnauthorized
	if status == http.StatusForbidden {
		if handler.probeFactory == nil {
			return false
		}
		path := strings.TrimSpace(repo.PathWithNamespace)
		if path == "" {
			path = strings.TrimSpace(repo.CodeHostProjectID)
		}
		probe := handler.probeFactory(repo.Provider)
		_, probeErr := probe.GetProject(ctx, repo.BaseURL, repo.Token, path)
		tokenFailed = probeErr != nil
	}
	if !tokenFailed {
		return false
	}
	repo.Status = StatusConfiguring
	repo.LastConfigurationError = redactToken(writebackErr.Error(), repo.Token)
	_ = handler.store.Update(ctx, repo)
	return true
}
