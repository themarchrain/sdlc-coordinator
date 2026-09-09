package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"sdlc-coordinator/internal/operationmarker"
)

// GitHubNormalizer 把 GitHub webhook 载荷(pull_request / issues / issue_comment)
// 规范化为内部事件,守卫语义与 GitLab/Gitea 对齐:developer-agent 标签门禁
// (labeled 为独立交付)、交付分支校验、同仓 PR、协调器操作评论过滤、
// rework 发起人预授权。
type GitHubNormalizer struct {
	reworkAllowedUserIDs map[int]struct{}
}

type GitHubNormalizerOption func(*GitHubNormalizer)

// WithGitHubReworkAllowedUserIDs 配置平台级 rework 白名单(平台 user id),
// 语义与 GitLab 侧 WithCodeHostReworkAllowedUserIDs 一致。
func WithGitHubReworkAllowedUserIDs(ids []int) GitHubNormalizerOption {
	return func(n *GitHubNormalizer) {
		if n.reworkAllowedUserIDs == nil {
			n.reworkAllowedUserIDs = map[int]struct{}{}
		}
		for _, id := range ids {
			if id > 0 {
				n.reworkAllowedUserIDs[id] = struct{}{}
			}
		}
	}
}

func NewGitHubNormalizer(options ...GitHubNormalizerOption) GitHubNormalizer {
	normalizer := GitHubNormalizer{reworkAllowedUserIDs: map[int]struct{}{}}
	for _, option := range options {
		option(&normalizer)
	}
	return normalizer
}

func (GitHubNormalizer) Source() string { return "github" }

func (n GitHubNormalizer) Receipt(raw []byte, metadata WebhookMetadata) WebhookReceipt {
	receipt := WebhookReceipt{EventType: strings.TrimSpace(metadata.EventType)}
	if deliveryID := strings.TrimSpace(metadata.DeliveryID); deliveryID != "" {
		receipt.DedupeKey = fmt.Sprintf("github:webhook:delivery:%s", deliveryID)
		return receipt
	}

	var payload githubWebhookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		receipt.DedupeKey = fmt.Sprintf("github:webhook:%s:%s", receipt.EventType, shortRawSHA256(raw))
		return receipt
	}
	var objectNumber int
	switch {
	case payload.PullRequest != nil && payload.PullRequest.Number > 0:
		objectNumber = payload.PullRequest.Number
	case payload.Issue != nil && payload.Issue.Number > 0:
		objectNumber = payload.Issue.Number
	case payload.Number > 0:
		objectNumber = payload.Number
	}
	externalID := fmt.Sprintf("%d:%d", payload.RepositoryID(), objectNumber)
	receipt.ExternalID = externalID
	receipt.EventAction = strings.TrimSpace(payload.Action)
	receipt.DedupeKey = fmt.Sprintf("github:webhook:%s:%d:%d:%s:%s",
		receipt.EventType, payload.RepositoryID(), objectNumber, receipt.EventAction, shortRawSHA256(raw))
	return receipt
}

type githubWebhookPayload struct {
	Action string `json:"action"`
	Number int    `json:"number"`
	// synchronize 交付顶层的 before/after:上一版与当前版源分支头,before 是
	// 增量审查的起点(base.sha 是目标分支 tip,语义不同不可混用)。
	Before      string             `json:"before"`
	Label       *githubLabel       `json:"label"`
	Issue       *githubIssue       `json:"issue"`
	PullRequest *githubPullRequest `json:"pull_request"`
	Comment     *githubComment     `json:"comment"`
	Repository  *githubRepository  `json:"repository"`
	Sender      *githubUser        `json:"sender"`
}

func (payload *githubWebhookPayload) RepositoryID() int64 {
	if payload.Repository != nil {
		return payload.Repository.ID
	}
	return 0
}

type githubUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type githubLabel struct {
	Name string `json:"name"`
}

type githubIssue struct {
	ID          int64           `json:"id"`
	Number      int             `json:"number"`
	Title       string          `json:"title"`
	Body        string          `json:"body"`
	State       string          `json:"state"`
	HTMLURL     string          `json:"html_url"`
	UpdatedAt   string          `json:"updated_at"`
	User        *githubUser     `json:"user"`
	Labels      []githubLabel   `json:"labels"`
	Assignees   []*githubUser   `json:"assignees"`
	PullRequest *map[string]any `json:"pull_request"`
}

func (issue *githubIssue) PullRequestURL() string {
	if issue == nil || issue.PullRequest == nil {
		return ""
	}
	for _, key := range []string{"html_url", "url"} {
		if value, ok := (*issue.PullRequest)[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

type githubPullRequest struct {
	ID        int64           `json:"id"`
	Number    int             `json:"number"`
	Title     string          `json:"title"`
	Body      string          `json:"body"`
	State     string          `json:"state"`
	Merged    bool            `json:"merged"`
	HTMLURL   string          `json:"html_url"`
	Head      githubBranchRef `json:"head"`
	Base      githubBranchRef `json:"base"`
	UpdatedAt string          `json:"updated_at"`
	User      *githubUser     `json:"user"`
	Assignees []*githubUser   `json:"assignees"`
}

type githubBranchRef struct {
	Ref  string            `json:"ref"`
	SHA  string            `json:"sha"`
	Repo *githubRepository `json:"repo"`
}

type githubRepository struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	CloneURL      string `json:"clone_url"`
	HTMLURL       string `json:"html_url"`
	DefaultBranch string `json:"default_branch"`
}

type githubComment struct {
	ID      int64       `json:"id"`
	Body    string      `json:"body"`
	HTMLURL string      `json:"html_url"`
	User    *githubUser `json:"user"`
}

func (n GitHubNormalizer) Normalize(raw []byte) (*NormalizationResult, error) {
	var payload githubWebhookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode github webhook payload: %w", err)
	}

	if payload.Comment != nil && payload.Issue != nil {
		if payload.Issue.PullRequest != nil {
			return n.normalizeGitHubPRComment(payload)
		}
		return n.normalizeGitHubIssueComment(payload)
	}
	if payload.PullRequest != nil {
		return n.normalizeGitHubPullRequest(payload)
	}
	if payload.Issue != nil {
		return n.normalizeGitHubIssue(payload)
	}
	return ignoredNormalization("unsupported github webhook payload")
}

func (n GitHubNormalizer) normalizeGitHubPullRequest(payload githubWebhookPayload) (*NormalizationResult, error) {
	pr := payload.PullRequest
	repo := payload.Repository
	switch strings.ToLower(strings.TrimSpace(payload.Action)) {
	case "opened", "synchronize", "reopened":
	case "closed":
		return n.normalizeGitHubPullRequestLifecycle(payload)
	default:
		return ignoredNormalization("unsupported pull_request action")
	}
	if !isOpenGitHubState(pr.State) {
		return ignoredNormalization("pull request is not opened")
	}
	if repo == nil || repo.ID <= 0 {
		return nil, errors.New("github repository id is required")
	}
	if pr.Number <= 0 {
		return nil, errors.New("github pull request number is required")
	}
	if strings.EqualFold(strings.TrimSpace(pr.Head.Ref), strings.TrimSpace(pr.Base.Ref)) ||
		strings.EqualFold(strings.TrimSpace(pr.Head.Ref), strings.TrimSpace(repo.DefaultBranch)) {
		return ignoredNormalization("pull request source branch is not a delivery branch")
	}
	if pr.Base.Repo != nil && strings.TrimSpace(pr.Base.Repo.FullName) != "" &&
		!strings.EqualFold(pr.Base.Repo.FullName, repo.FullName) {
		return ignoredNormalization("pull request is not a same-repository pull request")
	}

	projectID := strconv.FormatInt(repo.ID, 10)
	if err := requireGitHubFields(map[string]string{
		"repository.full_name":  repo.FullName,
		"repository.clone_url":  repo.CloneURL,
		"pull_request.html_url": pr.HTMLURL,
		"pull_request.head.sha": pr.Head.SHA,
	}); err != nil {
		return nil, err
	}

	// 平台动作词归一到跨平台词表(与 GitLab 对齐):下游按 update+base_sha 判
	// 增量审查。synchronize 的增量起点取顶层 before;缺 before 时 base_sha
	// 为空,任务工厂自动回落完整审查(优雅降级)。
	eventAction := strings.ToLower(strings.TrimSpace(payload.Action))
	incrementBase := ""
	metadata := map[string]any{
		"author_name":     githubDisplayName(pr.User),
		"author_username": githubUserName(pr.User),
		"author_email":    strings.TrimSpace(githubUserEmail(pr.User)),
		"assignee_ids":    githubUserIDs(pr.Assignees),
	}
	if eventAction == "synchronize" {
		eventAction = "update"
		incrementBase = strings.TrimSpace(payload.Before)
		metadata["raw_action"] = strings.TrimSpace(payload.Action)
	}

	return &NormalizationResult{Event: &NormalizedEvent{
		Source:        "github",
		Kind:          EventKindChangeRequestUpdated,
		EventType:     "pull_request",
		EventAction:   eventAction,
		ProjectID:     projectID,
		ProjectPath:   repo.FullName,
		RepositoryURL: repo.CloneURL,
		ObjectIID:     pr.Number,
		ObjectURL:     pr.HTMLURL,
		Title:         pr.Title,
		Description:   pr.Body,
		SourceBranch:  pr.Head.Ref,
		TargetBranch:  pr.Base.Ref,
		HeadSHA:       strings.TrimSpace(pr.Head.SHA),
		BaseSHA:       incrementBase,
		Version:       strings.TrimSpace(pr.Head.SHA),
		DedupeKey:     fmt.Sprintf("github:mr-review:%s:%d:%s", projectID, pr.Number, strings.TrimSpace(pr.Head.SHA)),
		Metadata:      metadata,
	}}, nil
}

func (n GitHubNormalizer) normalizeGitHubPullRequestLifecycle(payload githubWebhookPayload) (*NormalizationResult, error) {
	pr := payload.PullRequest
	repo := payload.Repository
	if repo == nil || repo.ID <= 0 {
		return nil, errors.New("github repository id is required")
	}
	state := "closed"
	if pr.Merged {
		state = "merged"
	}
	// GitHub 合并与关闭共用动作词 closed;归一到下游 switch 认的
	// merge/close(与 GitLab 词表一致),原始动作词留档 metadata。
	eventAction := "close"
	if pr.Merged {
		eventAction = "merge"
	}
	projectID := strconv.FormatInt(repo.ID, 10)
	return &NormalizationResult{Event: &NormalizedEvent{
		Source:        "github",
		Kind:          EventKindMergeRequestLifecycleChanged,
		EventType:     "pull_request",
		EventAction:   eventAction,
		ProjectID:     projectID,
		ProjectPath:   repo.FullName,
		RepositoryURL: repo.CloneURL,
		ObjectIID:     pr.Number,
		ObjectURL:     pr.HTMLURL,
		Title:         pr.Title,
		SourceBranch:  pr.Head.Ref,
		TargetBranch:  pr.Base.Ref,
		HeadSHA:       strings.TrimSpace(pr.Head.SHA),
		// 与 rework 事件共用键:lifecycle 终结按 dedupe key 定位 rework 任务。
		DedupeKey: fmt.Sprintf("github:mr-rework:%s:%d", projectID, pr.Number),
		Metadata: map[string]any{
			"lifecycle_state": state,
			"raw_action":      strings.TrimSpace(payload.Action),
			"assignee_ids":    githubUserIDs(pr.Assignees),
		},
	}}, nil
}

func (n GitHubNormalizer) normalizeGitHubIssue(payload githubWebhookPayload) (*NormalizationResult, error) {
	issue := payload.Issue
	repo := payload.Repository
	if !isOpenGitHubState(issue.State) {
		return ignoredNormalization("issue is not opened")
	}
	// labeled 是"先建后打标签"流程的事实启动信号,守卫读顶层 label 精确表达;
	// opened 则要求载荷里已带 developer-agent。
	switch strings.ToLower(strings.TrimSpace(payload.Action)) {
	case "opened":
	case "labeled":
		if payload.Label == nil || !strings.EqualFold(strings.TrimSpace(payload.Label.Name), "developer-agent") {
			return ignoredNormalization("labeled with other label")
		}
	default:
		return ignoredNormalization("unsupported issue action")
	}
	if !hasGitHubLabel(issue.Labels, "developer-agent") {
		return ignoredNormalization("issue does not have developer-agent label")
	}
	if repo == nil || repo.ID <= 0 {
		return nil, errors.New("github repository id is required")
	}
	if issue.Number <= 0 {
		return nil, errors.New("github issue number is required")
	}

	projectID := strconv.FormatInt(repo.ID, 10)
	if err := requireGitHubFields(map[string]string{
		"repository.full_name": repo.FullName,
		"repository.clone_url": repo.CloneURL,
		"issue.html_url":       issue.HTMLURL,
	}); err != nil {
		return nil, err
	}

	return &NormalizationResult{Event: &NormalizedEvent{
		Source:        "github",
		Kind:          EventKindWorkItemUpdated,
		EventType:     "issues",
		EventAction:   payload.Action,
		ProjectID:     projectID,
		ProjectPath:   repo.FullName,
		RepositoryURL: repo.CloneURL,
		ObjectIID:     issue.Number,
		ObjectURL:     issue.HTMLURL,
		Title:         issue.Title,
		Description:   issue.Body,
		TargetBranch:  repo.DefaultBranch,
		Version:       strings.TrimSpace(issue.UpdatedAt),
		DedupeKey:     fmt.Sprintf("github:issue-develop:%s:%d", projectID, issue.Number),
		Metadata: map[string]any{
			"assignee_ids": githubUserIDs(issue.Assignees),
			"assignees":    githubAssigneesMetadata(issue.Assignees),
		},
	}}, nil
}

func (n GitHubNormalizer) normalizeGitHubIssueComment(payload githubWebhookPayload) (*NormalizationResult, error) {
	issue := payload.Issue
	comment := payload.Comment
	repo := payload.Repository
	if !strings.EqualFold(strings.TrimSpace(payload.Action), "created") {
		return ignoredNormalization("comment action is not created")
	}
	if !isOpenGitHubState(issue.State) {
		return ignoredNormalization("issue is not opened")
	}
	if !hasGitHubLabel(issue.Labels, "developer-agent") {
		return ignoredNormalization("issue does not have developer-agent label")
	}
	if operationmarker.Contains(comment.Body) {
		return ignoredNormalization("coordinator operation comment")
	}
	// resume 评论的请求者白名单:作者 ∪ 指派人 ∪ 平台白名单;无指派人降级为仅作者。
	if comment.User == nil || comment.User.ID <= 0 {
		return ignoredNormalization("comment author is required")
	}
	issueAuthorID := 0
	if issue.User != nil {
		issueAuthorID = int(issue.User.ID)
	}
	allowed := resumeRequesterAllowed(int(comment.User.ID), issueAuthorID, githubUserIDs(issue.Assignees), n.reworkAllowedUserIDs)
	if !allowed {
		return ignoredNormalization("resume requester is not authorized")
	}
	resumeContext, ok := parseGitLabComment(comment.Body)
	if !ok {
		return ignoredNormalization("issue comment context is required")
	}
	if strings.TrimSpace(resumeContext) == "" {
		return ignoredNormalization("resume comment context is required")
	}
	if repo == nil || repo.ID <= 0 {
		return nil, errors.New("github repository id is required")
	}
	if issue.Number <= 0 || comment.ID <= 0 {
		return nil, errors.New("github issue number and comment id are required")
	}

	projectID := strconv.FormatInt(repo.ID, 10)
	if err := requireGitHubFields(map[string]string{
		"repository.full_name": repo.FullName,
		"repository.clone_url": repo.CloneURL,
		"issue.html_url":       issue.HTMLURL,
	}); err != nil {
		return nil, err
	}

	return &NormalizationResult{Event: &NormalizedEvent{
		Source:        "github",
		Kind:          EventKindDeveloperResumeRequested,
		EventType:     "issue_comment",
		EventAction:   payload.Action,
		ProjectID:     projectID,
		ProjectPath:   repo.FullName,
		RepositoryURL: repo.CloneURL,
		ObjectIID:     issue.Number,
		ObjectURL:     issue.HTMLURL,
		Title:         issue.Title,
		Description:   issue.Body,
		TargetBranch:  repo.DefaultBranch,
		Version:       strconv.FormatInt(comment.ID, 10),
		DedupeKey:     fmt.Sprintf("github:issue-develop:%s:%d", projectID, issue.Number),
		Metadata: map[string]any{
			"assignee_ids":         githubUserIDs(issue.Assignees),
			"comment_id":           int(comment.ID),
			"comment_url":          strings.TrimSpace(comment.HTMLURL),
			"resume_context":       resumeContext,
			"requester_id":         int(comment.User.ID),
			"requester_authorized": allowed,
		},
	}}, nil
}

func (n GitHubNormalizer) normalizeGitHubPRComment(payload githubWebhookPayload) (*NormalizationResult, error) {
	issue := payload.Issue
	comment := payload.Comment
	repo := payload.Repository
	if !strings.EqualFold(strings.TrimSpace(payload.Action), "created") {
		return ignoredNormalization("comment action is not created")
	}
	// PR 的 issue 与 PR 共用状态:已关闭/已合并的 PR 不再接受 rework 请求。
	if !isOpenGitHubState(issue.State) {
		return ignoredNormalization("merge request is not opened")
	}
	if operationmarker.Contains(comment.Body) {
		return ignoredNormalization("coordinator operation comment")
	}
	// 下游 rework 授权依赖 requester 身份;缺作者的评论既无法授权也不能 panic。
	if comment.User == nil || comment.User.ID <= 0 {
		return ignoredNormalization("comment author is required")
	}
	resumeContext, ok := parseGitLabComment(comment.Body)
	if !ok {
		return ignoredNormalization("merge request comment context is required")
	}
	if strings.TrimSpace(resumeContext) == "" {
		return ignoredNormalization("resume comment context is required")
	}
	// GitHub 的 issue 与 PR 同号:issue.number 即 PR 号。
	prNumber := issue.Number
	if prNumber <= 0 && payload.Number > 0 {
		prNumber = payload.Number
	}
	if repo == nil || repo.ID <= 0 {
		return nil, errors.New("github repository id is required")
	}
	if prNumber <= 0 || comment.ID <= 0 {
		return nil, errors.New("github pull request number and comment id are required")
	}
	objectURL := strings.TrimSpace(issue.PullRequestURL())
	if objectURL == "" {
		objectURL = strings.TrimSpace(issue.HTMLURL)
	}

	projectID := strconv.FormatInt(repo.ID, 10)
	if err := requireGitHubFields(map[string]string{
		"repository.full_name": repo.FullName,
		"repository.clone_url": repo.CloneURL,
	}); err != nil {
		return nil, err
	}

	return &NormalizationResult{Event: &NormalizedEvent{
		Source:        "github",
		Kind:          EventKindMergeRequestReworkRequested,
		EventType:     "issue_comment",
		EventAction:   payload.Action,
		ProjectID:     projectID,
		ProjectPath:   repo.FullName,
		RepositoryURL: repo.CloneURL,
		ObjectIID:     prNumber,
		ObjectURL:     objectURL,
		Title:         issue.Title,
		Description:   issue.Body,
		Version:       strconv.FormatInt(comment.ID, 10),
		DedupeKey:     fmt.Sprintf("github:mr-rework:%s:%d", projectID, prNumber),
		Metadata: map[string]any{
			"note_id":                  int(comment.ID),
			"note_url":                 strings.TrimSpace(comment.HTMLURL),
			"resume_context":           resumeContext,
			"requester_id":             int(comment.User.ID),
			"requester_name":           githubDisplayName(comment.User),
			"requester_username":       githubUserName(comment.User),
			"requester_email":          strings.TrimSpace(githubUserEmail(comment.User)),
			"requester_pre_authorized": githubReworkRequesterPreAuthorized(comment.User, issue, n.reworkAllowedUserIDs),
			"assignee_ids":             githubUserIDs(issue.Assignees),
		},
	}}, nil
}

// githubReworkRequesterPreAuthorized 与 GitLab/Gitea 侧对齐:PR 作者(issue.user)、
// PR assignee 与平台级白名单直接授权,其余请求者交给下游按起源 issue 的
// assignee 校验。
func githubReworkRequesterPreAuthorized(requester *githubUser, issue *githubIssue, reworkAllowedUserIDs map[int]struct{}) bool {
	if requester == nil || requester.ID <= 0 || issue == nil {
		return false
	}
	if issue.User != nil && issue.User.ID == requester.ID {
		return true
	}
	for _, assigneeID := range githubUserIDs(issue.Assignees) {
		if assigneeID == int(requester.ID) {
			return true
		}
	}
	_, ok := reworkAllowedUserIDs[int(requester.ID)]
	return ok
}

func hasGitHubLabel(labels []githubLabel, expected string) bool {
	for _, label := range labels {
		if strings.EqualFold(strings.TrimSpace(label.Name), expected) {
			return true
		}
	}
	return false
}

func isOpenGitHubState(state string) bool {
	return strings.EqualFold(strings.TrimSpace(state), "open")
}

func requireGitHubFields(fields map[string]string) error {
	for name, value := range fields {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("github %s is required", name)
		}
	}
	return nil
}

func githubUserIDs(users []*githubUser) []int {
	result := make([]int, 0, len(users))
	for _, user := range users {
		if user != nil && user.ID > 0 {
			result = append(result, int(user.ID))
		}
	}
	return result
}

func githubAssigneesMetadata(assignees []*githubUser) []map[string]any {
	result := make([]map[string]any, 0, len(assignees))
	for _, assignee := range assignees {
		if assignee == nil || (assignee.ID <= 0 && strings.TrimSpace(assignee.Login) == "") {
			continue
		}
		result = append(result, map[string]any{
			"id":       assignee.ID,
			"username": strings.TrimSpace(assignee.Login),
		})
	}
	return result
}

func githubDisplayName(user *githubUser) string {
	if user == nil {
		return ""
	}
	if name := strings.TrimSpace(user.Name); name != "" {
		return name
	}
	return strings.TrimSpace(user.Login)
}

func githubUserName(user *githubUser) string {
	if user == nil {
		return ""
	}
	return strings.TrimSpace(user.Login)
}

func githubUserEmail(user *githubUser) string {
	if user == nil {
		return ""
	}
	return strings.TrimSpace(user.Email)
}
