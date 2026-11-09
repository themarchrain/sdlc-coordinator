package gitea

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
		baseURL:    strings.TrimRight(baseURL, "/"),
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

type issueResponse struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
}

type noteResponse struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
}

type labelResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

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
		labelIDs, err := c.resolveLabelIDs(ctx, repo, owner, name, req.Labels)
		if err != nil {
			return nil, err
		}
		payload["labels"] = labelIDs
	}

	var issue issueResponse
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/v1/repos/%s/%s/issues", url.PathEscape(owner), url.PathEscape(name)), payload, &issue); err != nil {
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

	var pr issueResponse
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/v1/repos/%s/%s/pulls", url.PathEscape(owner), url.PathEscape(name)), payload, &pr); err != nil {
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
	issue, err := c.setIssueLabels(ctx, repo, issueIID, req.Labels)
	if err != nil {
		return nil, err
	}
	return &codehost.Issue{IID: issue.Number, WebURL: issue.HTMLURL}, nil
}

func (c *Client) SetMergeRequestLabels(ctx context.Context, repo string, mrIID int, req codehost.SetLabelsRequest) (*codehost.MergeRequest, error) {
	issue, err := c.setIssueLabels(ctx, repo, mrIID, req.Labels)
	if err != nil {
		return nil, err
	}
	return &codehost.MergeRequest{IID: issue.Number, WebURL: issue.HTMLURL}, nil
}

func (c *Client) AddMergeRequestLabels(ctx context.Context, repo string, mrIID int, req codehost.SetLabelsRequest) (*codehost.MergeRequest, error) {
	if err := c.addIssueLabels(ctx, repo, mrIID, req.Labels); err != nil {
		return nil, err
	}
	return &codehost.MergeRequest{IID: mrIID}, nil
}

type pullResponse struct {
	Number         int            `json:"number"`
	HTMLURL        string         `json:"html_url"`
	Head           pullBranchInfo `json:"head"`
	Base           pullBranchInfo `json:"base"`
	Merged         bool           `json:"merged"`
	MergeCommitSHA string         `json:"merge_commit_sha"`
}

type pullBranchInfo struct {
	Name string `json:"name"`
	Ref  string `json:"ref"`
	SHA  string `json:"sha"`
}

// branch 取分支名:Gitea ≥1.14 的 head/base 带 ref;旧版只有 name。ref 优先,
// name 兜底,两者皆空返回空串(调用方按缺失处理)。
func (info pullBranchInfo) branch() string {
	if ref := strings.TrimSpace(info.Ref); ref != "" {
		return ref
	}
	return strings.TrimSpace(info.Name)
}

// getPull 读取 PR 原始响应,不做门禁哨兵分类。GetMergeRequest 在其上做映射;
// 合并后的真相回读直接消费原始错误,保证「请求已发出、结果未知」不会被
// 升级成确定性拒绝。
func (c *Client) getPull(ctx context.Context, repo string, mrIID int) (*pullResponse, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	var pr pullResponse
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d", url.PathEscape(owner), url.PathEscape(name), mrIID), nil, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// mergeRequest 把平台 PR 原始响应映射为 codehost.MergeRequest;Merged/
// MergeCommitSHA 供自动合并门禁做拒绝/歧义后的真相恢复。
func (pr *pullResponse) mergeRequest() *codehost.MergeRequest {
	return &codehost.MergeRequest{
		IID:            pr.Number,
		WebURL:         pr.HTMLURL,
		SourceBranch:   pr.Head.branch(),
		TargetBranch:   pr.Base.branch(),
		HeadSHA:        strings.TrimSpace(pr.Head.SHA),
		Merged:         pr.Merged,
		MergeCommitSHA: strings.TrimSpace(pr.MergeCommitSHA),
	}
}

// GetMergeRequest 读取 PR 详情(分支与 HEAD);与 github/gitlab 客户端同名方法
// 形态一致。
func (c *Client) GetMergeRequest(ctx context.Context, repo string, mrIID int) (*codehost.MergeRequest, error) {
	pr, err := c.getPull(ctx, repo, mrIID)
	if err != nil {
		return nil, mapMergeStatusError(err)
	}
	return pr.mergeRequest(), nil
}

// MergePullRequest 执行 PR 合并:ExpectedHeadSHA 走 Gitea 原生 head_commit_id
// 断言,头部漂移由平台拒合。Gitea 合并成功响应是空 200,不携带结果,必须
// 回读 PR:merged=true 返回合并成功并透出 merge_commit_sha;merged=false
// 返回未合并结果(err=nil),由门禁回落人工;回读失败属于结果不确定,包装为
// Ambiguous 交给门禁下一轮回读真实状态。不删源分支、不设置 force_merge/
// merge_when_checks_succeed,不绕平台保护规则。
func (c *Client) MergePullRequest(ctx context.Context, repo string, mrIID int, req codehost.MergePullRequestRequest) (*codehost.MergePullRequestResult, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	method := strings.TrimSpace(req.MergeMethod)
	if method == "" {
		method = "merge"
	}
	payload := map[string]any{
		"do":                        method,
		"head_commit_id":            req.ExpectedHeadSHA,
		"delete_branch_after_merge": false,
	}
	if strings.TrimSpace(req.CommitTitle) != "" {
		payload["merge_title_field"] = req.CommitTitle
	}
	if strings.TrimSpace(req.CommitMessage) != "" {
		payload["merge_message_field"] = req.CommitMessage
	}
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d/merge", url.PathEscape(owner), url.PathEscape(name), mrIID), payload, nil); err != nil {
		return nil, mapMergeStatusError(err)
	}
	pr, err := c.getPull(ctx, repo, mrIID)
	if err != nil {
		// 合并请求已发出(空 200),此刻真相未知:回读失败属于结果不确定,
		// 保持 Ambiguous 由门禁下一轮回读真实状态;不得复用读入口的哨兵
		// 映射,否则已生效的合并会被记成确定性拒绝。
		return nil, codehost.NewAmbiguousError(fmt.Errorf("read back merged pull request %s#%d: %w", repo, mrIID, err))
	}
	return &codehost.MergePullRequestResult{
		Merged:          pr.Merged,
		MergedCommitSHA: strings.TrimSpace(pr.MergeCommitSHA),
	}, nil
}

// mapMergeStatusError 把确定性拒绝归到哨兵错误上:409=头部断言失配
// (ErrMergeHeadMismatch),其余 4xx 家族=平台明确拒绝、不可重试
// (ErrMergeRejected,含 404 的 PR 已删/仓库转移与 401/403 的凭据问题);
// 5xx 与网络类保持 Ambiguous 原样。429/408 属于可重试的 4xx
// (TransientRejection),必须先于 4xx 归类原样返回,不得改写。读与合两个
// 入口共用同一分类,门禁据此区分回落与重评;Issue/Labels 等非门禁写回
// 不经过此映射,保持原有 StatusError 语义。
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
	case status.StatusCode >= http.StatusBadRequest && status.StatusCode < http.StatusInternalServerError:
		return fmt.Errorf("%w: %s", codehost.ErrMergeRejected, status.Message)
	}
	return err
}

func splitRepo(repo string) (string, string, error) {
	parts := strings.Split(strings.Trim(repo, "/"), "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("gitea repo must be owner/repo, got %q", repo)
	}
	return parts[0], parts[1], nil
}

func (c *Client) createIssueComment(ctx context.Context, repo string, index int, req codehost.CreateNoteRequest) (*codehost.Note, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}

	var note noteResponse
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/v1/repos/%s/%s/issues/%d/comments", url.PathEscape(owner), url.PathEscape(name), index), map[string]any{
		"body": req.Body,
	}, &note); err != nil {
		return nil, err
	}
	return &codehost.Note{ID: note.ID, Body: note.Body}, nil
}

func (c *Client) setIssueLabels(ctx context.Context, repo string, index int, labels []string) (*issueResponse, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	labelIDs, err := c.resolveLabelIDs(ctx, repo, owner, name, labels)
	if err != nil {
		return nil, err
	}

	var updatedLabels []labelResponse
	if err := c.doJSON(ctx, http.MethodPut, fmt.Sprintf("/api/v1/repos/%s/%s/issues/%d/labels", url.PathEscape(owner), url.PathEscape(name), index), map[string]any{
		"labels": labelIDs,
	}, &updatedLabels); err != nil {
		return nil, err
	}
	return &issueResponse{Number: index}, nil
}

func (c *Client) addIssueLabels(ctx context.Context, repo string, index int, labels []string) error {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return err
	}
	labelIDs, err := c.resolveLabelIDs(ctx, repo, owner, name, labels)
	if err != nil {
		return err
	}

	var updatedLabels []labelResponse
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/v1/repos/%s/%s/issues/%d/labels", url.PathEscape(owner), url.PathEscape(name), index), map[string]any{
		"labels": labelIDs,
	}, &updatedLabels); err != nil {
		return err
	}
	return nil
}

// giteaPageSize 是列表接口的取页大小;服务器会把更大的 limit 截到自身上限。
const giteaPageSize = 50

func (c *Client) resolveLabelIDs(ctx context.Context, repo, owner, name string, labels []string) ([]int, error) {
	if len(labels) == 0 {
		return nil, nil
	}

	byName := make(map[string]int, len(labels))
	basePath := fmt.Sprintf("/api/v1/repos/%s/%s/labels", url.PathEscape(owner), url.PathEscape(name))
	for page := 1; ; page++ {
		query := url.Values{"page": {strconv.Itoa(page)}, "limit": {strconv.Itoa(giteaPageSize)}}
		var remoteLabels []labelResponse
		if err := c.doJSON(ctx, http.MethodGet, basePath+"?"+query.Encode(), nil, &remoteLabels); err != nil {
			return nil, err
		}
		for _, label := range remoteLabels {
			byName[label.Name] = label.ID
		}
		if len(remoteLabels) < giteaPageSize {
			break
		}
	}

	ids := make([]int, 0, len(labels))
	for _, label := range labels {
		id, ok := byName[label]
		if !ok {
			return nil, fmt.Errorf("gitea label %q not found in %s", label, repo)
		}
		ids = append(ids, id)
	}
	return ids, nil
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
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return codehost.NewAmbiguousError(fmt.Errorf("perform request: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		err := &StatusError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("gitea %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(data)))}
		if resp.StatusCode >= http.StatusInternalServerError {
			return codehost.NewAmbiguousError(err)
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusRequestTimeout {
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
	StatusCode int
	Message    string
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
