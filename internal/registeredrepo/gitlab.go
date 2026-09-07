package registeredrepo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"sdlc-coordinator/internal/adapters/gitlab"
)

type ProjectIdentity struct {
	ID                string
	PathWithNamespace string
	DefaultBranch     string
}

type ProjectProbe interface {
	GetProject(ctx context.Context, baseURL, token, pathWithNamespace string) (ProjectIdentity, error)
}

type ActivityChecker interface {
	HasActiveWork(ctx context.Context, repositoryID string) (bool, error)
}

type HTTPProbe struct {
	HTTP *http.Client
}

func NewHTTPProbe(httpClient *http.Client) HTTPProbe {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return HTTPProbe{HTTP: httpClient}
}

func (probe HTTPProbe) GetProject(ctx context.Context, baseURL, token, pathWithNamespace string) (ProjectIdentity, error) {
	client := gitlab.NewClient(baseURL, token, gitlab.WithHTTPClient(probe.HTTP))
	project, err := client.GetProject(ctx, pathWithNamespace)
	if err != nil {
		return ProjectIdentity{}, err
	}
	if project == nil || project.ID == 0 {
		return ProjectIdentity{}, fmt.Errorf("gitlab project identity missing")
	}
	return ProjectIdentity{
		ID:                strconv.Itoa(project.ID),
		PathWithNamespace: project.PathWithNamespace,
		DefaultBranch:     project.DefaultBranch,
	}, nil
}

// GiteaHTTPProbe 通过 /api/v1/repos/{owner}/{repo} 拉取仓库身份,与 GitLab
// 探针共用 ProjectIdentity 形状(数字 ID / full_name / default branch)。
type GiteaHTTPProbe struct {
	HTTP *http.Client
}

func NewGiteaHTTPProbe(httpClient *http.Client) GiteaHTTPProbe {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return GiteaHTTPProbe{HTTP: httpClient}
}

func (probe GiteaHTTPProbe) GetProject(ctx context.Context, baseURL, token, pathWithNamespace string) (ProjectIdentity, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/api/v1/repos/" + strings.Trim(pathWithNamespace, "/")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ProjectIdentity{}, fmt.Errorf("create gitea probe request: %w", err)
	}
	if strings.TrimSpace(token) != "" {
		request.Header.Set("Authorization", "token "+strings.TrimSpace(token))
	}
	response, err := probe.HTTP.Do(request)
	if err != nil {
		return ProjectIdentity{}, fmt.Errorf("gitea probe request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ProjectIdentity{}, fmt.Errorf("gitea probe: %s", response.Status)
	}
	var project struct {
		ID            int64  `json:"id"`
		FullName      string `json:"full_name"`
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(response.Body).Decode(&project); err != nil {
		return ProjectIdentity{}, fmt.Errorf("decode gitea probe response: %w", err)
	}
	if project.ID == 0 {
		return ProjectIdentity{}, fmt.Errorf("gitea project identity missing")
	}
	return ProjectIdentity{
		ID:                strconv.FormatInt(project.ID, 10),
		PathWithNamespace: project.FullName,
		DefaultBranch:     project.DefaultBranch,
	}, nil
}

// NewProviderProbe 按 codehost provider 返回仓库身份探针。未知 provider 回退
// GitLab 仅作防御默认:所有生产写入路径(Create/Patch 校验、Enable 门禁)已保证
// provider ∈ 枚举,探针永远收到合法值;此回退不可作为跨平台分派的依赖。
func NewProviderProbe(provider string, httpClient *http.Client) ProjectProbe {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "gitea":
		return NewGiteaHTTPProbe(httpClient)
	case "github":
		return NewGitHubHTTPProbe(httpClient)
	default:
		return NewHTTPProbe(httpClient)
	}
}

// GitHubHTTPProbe 通过 /repos/{owner}/{repo} 拉取仓库身份,与 GitLab/Gitea
// 探针共用 ProjectIdentity 形状(数字 ID / full_name / default branch)。
type GitHubHTTPProbe struct {
	HTTP *http.Client
}

func NewGitHubHTTPProbe(httpClient *http.Client) GitHubHTTPProbe {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return GitHubHTTPProbe{HTTP: httpClient}
}

func (probe GitHubHTTPProbe) GetProject(ctx context.Context, baseURL, token, pathWithNamespace string) (ProjectIdentity, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/repos/" + strings.Trim(pathWithNamespace, "/")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ProjectIdentity{}, fmt.Errorf("create github probe request: %w", err)
	}
	if strings.TrimSpace(token) != "" {
		request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	}
	response, err := probe.HTTP.Do(request)
	if err != nil {
		return ProjectIdentity{}, fmt.Errorf("github probe request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ProjectIdentity{}, fmt.Errorf("github probe: %s", response.Status)
	}
	var project struct {
		ID            int64  `json:"id"`
		FullName      string `json:"full_name"`
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(response.Body).Decode(&project); err != nil {
		return ProjectIdentity{}, fmt.Errorf("decode github probe response: %w", err)
	}
	if project.ID == 0 {
		return ProjectIdentity{}, fmt.Errorf("github project identity missing")
	}
	return ProjectIdentity{
		ID:                strconv.FormatInt(project.ID, 10),
		PathWithNamespace: project.FullName,
		DefaultBranch:     project.DefaultBranch,
	}, nil
}
