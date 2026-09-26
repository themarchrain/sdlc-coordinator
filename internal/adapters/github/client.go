package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"sdlc-coordinator/internal/adapters/codehost"
)

// Client 走 GitHub REST API;github.com 的仓库直接配网页域 https://github.com,
// REST 侧经 APIBaseURL 派生 API 域(GHE 仍配 https://{host}/api/v3),
// base URL 之外不做路径加工。
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type Option func(*Client)

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

func NewClient(baseURL, token string, opts ...Option) *Client {
	client := &Client{
		baseURL:    APIBaseURL(baseURL),
		token:      token,
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(client)
	}
	if client.httpClient == nil {
		client.httpClient = http.DefaultClient
	}
	return client
}

type idResponse struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
}

type noteResponse struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
}

type pullResponse struct {
	Number         int           `json:"number"`
	HTMLURL        string        `json:"html_url"`
	Merged         bool          `json:"merged"`
	MergeCommitSHA string        `json:"merge_commit_sha"`
	Head           pullBranchRef `json:"head"`
	Base           pullBranchRef `json:"base"`
}

type pullBranchRef struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

const apiVersion = "2022-11-28"

func (c *Client) CreateIssue(ctx context.Context, repo string, req codehost.CreateIssueRequest) (*codehost.Issue, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"title": req.Title,
		"body":  req.Description,
	}
	if len(req.Labels) > 0 {
		payload["labels"] = append([]string(nil), req.Labels...)
	}

	var issue idResponse
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/issues", url.PathEscape(owner), url.PathEscape(name)), payload, &issue); err != nil {
		return nil, err
	}
	return &codehost.Issue{IID: issue.Number, WebURL: issue.HTMLURL}, nil
}

func (c *Client) CreateMergeRequest(ctx context.Context, repo string, req codehost.CreateMergeRequestRequest) (*codehost.MergeRequest, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"title": req.Title,
		"body":  req.Description,
		"head":  req.SourceBranch,
		"base":  req.TargetBranch,
	}

	var pr idResponse
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/pulls", url.PathEscape(owner), url.PathEscape(name)), payload, &pr); err != nil {
		return nil, err
	}
	if len(req.Labels) > 0 {
		if _, err := c.SetMergeRequestLabels(ctx, repo, pr.Number, codehost.SetLabelsRequest{Labels: req.Labels}); err != nil {
			return nil, err
		}
	}
	return &codehost.MergeRequest{IID: pr.Number, WebURL: pr.HTMLURL}, nil
}

func (c *Client) CreateIssueNote(ctx context.Context, repo string, issueIID int, req codehost.CreateNoteRequest) (*codehost.Note, error) {
	return c.createIssueComment(ctx, repo, issueIID, req)
}

func (c *Client) CreateMergeRequestNote(ctx context.Context, repo string, mrIID int, req codehost.CreateNoteRequest) (*codehost.Note, error) {
	return c.createIssueComment(ctx, repo, mrIID, req)
}

func (c *Client) SetIssueLabels(ctx context.Context, repo string, issueIID int, req codehost.SetLabelsRequest) (*codehost.Issue, error) {
	if err := c.replaceIssueLabels(ctx, repo, issueIID, http.MethodPut, req.Labels); err != nil {
		return nil, err
	}
	return &codehost.Issue{IID: issueIID}, nil
}

func (c *Client) SetMergeRequestLabels(ctx context.Context, repo string, mrIID int, req codehost.SetLabelsRequest) (*codehost.MergeRequest, error) {
	if err := c.replaceIssueLabels(ctx, repo, mrIID, http.MethodPut, req.Labels); err != nil {
		return nil, err
	}
	return &codehost.MergeRequest{IID: mrIID}, nil
}

func (c *Client) AddMergeRequestLabels(ctx context.Context, repo string, mrIID int, req codehost.SetLabelsRequest) (*codehost.MergeRequest, error) {
	if err := c.replaceIssueLabels(ctx, repo, mrIID, http.MethodPost, req.Labels); err != nil {
		return nil, err
	}
	return &codehost.MergeRequest{IID: mrIID}, nil
}

// GetMergeRequest 读取 PR 详情(分支与 HEAD;pulls API 的 head/base 是对象,
// 取 ref/sha);与 gitea/gitlab 客户端同名方法形态一致。
func (c *Client) GetMergeRequest(ctx context.Context, repo string, mrIID int) (*codehost.MergeRequest, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	var pr pullResponse
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/pulls/%d", url.PathEscape(owner), url.PathEscape(name), mrIID), nil, &pr); err != nil {
		return nil, mapMergeStatusError(err)
	}
	return &codehost.MergeRequest{
		IID:            pr.Number,
		WebURL:         pr.HTMLURL,
		SourceBranch:   strings.TrimSpace(pr.Head.Ref),
		TargetBranch:   strings.TrimSpace(pr.Base.Ref),
		HeadSHA:        strings.TrimSpace(pr.Head.SHA),
		Merged:         pr.Merged,
		MergeCommitSHA: strings.TrimSpace(pr.MergeCommitSHA),
	}, nil
}

// mergeResponse 是 PUT pulls/{n}/merge 的响应体:merged 与最终 commit sha。
type mergeResponse struct {
	SHA     string `json:"sha"`
	Merged  bool   `json:"merged"`
	Message string `json:"message"`
}

// MergePullRequest 执行 PR 合并,sha 参数即头部断言:头部漂移平台返回 409,
// 映射为 ErrMergeHeadMismatch;保护规则等明确拒绝(403/405/422)映射为
// ErrMergeRejected 并携带平台原因——门禁层据此回落人工,不绕平台规则。
func (c *Client) MergePullRequest(ctx context.Context, repo string, mrIID int, req codehost.MergePullRequestRequest) (*codehost.MergePullRequestResult, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"sha": req.ExpectedHeadSHA,
	}
	if strings.TrimSpace(req.MergeMethod) != "" {
		payload["merge_method"] = strings.TrimSpace(req.MergeMethod)
	}
	if strings.TrimSpace(req.CommitTitle) != "" {
		payload["commit_title"] = req.CommitTitle
	}
	if strings.TrimSpace(req.CommitMessage) != "" {
		payload["commit_message"] = req.CommitMessage
	}

	var merged mergeResponse
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", url.PathEscape(owner), url.PathEscape(name), mrIID)
	if err := c.doJSON(ctx, http.MethodPut, path, payload, &merged); err != nil {
		return nil, mapMergeStatusError(err)
	}
	return &codehost.MergePullRequestResult{Merged: merged.Merged, MergedCommitSHA: strings.TrimSpace(merged.SHA)}, nil
}

// mapMergeStatusError 把确定性拒绝归到哨兵错误上:409=头部断言失配
// (ErrMergeHeadMismatch),其余 4xx 家族=平台明确拒绝、不可重试
// (ErrMergeRejected,含 404 的 PR 已删/仓库转移与 401/403 的凭据问题);
// 5xx 与网络类保持 Ambiguous 原样。429/408 以及带有 GitHub 限流信号的 403
// 属于可重试的 4xx(TransientRejection),必须先于 4xx 归类原样返回,不得改写。
// 读与合两个入口共用同一分类,门禁据此区分回落与重评。
func mapMergeStatusError(err error) error {
	if codehost.IsTransientRejection(err) {
		return err
	}
	var status *StatusError
	if !errors.As(err, &status) {
		return err
	}
	switch {
	case status.StatusCode == http.StatusConflict:
		return fmt.Errorf("%w: %s", codehost.ErrMergeHeadMismatch, status.Message)
	case status.StatusCode >= 400 && status.StatusCode < 500:
		return fmt.Errorf("%w: %s", codehost.ErrMergeRejected, status.Message)
	}
	return err
}

func splitRepo(repo string) (string, string, error) {
	parts := strings.Split(strings.Trim(repo, "/"), "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("github repo must be owner/repo, got %q", repo)
	}
	return parts[0], parts[1], nil
}

func (c *Client) createIssueComment(ctx context.Context, repo string, index int, req codehost.CreateNoteRequest) (*codehost.Note, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	// GitHub 把 PR 评论也视作 issue 评论,统一走 issues comments API。
	var note noteResponse
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/issues/%d/comments", url.PathEscape(owner), url.PathEscape(name), index), map[string]any{
		"body": req.Body,
	}, &note); err != nil {
		return nil, err
	}
	return &codehost.Note{ID: note.ID, Body: note.Body}, nil
}

func (c *Client) replaceIssueLabels(ctx context.Context, repo string, index int, method string, labels []string) error {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return err
	}
	var updated []struct {
		Name string `json:"name"`
	}
	return c.doJSON(ctx, method, fmt.Sprintf("/repos/%s/%s/issues/%d/labels", url.PathEscape(owner), url.PathEscape(name), index), map[string]any{
		"labels": append([]string(nil), labels...),
	}, &updated)
}

func (c *Client) doJSON(ctx context.Context, method, path string, in any, out any) error {
	var body io.Reader
	if in != nil {
		buf := &bytes.Buffer{}
		if err := json.NewEncoder(buf).Encode(in); err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		body = buf
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return codehost.NewAmbiguousError(fmt.Errorf("perform request: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		err := &StatusError{
			StatusCode:         resp.StatusCode,
			Message:            fmt.Sprintf("github %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(data))),
			RetryAfter:         strings.TrimSpace(resp.Header.Get("Retry-After")),
			RateLimitRemaining: strings.TrimSpace(resp.Header.Get("X-RateLimit-Remaining")),
			RateLimitReset:     strings.TrimSpace(resp.Header.Get("X-RateLimit-Reset")),
		}
		if resp.StatusCode >= http.StatusInternalServerError {
			return codehost.NewAmbiguousError(err)
		}
		if isTransientHTTPRejection(err) {
			return codehost.NewTransientRejection(err)
		}
		return err
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		err := fmt.Errorf("decode response: %w", err)
		if isMutatingMethod(method) {
			return codehost.NewAmbiguousError(err)
		}
		return err
	}
	return nil
}

func isTransientHTTPRejection(err *StatusError) bool {
	if err.StatusCode == http.StatusTooManyRequests || err.StatusCode == http.StatusRequestTimeout {
		return true
	}
	if err.StatusCode != http.StatusForbidden {
		return false
	}
	if err.RetryAfter != "" || err.RateLimitRemaining == "0" {
		return true
	}
	message := strings.ToLower(err.Message)
	return strings.Contains(message, "rate limit")
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// StatusError 携带 HTTP 状态码,供写回凭据降级(401/403)等调用方识别。
type StatusError struct {
	StatusCode         int
	Message            string
	RetryAfter         string
	RateLimitRemaining string
	RateLimitReset     string
}

func (err *StatusError) Error() string { return err.Message }

// StatusCode 提取错误链中的 HTTP 状态码;无状态信息返回 0。
func StatusCode(err error) int {
	var statusErr *StatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode
	}
	return 0
}

// listAll 翻页拉取列表直到短页;GitHub 分页参数为 per_page(上限 100)+ page。
// maxPages 是防御性上限,避免异常服务器导致死循环。basePath 可已带 query。
func (c *Client) listAll(ctx context.Context, basePath string, perPage int, maxPages int, page func(raw json.RawMessage) (bool, error)) error {
	separator := "?"
	if strings.Contains(basePath, "?") {
		separator = "&"
	}
	for pageNum := 1; pageNum <= maxPages; pageNum++ {
		query := url.Values{"per_page": {strconv.Itoa(perPage)}, "page": {strconv.Itoa(pageNum)}}
		var raw json.RawMessage
		if err := c.doJSON(ctx, http.MethodGet, basePath+separator+query.Encode(), nil, &raw); err != nil {
			return err
		}
		stop, err := page(raw)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return fmt.Errorf("decode list response: %w", err)
		}
		if len(items) < perPage {
			return nil
		}
	}
	return nil
}
