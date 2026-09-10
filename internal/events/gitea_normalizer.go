package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"sdlc-coordinator/internal/operationmarker"
)

// GiteaNormalizer 把 Gitea webhook 载荷(issue / pull_request / issue_comment)
// 规范化为内部事件,守卫语义与 GitLabNormalizer 对齐:developer-agent 标签门禁、
// 交付分支校验、同仓 PR、协调器操作评论过滤。
type GiteaNormalizer struct {
	reworkAllowedUserIDs map[int]struct{}
}

type GiteaNormalizerOption func(*GiteaNormalizer)

// WithGiteaReworkAllowedUserIDs 配置平台级 rework 白名单(平台 user id),
// 语义与 GitLab 侧 WithGitLabReworkAllowedUserIDsLookup 一致。
func WithGiteaReworkAllowedUserIDs(ids []int) GiteaNormalizerOption {
	return func(n *GiteaNormalizer) {
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

func NewGiteaNormalizer(options ...GiteaNormalizerOption) GiteaNormalizer {
	normalizer := GiteaNormalizer{reworkAllowedUserIDs: map[int]struct{}{}}
	for _, option := range options {
		option(&normalizer)
	}
	return normalizer
}

func (GiteaNormalizer) Source() string { return "gitea" }

func (n GiteaNormalizer) Receipt(raw []byte, metadata WebhookMetadata) WebhookReceipt {
	receipt := WebhookReceipt{EventType: strings.TrimSpace(metadata.EventType)}
	if deliveryID := strings.TrimSpace(metadata.DeliveryID); deliveryID != "" {
		receipt.DedupeKey = fmt.Sprintf("gitea:webhook:delivery:%s", deliveryID)
		return receipt
	}

	var payload giteaWebhookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		receipt.DedupeKey = fmt.Sprintf("gitea:webhook:%s:%s", receipt.EventType, shortRawSHA256(raw))
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
	receipt.DedupeKey = fmt.Sprintf("gitea:webhook:%s:%d:%d:%s:%s",
		receipt.EventType, payload.RepositoryID(), objectNumber, receipt.EventAction, shortRawSHA256(raw))
	return receipt
}

type giteaWebhookPayload struct {
	Action string `json:"action"`
	Number int    `json:"number"`
	// synchronized 交付若带顶层 before,即为增量审查起点(上一版源分支头)。
	Before      string            `json:"before"`
	Issue       *giteaIssue       `json:"issue"`
	PullRequest *giteaPullRequest `json:"pull_request"`
	Comment     *giteaComment     `json:"comment"`
	Repository  *giteaRepository  `json:"repository"`
	Sender      *giteaUser        `json:"sender"`
}

func (payload *giteaWebhookPayload) RepositoryID() int64 {
	if payload.Repository != nil {
		return payload.Repository.ID
	}
	return 0
}

type giteaUser struct {
	ID       int64  `json:"id"`
	UserName string `json:"login"`
	FullName string `json:"full_name"`
	Email    string `json:"email"`
}

type giteaLabel struct {
	Name string `json:"name"`
}

type giteaIssue struct {
	ID          int64           `json:"id"`
	Number      int             `json:"number"`
	Title       string          `json:"title"`
	Body        string          `json:"body"`
	State       string          `json:"state"`
	HTMLURL     string          `json:"html_url"`
	UpdatedAt   string          `json:"updated_at"`
	User        *giteaUser      `json:"user"`
	Labels      []giteaLabel    `json:"labels"`
	Assignees   []*giteaUser    `json:"assignees"`
	PullRequest *map[string]any `json:"pull_request"`
}

func (issue *giteaIssue) PullRequestURL() string {
	if issue == nil || issue.PullRequest == nil {
		return ""
	}
	// Gitea PullRequestMeta 只带 html_url;保留 url 键以兼容 GitHub 形状的载荷。
	for _, key := range []string{"html_url", "url"} {
		if value, ok := (*issue.PullRequest)[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

type giteaPullRequest struct {
	ID        int64          `json:"id"`
	Number    int            `json:"number"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	State     string         `json:"state"`
	Merged    bool           `json:"merged"`
	HTMLURL   string         `json:"html_url"`
	Head      giteaBranchRef `json:"head"`
	Base      giteaBranchRef `json:"base"`
	UpdatedAt string         `json:"updated_at"`
	User      *giteaUser     `json:"user"`
	Assignees []*giteaUser   `json:"assignees"`
}

type giteaBranchRef struct {
	Ref  string           `json:"ref"`
	SHA  string           `json:"sha"`
	Repo *giteaRepository `json:"repo"`
}

type giteaRepository struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	CloneURL      string `json:"clone_url"`
	HTMLURL       string `json:"html_url"`
	DefaultBranch string `json:"default_branch"`
}

type giteaComment struct {
	ID      int64      `json:"id"`
	Body    string     `json:"body"`
	HTMLURL string     `json:"html_url"`
	User    *giteaUser `json:"user"`
}

func (n GiteaNormalizer) Normalize(raw []byte) (*NormalizationResult, error) {
	var payload giteaWebhookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode gitea webhook payload: %w", err)
	}

	// issue_comment:评论挂在 issue 上;issue 带 pull_request 链接即为 PR 评论。
	if payload.Comment != nil && payload.Issue != nil {
		if payload.Issue.PullRequest != nil {
			return n.normalizeGiteaPRComment(payload)
		}
		return n.normalizeGiteaIssueComment(payload)
	}
	if payload.PullRequest != nil {
		return n.normalizeGiteaPullRequest(payload)
	}
	if payload.Issue != nil {
		return n.normalizeGiteaIssue(payload)
	}
	return ignoredNormalization("unsupported gitea webhook payload")
}

func (n GiteaNormalizer) normalizeGiteaPullRequest(payload giteaWebhookPayload) (*NormalizationResult, error) {
	pr := payload.PullRequest
	repo := payload.Repository
	switch strings.ToLower(strings.TrimSpace(payload.Action)) {
	case "opened", "synchronized", "reopened":
	case "closed":
		return n.normalizeGiteaPullRequestLifecycle(payload)
	default:
		return ignoredNormalization("unsupported pull_request action")
	}
	if !isOpenedGiteaState(pr.State) {
		return ignoredNormalization("pull request is not opened")
	}
	if repo == nil || repo.ID <= 0 {
		return nil, errors.New("gitea repository id is required")
	}
	if pr.Number <= 0 {
		return nil, errors.New("gitea pull request number is required")
	}
	if strings.EqualFold(strings.TrimSpace(pr.Head.Ref), strings.TrimSpace(pr.Base.Ref)) ||
		strings.EqualFold(strings.TrimSpace(pr.Head.Ref), strings.TrimSpace(repo.DefaultBranch)) {
		return ignoredNormalization("pull request source branch is not a delivery branch")
	}
	if pr.Base.Repo != nil && repo != nil && pr.Base.Repo.FullName != "" && !strings.EqualFold(pr.Base.Repo.FullName, repo.FullName) {
		return ignoredNormalization("pull request is not a same-repository pull request")
	}

	projectID := strconv.FormatInt(repo.ID, 10)
	if err := requireGiteaFields(map[string]string{
		"repository.full_name":  repo.FullName,
		"repository.clone_url":  repo.CloneURL,
		"pull_request.html_url": pr.HTMLURL,
		"pull_request.head.sha": pr.Head.SHA,
	}); err != nil {
		return nil, err
	}

	// 平台动作词归一到跨平台词表(与 GitLab 对齐):下游按 update+base_sha 判
	// 增量审查。Gitea 的 synchronized 交付通常不带 before,缺增量起点时
	// base_sha 为空,任务工厂自动回落完整审查(优雅降级)。
	eventAction := strings.ToLower(strings.TrimSpace(payload.Action))
	incrementBase := ""
	metadata := map[string]any{
		"author_name":     giteaDisplayName(pr.User),
		"author_username": giteaUserName(pr.User),
		"author_email":    strings.TrimSpace(giteaUserEmail(pr.User)),
		"assignee_ids":    giteaUserIDs(pr.Assignees),
	}
	if eventAction == "synchronized" {
		eventAction = "update"
		incrementBase = strings.TrimSpace(payload.Before)
		metadata["raw_action"] = strings.TrimSpace(payload.Action)
	}

	return &NormalizationResult{Event: &NormalizedEvent{
		Source:        "gitea",
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
		Version:       strings.TrimSpace(pr.UpdatedAt),
		DedupeKey:     fmt.Sprintf("gitea:mr-review:%s:%d:%s", projectID, pr.Number, strings.TrimSpace(pr.Head.SHA)),
		Metadata:      metadata,
	}}, nil
}

func (n GiteaNormalizer) normalizeGiteaPullRequestLifecycle(payload giteaWebhookPayload) (*NormalizationResult, error) {
	pr := payload.PullRequest
	repo := payload.Repository
	if repo == nil || repo.ID <= 0 {
		return nil, errors.New("gitea repository id is required")
	}
	state := "closed"
	if pr.Merged {
		state = "merged"
	}
	// Gitea 合并与关闭共用动作词 closed;归一到下游 switch 认的
	// merge/close(与 GitLab 词表一致),原始动作词留档 metadata。
	eventAction := "close"
	if pr.Merged {
		eventAction = "merge"
	}
	projectID := strconv.FormatInt(repo.ID, 10)
	return &NormalizationResult{Event: &NormalizedEvent{
		Source:        "gitea",
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
		DedupeKey: fmt.Sprintf("gitea:mr-rework:%s:%d", projectID, pr.Number),
		Metadata: map[string]any{
			"lifecycle_state": state,
			"raw_action":      strings.TrimSpace(payload.Action),
			"assignee_ids":    giteaUserIDs(pr.Assignees),
		},
	}}, nil
}

func (n GiteaNormalizer) normalizeGiteaIssue(payload giteaWebhookPayload) (*NormalizationResult, error) {
	issue := payload.Issue
	repo := payload.Repository
	if !isOpenedGiteaState(issue.State) {
		return ignoredNormalization("issue is not opened")
	}
	// label_updated 是 UI 流程的事实启动信号:issue 先建后打标签,此时
	// issue.labels 已包含 developer-agent;与带标签的 opened 同等对待。
	switch strings.ToLower(strings.TrimSpace(payload.Action)) {
	case "opened", "label_updated":
	default:
		return ignoredNormalization("unsupported issue action")
	}
	if !hasGiteaLabel(issue.Labels, "developer-agent") {
		return ignoredNormalization("issue does not have developer-agent label")
	}
	if repo == nil || repo.ID <= 0 {
		return nil, errors.New("gitea repository id is required")
	}
	if issue.Number <= 0 {
		return nil, errors.New("gitea issue number is required")
	}

	projectID := strconv.FormatInt(repo.ID, 10)
	if err := requireGiteaFields(map[string]string{
		"repository.full_name": repo.FullName,
		"repository.clone_url": repo.CloneURL,
		"issue.html_url":       issue.HTMLURL,
	}); err != nil {
		return nil, err
	}

	return &NormalizationResult{Event: &NormalizedEvent{
		Source:        "gitea",
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
		DedupeKey:     fmt.Sprintf("gitea:issue-develop:%s:%d", projectID, issue.Number),
		Metadata: map[string]any{
			"assignee_ids": giteaUserIDs(issue.Assignees),
			"assignees":    giteaAssigneesMetadata(issue.Assignees),
		},
	}}, nil
}

func (n GiteaNormalizer) normalizeGiteaIssueComment(payload giteaWebhookPayload) (*NormalizationResult, error) {
	issue := payload.Issue
	comment := payload.Comment
	repo := payload.Repository
	if !strings.EqualFold(strings.TrimSpace(payload.Action), "created") {
		return ignoredNormalization("comment action is not created")
	}
	if !isOpenedGiteaState(issue.State) {
		return ignoredNormalization("issue is not opened")
	}
	if !hasGiteaLabel(issue.Labels, "developer-agent") {
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
	allowed := resumeRequesterAllowed(int(comment.User.ID), issueAuthorID, giteaUserIDs(issue.Assignees), n.reworkAllowedUserIDs)
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
		return nil, errors.New("gitea repository id is required")
	}
	if issue.Number <= 0 || comment.ID <= 0 {
		return nil, errors.New("gitea issue number and comment id are required")
	}

	projectID := strconv.FormatInt(repo.ID, 10)
	if err := requireGiteaFields(map[string]string{
		"repository.full_name": repo.FullName,
		"repository.clone_url": repo.CloneURL,
		"issue.html_url":       issue.HTMLURL,
	}); err != nil {
		return nil, err
	}

	return &NormalizationResult{Event: &NormalizedEvent{
		Source:        "gitea",
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
		DedupeKey:     fmt.Sprintf("gitea:issue-develop:%s:%d", projectID, issue.Number),
		Metadata: map[string]any{
			"assignee_ids":         giteaUserIDs(issue.Assignees),
			"comment_id":           giteaInt(comment.ID),
			"comment_url":          strings.TrimSpace(comment.HTMLURL),
			"resume_context":       resumeContext,
			"requester_id":         giteaInt(comment.User.ID),
			"requester_authorized": allowed,
		},
	}}, nil
}

func (n GiteaNormalizer) normalizeGiteaPRComment(payload giteaWebhookPayload) (*NormalizationResult, error) {
	issue := payload.Issue
	comment := payload.Comment
	repo := payload.Repository
	if !strings.EqualFold(strings.TrimSpace(payload.Action), "created") {
		return ignoredNormalization("comment action is not created")
	}
	// PR 的 issue 与 PR 共用状态:已关闭/已合并的 MR 不再接受 rework 请求。
	if !isOpenedGiteaState(issue.State) {
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
	prNumber := 0
	if issue.PullRequest != nil {
		if url := strings.TrimSpace(issue.PullRequestURL()); url != "" {
			if index := strings.LastIndex(url, "/"); index >= 0 {
				if parsed, err := strconv.Atoi(url[index+1:]); err == nil {
					prNumber = parsed
				}
			}
		}
	}
	if prNumber <= 0 && payload.Number > 0 {
		prNumber = payload.Number
	}
	if repo == nil || repo.ID <= 0 {
		return nil, errors.New("gitea repository id is required")
	}
	if prNumber <= 0 || comment.ID <= 0 {
		return nil, errors.New("gitea pull request number and comment id are required")
	}
	objectURL := strings.TrimSpace(issue.PullRequestURL())
	if objectURL == "" {
		// PR 的 issue.html_url 即 PR 页面,作为链接兜底。
		objectURL = strings.TrimSpace(issue.HTMLURL)
	}

	projectID := strconv.FormatInt(repo.ID, 10)
	if err := requireGiteaFields(map[string]string{
		"repository.full_name": repo.FullName,
		"repository.clone_url": repo.CloneURL,
	}); err != nil {
		return nil, err
	}

	// 评论载荷顶层自带完整 pull_request 对象(Gitea 实测形状):直读 head/base 快照
	// 定位字段供 rework 派发使用;缺失时留空,分支由写回 enrichment 兜底、head SHA
	// 由 workspace 契约兜底,fail-closed 语义不变。
	var headSHA, sourceBranch, targetBranch string
	if pr := payload.PullRequest; pr != nil {
		headSHA = strings.TrimSpace(pr.Head.SHA)
		sourceBranch = strings.TrimSpace(pr.Head.Ref)
		targetBranch = strings.TrimSpace(pr.Base.Ref)
	}

	return &NormalizationResult{Event: &NormalizedEvent{
		Source:        "gitea",
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
		SourceBranch:  sourceBranch,
		TargetBranch:  targetBranch,
		HeadSHA:       headSHA,
		Version:       strconv.FormatInt(comment.ID, 10),
		DedupeKey:     fmt.Sprintf("gitea:mr-rework:%s:%d", projectID, prNumber),
		Metadata: map[string]any{
			"note_id":                  giteaInt(comment.ID),
			"note_url":                 strings.TrimSpace(comment.HTMLURL),
			"resume_context":           resumeContext,
			"requester_id":             giteaInt(comment.User.ID),
			"requester_name":           giteaDisplayName(comment.User),
			"requester_username":       giteaUserName(comment.User),
			"requester_email":          strings.TrimSpace(giteaUserEmail(comment.User)),
			"requester_pre_authorized": giteaReworkRequesterPreAuthorized(comment.User, issue, n.reworkAllowedUserIDs),
			"assignee_ids":             giteaUserIDs(issue.Assignees),
		},
	}}, nil
}

// giteaReworkRequesterPreAuthorized 与 GitLab 侧 reworkRequesterAllowed 对齐:
// PR 作者(issue.user)、PR assignee 与平台级白名单直接授权,其余请求者交给
// 下游按起源 issue 的 assignee 校验。
func giteaReworkRequesterPreAuthorized(requester *giteaUser, issue *giteaIssue, reworkAllowedUserIDs map[int]struct{}) bool {
	if requester == nil || requester.ID <= 0 || issue == nil {
		return false
	}
	if issue.User != nil && issue.User.ID == requester.ID {
		return true
	}
	for _, assigneeID := range giteaUserIDs(issue.Assignees) {
		if assigneeID == int(requester.ID) {
			return true
		}
	}
	_, ok := reworkAllowedUserIDs[int(requester.ID)]
	return ok
}

func hasGiteaLabel(labels []giteaLabel, expected string) bool {
	for _, label := range labels {
		if strings.EqualFold(strings.TrimSpace(label.Name), expected) {
			return true
		}
	}
	return false
}

func isOpenedGiteaState(state string) bool {
	return strings.EqualFold(strings.TrimSpace(state), "open")
}

func requireGiteaFields(fields map[string]string) error {
	for name, value := range fields {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("gitea %s is required", name)
		}
	}
	return nil
}

func giteaUserIDs(users []*giteaUser) []int {
	result := make([]int, 0, len(users))
	for _, user := range users {
		if user != nil && user.ID > 0 {
			result = append(result, int(user.ID))
		}
	}
	return result
}

func giteaInt(value int64) int {
	return int(value)
}

func giteaAssigneesMetadata(assignees []*giteaUser) []map[string]any {
	result := make([]map[string]any, 0, len(assignees))
	for _, assignee := range assignees {
		if assignee == nil || (assignee.ID <= 0 && strings.TrimSpace(assignee.UserName) == "") {
			continue
		}
		result = append(result, map[string]any{
			"id":       assignee.ID,
			"username": strings.TrimSpace(assignee.UserName),
		})
	}
	return result
}

func giteaDisplayName(user *giteaUser) string {
	if user == nil {
		return ""
	}
	if name := strings.TrimSpace(user.FullName); name != "" {
		return name
	}
	return strings.TrimSpace(user.UserName)
}

func giteaUserName(user *giteaUser) string {
	if user == nil {
		return ""
	}
	return strings.TrimSpace(user.UserName)
}

func giteaUserEmail(user *giteaUser) string {
	if user == nil {
		return ""
	}
	return strings.TrimSpace(user.Email)
}
