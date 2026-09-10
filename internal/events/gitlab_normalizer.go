package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"sdlc-coordinator/internal/operationmarker"
)

type GitLabNormalizer struct {
	reworkAllowedUserIDs map[int]struct{}
}

type GitLabNormalizerOption func(*GitLabNormalizer)

// WithCodeHostReworkAllowedUserIDs 配置平台级 rework 白名单(平台 user id)。
// 白名单语义为 codehost 通用,三平台 normalizer 共用同一份配置。
// WithGitLabReworkAllowedUserIDsLookup 以查找函数提供接续开发白名单:
// normalizer 在每次 webhook 处理时重新物化,组织段配置保存后的下一个
// 事件即用新白名单(热生效)。
func WithGitLabReworkAllowedUserIDsLookup(lookup func() []int) GitLabNormalizerOption {
	return func(normalizer *GitLabNormalizer) {
		ids := lookup()
		if normalizer.reworkAllowedUserIDs == nil {
			normalizer.reworkAllowedUserIDs = map[int]struct{}{}
		}
		for _, id := range ids {
			normalizer.reworkAllowedUserIDs[id] = struct{}{}
		}
	}
}

func WithCodeHostReworkAllowedUserIDs(ids []int) GitLabNormalizerOption {
	return func(normalizer *GitLabNormalizer) {
		for _, id := range ids {
			if id > 0 {
				normalizer.reworkAllowedUserIDs[id] = struct{}{}
			}
		}
	}
}

func NewGitLabNormalizer(opts ...GitLabNormalizerOption) GitLabNormalizer {
	normalizer := GitLabNormalizer{reworkAllowedUserIDs: map[int]struct{}{}}
	for _, opt := range opts {
		if opt != nil {
			opt(&normalizer)
		}
	}
	return normalizer
}

func (n GitLabNormalizer) Source() string { return "gitlab" }

// Receipt 从载荷中解析仓库/对象身份,构造幂等回执;交付 UUID 优先。
func (n GitLabNormalizer) Receipt(raw []byte, metadata WebhookMetadata) WebhookReceipt {
	gitlabMeta := GitLabWebhookMetadata{
		EventName:   metadata.EventType,
		EventUUID:   metadata.DeliveryID,
		InstanceURL: metadata.InstanceURL,
	}
	gitlabReceipt := newGitLabReceipt(raw, gitlabMeta)
	return WebhookReceipt{
		DedupeKey:   gitlabReceipt.DedupeKey,
		EventType:   gitlabReceipt.EventType,
		EventAction: gitlabReceipt.EventAction,
		ExternalID:  gitlabReceipt.ExternalID,
	}
}

func (n GitLabNormalizer) Normalize(raw []byte) (*NormalizationResult, error) {
	var payload gitLabWebhookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode gitlab payload: %w", err)
	}

	switch payload.ObjectKind {
	case "merge_request":
		return normalizeGitLabMergeRequest(payload)
	case "issue":
		return normalizeGitLabIssue(payload)
	case "note":
		if strings.EqualFold(strings.TrimSpace(payload.ObjectAttributes.NoteableType), "MergeRequest") {
			return n.normalizeGitLabMergeRequestReworkNote(payload)
		}
		return n.normalizeGitLabIssueResumeNote(payload)
	default:
		return ignoredNormalization("unsupported gitlab object kind")
	}
}

type gitLabWebhookPayload struct {
	ObjectKind       string                 `json:"object_kind"`
	EventType        string                 `json:"event_type"`
	User             gitLabUser             `json:"user"`
	Project          gitLabProject          `json:"project"`
	Repository       gitLabRepository       `json:"repository"`
	ObjectAttributes gitLabObjectAttributes `json:"object_attributes"`
	Labels           []gitLabLabel          `json:"labels"`
	Issue            gitLabIssue            `json:"issue"`
	MergeRequest     gitLabMergeRequest     `json:"merge_request"`
	Assignees        []gitLabAssignee       `json:"assignees"`
}

type gitLabUser struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

type gitLabAssignee struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

type gitLabProject struct {
	ID                int    `json:"id"`
	PathWithNamespace string `json:"path_with_namespace"`
	GitSSHURL         string `json:"git_ssh_url"`
	GitHTTPURL        string `json:"git_http_url"`
	DefaultBranch     string `json:"default_branch"`
}

type gitLabRepository struct {
	GitSSHURL  string `json:"git_ssh_url"`
	GitHTTPURL string `json:"git_http_url"`
	URL        string `json:"url"`
}

type gitLabObjectAttributes struct {
	ID           int              `json:"id"`
	IID          int              `json:"iid"`
	Action       string           `json:"action"`
	State        string           `json:"state"`
	Title        string           `json:"title"`
	Description  string           `json:"description"`
	SourceBranch string           `json:"source_branch"`
	TargetBranch string           `json:"target_branch"`
	URL          string           `json:"url"`
	OldRev       string           `json:"oldrev"`
	UpdatedAt    string           `json:"updated_at"`
	LastCommit   gitLabLastCommit `json:"last_commit"`
	AssigneeIDs  []int            `json:"assignee_ids"`
	NoteableType string           `json:"noteable_type"`
	Note         string           `json:"note"`
	System       bool             `json:"system"`
	Type         string           `json:"type"`
	DiscussionID string           `json:"discussion_id"`
}

type gitLabLastCommit struct {
	ID     string       `json:"id"`
	Author gitLabAuthor `json:"author"`
}

type gitLabAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type gitLabLabel struct {
	Title string `json:"title"`
}

type gitLabIssue struct {
	IID         int           `json:"iid"`
	AuthorID    int           `json:"author_id"`
	State       string        `json:"state"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	URL         string        `json:"url"`
	AssigneeIDs []int         `json:"assignee_ids"`
	Labels      []gitLabLabel `json:"labels"`
}

type gitLabMergeRequest struct {
	ID              int              `json:"id"`
	IID             int              `json:"iid"`
	AuthorID        int              `json:"author_id"`
	AssigneeID      int              `json:"assignee_id"`
	AssigneeIDs     []int            `json:"assignee_ids"`
	ReviewerIDs     []int            `json:"reviewer_ids"`
	State           string           `json:"state"`
	Title           string           `json:"title"`
	Description     string           `json:"description"`
	SourceBranch    string           `json:"source_branch"`
	TargetBranch    string           `json:"target_branch"`
	SourceProjectID int              `json:"source_project_id"`
	TargetProjectID int              `json:"target_project_id"`
	URL             string           `json:"url"`
	LastCommit      gitLabLastCommit `json:"last_commit"`
}

func (n GitLabNormalizer) normalizeGitLabMergeRequestReworkNote(payload gitLabWebhookPayload) (*NormalizationResult, error) {
	attr := payload.ObjectAttributes
	mr := payload.MergeRequest
	if !strings.EqualFold(strings.TrimSpace(attr.Action), "create") {
		return ignoredNormalization("note action is not create")
	}
	if attr.System {
		return ignoredNormalization("system note")
	}
	if !isOpenedGitLabState(mr.State) {
		return ignoredNormalization("merge request is not opened")
	}
	if operationmarker.Contains(attr.Note) {
		return ignoredNormalization("coordinator operation note")
	}
	resumeContext, ok := parseGitLabComment(attr.Note)
	if !ok {
		return ignoredNormalization("merge request comment context is required")
	}
	if strings.TrimSpace(resumeContext) == "" {
		return ignoredNormalization("resume comment context is required")
	}
	if payload.Project.ID <= 0 {
		return nil, errors.New("gitlab project id is required")
	}
	if mr.IID <= 0 {
		return nil, errors.New("gitlab merge request iid is required")
	}
	if attr.ID <= 0 {
		return nil, errors.New("gitlab note id is required")
	}
	if mr.SourceProjectID != payload.Project.ID || mr.TargetProjectID != payload.Project.ID {
		return ignoredNormalization("merge request is not a same-project merge request")
	}
	if strings.EqualFold(strings.TrimSpace(mr.SourceBranch), strings.TrimSpace(mr.TargetBranch)) ||
		strings.EqualFold(strings.TrimSpace(mr.SourceBranch), strings.TrimSpace(payload.Project.DefaultBranch)) {
		return ignoredNormalization("merge request source branch is not a delivery branch")
	}
	requesterPreAuthorized := n.reworkRequesterAllowed(payload.User.ID, mr)

	projectID := strconv.Itoa(payload.Project.ID)
	repoURL := gitLabRepositoryURL(payload)
	if err := requireGitLabFields(map[string]string{
		"project.path_with_namespace":  payload.Project.PathWithNamespace,
		"repository url":               repoURL,
		"merge_request.url":            mr.URL,
		"merge_request.source_branch":  mr.SourceBranch,
		"merge_request.target_branch":  mr.TargetBranch,
		"merge_request.last_commit.id": mr.LastCommit.ID,
		"object_attributes.url":        attr.URL,
	}); err != nil {
		return nil, err
	}

	return &NormalizationResult{Event: &NormalizedEvent{
		Source:        "gitlab",
		Kind:          EventKindMergeRequestReworkRequested,
		EventType:     eventTypeOrDefault(payload.EventType, payload.ObjectKind),
		EventAction:   attr.Action,
		ProjectID:     projectID,
		ProjectPath:   payload.Project.PathWithNamespace,
		RepositoryURL: repoURL,
		ObjectIID:     mr.IID,
		ObjectURL:     mr.URL,
		Title:         mr.Title,
		Description:   mr.Description,
		SourceBranch:  mr.SourceBranch,
		TargetBranch:  mr.TargetBranch,
		HeadSHA:       strings.TrimSpace(mr.LastCommit.ID),
		Version:       strconv.Itoa(attr.ID),
		DedupeKey:     fmt.Sprintf("gitlab:mr-rework:%s:%d", projectID, mr.IID),
		Metadata: map[string]any{
			"note_id":                  attr.ID,
			"note_url":                 strings.TrimSpace(attr.URL),
			"note_type":                strings.TrimSpace(attr.Type),
			"discussion_id":            strings.TrimSpace(attr.DiscussionID),
			"resume_context":           strings.TrimSpace(resumeContext),
			"requester_id":             payload.User.ID,
			"requester_name":           strings.TrimSpace(payload.User.Name),
			"requester_username":       strings.TrimSpace(payload.User.Username),
			"requester_email":          strings.TrimSpace(payload.User.Email),
			"author_id":                mr.AuthorID,
			"assignee_ids":             append([]int(nil), mr.AssigneeIDs...),
			"reviewer_ids":             append([]int(nil), mr.ReviewerIDs...),
			"source_project_id":        mr.SourceProjectID,
			"target_project_id":        mr.TargetProjectID,
			"requester_pre_authorized": requesterPreAuthorized,
		},
	}}, nil
}

func (n GitLabNormalizer) reworkRequesterAllowed(userID int, mr gitLabMergeRequest) bool {
	if userID <= 0 {
		return false
	}
	if userID == mr.AuthorID || userID == mr.AssigneeID {
		return true
	}
	for _, id := range append(append([]int(nil), mr.AssigneeIDs...), mr.ReviewerIDs...) {
		if userID == id {
			return true
		}
	}
	_, ok := n.reworkAllowedUserIDs[userID]
	return ok
}

func normalizeGitLabMergeRequest(payload gitLabWebhookPayload) (*NormalizationResult, error) {
	attr := payload.ObjectAttributes
	action := strings.ToLower(strings.TrimSpace(attr.Action))
	state := strings.ToLower(strings.TrimSpace(attr.State))
	if (action == "merge" && state == "merged") || (action == "close" && state == "closed") {
		return normalizeGitLabMergeRequestLifecycle(payload, state)
	}
	if !isOpenedGitLabState(attr.State) {
		return ignoredNormalization("merge request is not opened")
	}

	switch action {
	case "open", "opened":
	case "update":
		if strings.TrimSpace(attr.OldRev) == "" {
			return ignoredNormalization("merge request update does not include oldrev")
		}
	default:
		return ignoredNormalization("unsupported merge request action")
	}

	projectID := strconv.Itoa(payload.Project.ID)
	repoURL := gitLabRepositoryURL(payload)
	if payload.Project.ID <= 0 {
		return nil, errors.New("gitlab project id is required")
	}
	if err := requireGitLabFields(map[string]string{
		"project.id":                       projectID,
		"project.path_with_namespace":      payload.Project.PathWithNamespace,
		"repository url":                   repoURL,
		"object_attributes.url":            attr.URL,
		"object_attributes.last_commit.id": attr.LastCommit.ID,
		"object_attributes.source_branch":  attr.SourceBranch,
		"object_attributes.target_branch":  attr.TargetBranch,
	}); err != nil {
		return nil, err
	}
	if attr.IID <= 0 {
		return nil, errors.New("gitlab merge request iid is required")
	}

	headSHA := strings.TrimSpace(attr.LastCommit.ID)
	event := &NormalizedEvent{
		Source:        "gitlab",
		Kind:          EventKindChangeRequestUpdated,
		EventType:     eventTypeOrDefault(payload.EventType, payload.ObjectKind),
		EventAction:   attr.Action,
		ProjectID:     projectID,
		ProjectPath:   payload.Project.PathWithNamespace,
		RepositoryURL: repoURL,
		ObjectIID:     attr.IID,
		ObjectURL:     attr.URL,
		Title:         attr.Title,
		Description:   attr.Description,
		SourceBranch:  attr.SourceBranch,
		TargetBranch:  attr.TargetBranch,
		HeadSHA:       headSHA,
		BaseSHA:       attr.OldRev,
		Version:       headSHA,
		DedupeKey:     fmt.Sprintf("gitlab:mr-review:%s:%d:%s", projectID, attr.IID, headSHA),
		Metadata:      gitLabMergeRequestMetadata(payload),
	}
	return &NormalizationResult{Event: event}, nil
}

func normalizeGitLabMergeRequestLifecycle(payload gitLabWebhookPayload, state string) (*NormalizationResult, error) {
	attr := payload.ObjectAttributes
	if payload.Project.ID <= 0 {
		return nil, errors.New("gitlab project id is required")
	}
	if attr.IID <= 0 {
		return nil, errors.New("gitlab merge request iid is required")
	}
	projectID := strconv.Itoa(payload.Project.ID)
	if err := requireGitLabFields(map[string]string{
		"project.path_with_namespace": payload.Project.PathWithNamespace,
		"object_attributes.url":       attr.URL,
	}); err != nil {
		return nil, err
	}

	return &NormalizationResult{Event: &NormalizedEvent{
		Source:        "gitlab",
		Kind:          EventKindMergeRequestLifecycleChanged,
		EventType:     eventTypeOrDefault(payload.EventType, payload.ObjectKind),
		EventAction:   attr.Action,
		ProjectID:     projectID,
		ProjectPath:   payload.Project.PathWithNamespace,
		RepositoryURL: gitLabRepositoryURL(payload),
		ObjectIID:     attr.IID,
		ObjectURL:     attr.URL,
		Title:         attr.Title,
		Description:   attr.Description,
		SourceBranch:  attr.SourceBranch,
		TargetBranch:  attr.TargetBranch,
		HeadSHA:       strings.TrimSpace(attr.LastCommit.ID),
		Version:       state + ":" + strings.TrimSpace(attr.LastCommit.ID),
		DedupeKey:     fmt.Sprintf("gitlab:mr-rework:%s:%d", projectID, attr.IID),
		Metadata: map[string]any{
			"lifecycle_state": state,
		},
	}}, nil
}

func gitLabMergeRequestMetadata(payload gitLabWebhookPayload) map[string]any {
	name := strings.TrimSpace(payload.ObjectAttributes.LastCommit.Author.Name)
	email := strings.TrimSpace(payload.ObjectAttributes.LastCommit.Author.Email)
	if name == "" {
		name = strings.TrimSpace(payload.User.Name)
	}
	if email == "" {
		email = strings.TrimSpace(payload.User.Email)
	}
	return map[string]any{
		"author_name":     name,
		"author_username": strings.TrimSpace(payload.User.Username),
		"author_email":    email,
	}
}

func normalizeGitLabIssue(payload gitLabWebhookPayload) (*NormalizationResult, error) {
	attr := payload.ObjectAttributes
	if !isOpenedGitLabState(attr.State) {
		return ignoredNormalization("issue is not opened")
	}

	action := strings.ToLower(attr.Action)
	switch action {
	case "open", "opened":
	default:
		return ignoredNormalization("unsupported issue action")
	}
	if !hasGitLabLabel(payload.Labels, "developer-agent") {
		return ignoredNormalization("issue does not have developer-agent label")
	}

	projectID := strconv.Itoa(payload.Project.ID)
	repoURL := gitLabRepositoryURL(payload)
	if payload.Project.ID <= 0 {
		return nil, errors.New("gitlab project id is required")
	}
	if err := requireGitLabFields(map[string]string{
		"project.id":                  projectID,
		"project.path_with_namespace": payload.Project.PathWithNamespace,
		"repository url":              repoURL,
		"object_attributes.url":       attr.URL,
	}); err != nil {
		return nil, err
	}
	if attr.IID <= 0 {
		return nil, errors.New("gitlab issue iid is required")
	}

	event := &NormalizedEvent{
		Source:        "gitlab",
		Kind:          EventKindWorkItemUpdated,
		EventType:     eventTypeOrDefault(payload.EventType, payload.ObjectKind),
		EventAction:   attr.Action,
		ProjectID:     projectID,
		ProjectPath:   payload.Project.PathWithNamespace,
		RepositoryURL: repoURL,
		ObjectIID:     attr.IID,
		ObjectURL:     attr.URL,
		Title:         attr.Title,
		Description:   attr.Description,
		TargetBranch:  payload.Project.DefaultBranch,
		Version:       attr.UpdatedAt,
		DedupeKey:     fmt.Sprintf("gitlab:issue-develop:%s:%d", projectID, attr.IID),
		Metadata: map[string]any{
			"assignee_ids": append([]int(nil), attr.AssigneeIDs...),
			"assignees":    gitLabAssigneesMetadata(payload.Assignees),
		},
	}
	return &NormalizationResult{Event: event}, nil
}

func gitLabAssigneesMetadata(assignees []gitLabAssignee) []map[string]any {
	result := make([]map[string]any, 0, len(assignees))
	for _, assignee := range assignees {
		username := strings.TrimSpace(assignee.Username)
		if assignee.ID <= 0 && username == "" {
			continue
		}
		result = append(result, map[string]any{
			"id":       assignee.ID,
			"username": username,
		})
	}
	return result
}

func (n GitLabNormalizer) normalizeGitLabIssueResumeNote(payload gitLabWebhookPayload) (*NormalizationResult, error) {
	attr := payload.ObjectAttributes
	issue := payload.Issue
	if !strings.EqualFold(strings.TrimSpace(attr.NoteableType), "issue") {
		return ignoredNormalization("note is not attached to an issue")
	}
	if !strings.EqualFold(strings.TrimSpace(attr.Action), "create") {
		return ignoredNormalization("note action is not create")
	}
	if !isOpenedGitLabState(issue.State) {
		return ignoredNormalization("issue is not opened")
	}
	if !hasGitLabLabel(issue.Labels, "developer-agent") {
		return ignoredNormalization("issue does not have developer-agent label")
	}
	if operationmarker.Contains(attr.Note) {
		return ignoredNormalization("coordinator operation note")
	}
	// resume 评论的请求者白名单:作者 ∪ 指派人 ∪ 平台白名单;无指派人降级为仅作者。
	allowed := resumeRequesterAllowed(payload.User.ID, issue.AuthorID, issue.AssigneeIDs, n.reworkAllowedUserIDs)
	if !allowed {
		return ignoredNormalization("resume requester is not authorized")
	}
	resumeContext, ok := parseGitLabComment(attr.Note)
	if !ok {
		return ignoredNormalization("issue comment context is required")
	}
	if strings.TrimSpace(resumeContext) == "" {
		return ignoredNormalization("resume comment context is required")
	}
	if payload.Project.ID <= 0 {
		return nil, errors.New("gitlab project id is required")
	}
	projectID := strconv.Itoa(payload.Project.ID)
	repoURL := gitLabRepositoryURL(payload)
	if err := requireGitLabFields(map[string]string{
		"project.id":                  projectID,
		"project.path_with_namespace": payload.Project.PathWithNamespace,
		"repository url":              repoURL,
		"issue.url":                   issue.URL,
	}); err != nil {
		return nil, err
	}
	if issue.IID <= 0 {
		return nil, errors.New("gitlab issue iid is required")
	}
	if attr.ID <= 0 {
		return nil, errors.New("gitlab note id is required")
	}

	return &NormalizationResult{Event: &NormalizedEvent{
		Source:        "gitlab",
		Kind:          EventKindDeveloperResumeRequested,
		EventType:     eventTypeOrDefault(payload.EventType, payload.ObjectKind),
		EventAction:   attr.Action,
		ProjectID:     projectID,
		ProjectPath:   payload.Project.PathWithNamespace,
		RepositoryURL: repoURL,
		ObjectIID:     issue.IID,
		ObjectURL:     issue.URL,
		Title:         issue.Title,
		Description:   issue.Description,
		TargetBranch:  payload.Project.DefaultBranch,
		Version:       strconv.Itoa(attr.ID),
		DedupeKey:     fmt.Sprintf("gitlab:issue-develop:%s:%d", projectID, issue.IID),
		Metadata: map[string]any{
			"assignee_ids":         append([]int(nil), issue.AssigneeIDs...),
			"comment_id":           attr.ID,
			"resume_context":       resumeContext,
			"requester_id":         payload.User.ID,
			"requester_authorized": allowed,
		},
	}}, nil
}

func hasGitLabLabel(labels []gitLabLabel, expected string) bool {
	for _, label := range labels {
		if strings.EqualFold(strings.TrimSpace(label.Title), expected) {
			return true
		}
	}
	return false
}

// parseGitLabComment 把整条评论作为 resume/rework 上下文。正文首个非空行
// 恰为 "Developer Agent: resume" 时剥离该行，仅剩标记的评论
// 由上层按空上下文拒绝。
func parseGitLabComment(note string) (string, bool) {
	note = strings.TrimSpace(strings.ReplaceAll(note, "\r\n", "\n"))
	if note == "" {
		return "", false
	}
	lines := strings.Split(note, "\n")
	for index, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.TrimSpace(line) == "Developer Agent: resume" {
			return strings.TrimSpace(strings.Join(lines[index+1:], "\n")), true
		}
		break
	}
	return note, true
}

func isOpenedGitLabState(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "open", "opened":
		return true
	default:
		return false
	}
}

func gitLabRepositoryURL(payload gitLabWebhookPayload) string {
	for _, candidate := range []string{
		payload.Project.GitHTTPURL,
		payload.Repository.GitHTTPURL,
		payload.Project.GitSSHURL,
		payload.Repository.GitSSHURL,
		payload.Repository.URL,
	} {
		if strings.TrimSpace(candidate) != "" {
			return candidate
		}
	}
	return ""
}

func eventTypeOrDefault(eventType, objectKind string) string {
	if eventType != "" {
		return eventType
	}
	return objectKind
}

func requireGitLabFields(fields map[string]string) error {
	for name, value := range fields {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	return nil
}
