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

// GitHub 平台身份在线查询:/users/{username}(仓库 token 即可查公开身份)。
type githubLookupClient struct {
	httpClient *http.Client
	timeout    time.Duration
}

func NewGitHubLookupClient(httpClient *http.Client) IdentityLookup {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	safeHTTPClient := *httpClient
	safeHTTPClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &githubLookupClient{httpClient: &safeHTTPClient, timeout: 8 * time.Second}
}

func (client *githubLookupClient) Lookup(ctx context.Context, request LookupRequest) (LookupResult, error) {
	baseURL, username, err := normalizeGitHubLookupRequest(request)
	if err != nil {
		return LookupResult{}, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()

	endpoint := strings.TrimRight(baseURL, "/") + "/users/" + url.PathEscape(username)
	httpRequest, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return LookupResult{}, fmt.Errorf("%w: request", ErrInvalidGitHubLookup)
	}
	httpRequest.Header.Set("Accept", "application/vnd.github+json")
	if token := strings.TrimSpace(request.Token); token != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+token)
	}

	response, err := client.httpClient.Do(httpRequest)
	if err != nil {
		return LookupResult{}, ErrGitHubUnavailable
	}
	defer response.Body.Close()
	// 404(查无此人)不是故障:返回空候选让批量补全继续、单条查询退回手工填写。
	if response.StatusCode == http.StatusNotFound {
		return LookupResult{Candidates: []LookupCandidate{}}, nil
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return LookupResult{}, ErrGitHubAuthFailed
	}
	if response.StatusCode != http.StatusOK {
		return LookupResult{}, ErrGitHubUnavailable
	}

	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(response.Body).Decode(&user); err != nil {
		return LookupResult{}, fmt.Errorf("%w: response", ErrInvalidGitHubLookup)
	}
	if strings.TrimSpace(user.Login) == "" {
		return LookupResult{}, fmt.Errorf("%w: empty username", ErrInvalidGitHubLookup)
	}
	displayName := strings.TrimSpace(user.Name)
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

func normalizeGitHubLookupRequest(request LookupRequest) (baseURL, username string, err error) {
	if strings.ToLower(strings.TrimSpace(request.Provider)) != "github" {
		return "", "", fmt.Errorf("%w: provider must be github", ErrInvalidGitHubLookup)
	}
	baseURL = strings.TrimSpace(request.BaseURL)
	if baseURL == "" {
		return "", "", fmt.Errorf("%w: invalid base_url", ErrInvalidGitHubLookup)
	}
	if strings.TrimSpace(request.Token) == "" {
		return "", "", fmt.Errorf("%w: token is required", ErrInvalidGitHubLookup)
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
		return "", "", fmt.Errorf("%w: email or username is required", ErrInvalidGitHubLookup)
	}
	return baseURL, username, nil
}

var (
	ErrInvalidGitHubLookup = errors.New("invalid github lookup")
	ErrGitHubUnavailable   = errors.New("github unavailable")
)
