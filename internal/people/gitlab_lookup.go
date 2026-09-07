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

var (
	ErrInvalidGitLabLookup = errors.New("invalid GitLab lookup request")
	ErrGitLabAuthFailed    = errors.New("GitLab authentication failed")
	ErrGitLabUnavailable   = errors.New("GitLab unavailable")
	ErrGitLabLookupFailed  = errors.New("GitLab identity lookup failed")
)

type LookupRequest struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	Token    string `json:"token"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

type LookupCandidate struct {
	ExternalUserID string `json:"external_user_id"`
	Username       string `json:"username"`
	DisplayName    string `json:"display_name"`
}

type LookupResult struct {
	Candidates      []LookupCandidate `json:"candidates"`
	ExactMatchIndex *int              `json:"exact_match_index"`
}

type IdentityLookup interface {
	Lookup(context.Context, LookupRequest) (LookupResult, error)
}

type GitLabLookupOption func(*gitLabLookupClient)

type gitLabLookupClient struct {
	httpClient *http.Client
	timeout    time.Duration
}

func NewGitLabLookupClient(httpClient *http.Client, options ...GitLabLookupOption) IdentityLookup {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	safeHTTPClient := *httpClient
	safeHTTPClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	client := &gitLabLookupClient{httpClient: &safeHTTPClient, timeout: 8 * time.Second}
	for _, option := range options {
		option(client)
	}
	return client
}

func WithGitLabLookupTimeout(timeout time.Duration) GitLabLookupOption {
	return func(client *gitLabLookupClient) {
		if timeout > 0 {
			client.timeout = timeout
		}
	}
}

func (client *gitLabLookupClient) Lookup(ctx context.Context, request LookupRequest) (LookupResult, error) {
	baseURL, username, err := normalizeLookupRequest(request)
	if err != nil {
		return LookupResult{}, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()

	endpoint, err := url.Parse(baseURL + "/api/v4/users")
	if err != nil {
		return LookupResult{}, fmt.Errorf("%w: endpoint", ErrInvalidGitLabLookup)
	}
	query := endpoint.Query()
	query.Set("search", username)
	endpoint.RawQuery = query.Encode()

	httpRequest, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return LookupResult{}, fmt.Errorf("%w: request", ErrInvalidGitLabLookup)
	}
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("PRIVATE-TOKEN", strings.TrimSpace(request.Token))

	response, err := client.httpClient.Do(httpRequest)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
			return LookupResult{}, ErrGitLabUnavailable
		}
		return LookupResult{}, ErrGitLabUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return LookupResult{}, ErrGitLabAuthFailed
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return LookupResult{}, ErrGitLabLookupFailed
	}

	var users []gitLabUser
	decoder := json.NewDecoder(http.MaxBytesReader(nil, response.Body, 1<<20))
	if err := decoder.Decode(&users); err != nil {
		return LookupResult{}, ErrGitLabLookupFailed
	}
	candidates := make([]LookupCandidate, 0, len(users))
	var exactIndices []int
	for _, user := range users {
		if strings.TrimSpace(user.Username) == "" {
			continue
		}
		candidates = append(candidates, LookupCandidate{ExternalUserID: fmt.Sprint(user.ID), Username: user.Username, DisplayName: user.Name})
		if strings.EqualFold(user.Username, username) {
			exactIndices = append(exactIndices, len(candidates)-1)
		}
	}
	result := LookupResult{Candidates: candidates}
	if len(exactIndices) == 1 {
		result.ExactMatchIndex = &exactIndices[0]
	}
	return result, nil
}

type gitLabUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

func normalizeLookupRequest(request LookupRequest) (baseURL, username string, err error) {
	if strings.ToLower(strings.TrimSpace(request.Provider)) != "gitlab" {
		return "", "", fmt.Errorf("%w: provider must be gitlab", ErrInvalidGitLabLookup)
	}
	baseURL, err = normalizeBaseURL(request.BaseURL)
	if err != nil {
		return "", "", fmt.Errorf("%w: invalid base_url", ErrInvalidGitLabLookup)
	}
	if strings.TrimSpace(request.Token) == "" {
		return "", "", fmt.Errorf("%w: token is required", ErrInvalidGitLabLookup)
	}
	username = strings.TrimSpace(request.Username)
	if username == "" {
		email := strings.TrimSpace(request.Email)
		at := strings.Index(email, "@")
		if at > 0 {
			username = email[:at]
		}
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return "", "", fmt.Errorf("%w: email or username is required", ErrInvalidGitLabLookup)
	}
	return baseURL, username, nil
}
