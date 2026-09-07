package people

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Gitea 平台身份在线查询:/api/v1/users/{username}(管理员 token 可查任意用户)。
type giteaLookupClient struct {
	httpClient *http.Client
	timeout    time.Duration
}

func NewGiteaLookupClient(httpClient *http.Client) IdentityLookup {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	safeHTTPClient := *httpClient
	safeHTTPClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &giteaLookupClient{httpClient: &safeHTTPClient, timeout: 8 * time.Second}
}

func (client *giteaLookupClient) Lookup(ctx context.Context, request LookupRequest) (LookupResult, error) {
	baseURL, username, err := normalizeGiteaLookupRequest(request)
	if err != nil {
		return LookupResult{}, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()

	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/users/" + url.PathEscape(username)
	httpRequest, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return LookupResult{}, fmt.Errorf("%w: request", ErrInvalidGiteaLookup)
	}
	httpRequest.Header.Set("Accept", "application/json")
	if token := strings.TrimSpace(request.Token); token != "" {
		httpRequest.Header.Set("Authorization", "token "+token)
	}

	response, err := client.httpClient.Do(httpRequest)
	if err != nil {
		return LookupResult{}, ErrGiteaUnavailable
	}
	defer response.Body.Close()
	// 404(查无此人)不是故障:邮箱前缀猜用户名未命中是常态,返回空候选让
	// 批量补全继续、单条查询退回手工填写,与 GitLab search 无结果的语义对齐。
	if response.StatusCode == http.StatusNotFound {
		return LookupResult{Candidates: []LookupCandidate{}}, nil
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return LookupResult{}, ErrGiteaAuthFailed
	}
	if response.StatusCode != http.StatusOK {
		return LookupResult{}, ErrGiteaUnavailable
	}

	var user struct {
		ID       int64  `json:"id"`
		Login    string `json:"login"`
		FullName string `json:"full_name"`
	}
	if err := json.NewDecoder(response.Body).Decode(&user); err != nil {
		return LookupResult{}, fmt.Errorf("%w: response", ErrInvalidGiteaLookup)
	}
	if strings.TrimSpace(user.Login) == "" {
		return LookupResult{}, fmt.Errorf("%w: empty username", ErrInvalidGiteaLookup)
	}
	displayName := strings.TrimSpace(user.FullName)
	if displayName == "" {
		displayName = strings.TrimSpace(user.Login)
	}
	index := 0
	return LookupResult{
		Candidates: []LookupCandidate{{
			ExternalUserID: fmt.Sprintf("%d", user.ID),
			Username:       strings.TrimSpace(user.Login),
			DisplayName:    displayName,
		}},
		ExactMatchIndex: &index,
	}, nil
}

var (
	ErrInvalidGiteaLookup = errors.New("invalid gitea lookup")
	ErrGiteaUnavailable   = errors.New("gitea unavailable")
)

// normalizeGiteaLookupRequest 校验 Gitea 查询请求;用户名可由邮箱 @ 前缀兜底
// (Gitea 用户名惯例与邮箱前缀一致,查不到时单条失败,不阻塞批次)。
func normalizeGiteaLookupRequest(request LookupRequest) (baseURL, username string, err error) {
	if strings.ToLower(strings.TrimSpace(request.Provider)) != "gitea" {
		return "", "", fmt.Errorf("%w: provider must be gitea", ErrInvalidGiteaLookup)
	}
	baseURL, err = normalizeBaseURL(request.BaseURL)
	if err != nil {
		return "", "", fmt.Errorf("%w: invalid base_url", ErrInvalidGiteaLookup)
	}
	if strings.TrimSpace(request.Token) == "" {
		return "", "", fmt.Errorf("%w: token is required", ErrInvalidGiteaLookup)
	}
	username = strings.TrimSpace(request.Username)
	if username == "" {
		email := strings.TrimSpace(request.Email)
		if at := strings.Index(email, "@"); at > 0 {
			username = email[:at]
		}
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return "", "", fmt.Errorf("%w: email or username is required", ErrInvalidGiteaLookup)
	}
	return baseURL, username, nil
}
