package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
)

var ErrMergeRequestNotManaged = model.ErrMergeRequestNotManaged

func (o *Orchestrator) StartOrResumeMergeRequestRework(ctx context.Context, request model.MergeRequestReworkRequest) (*model.Task, error) {
	if o.writebacks == nil {
		return nil, ErrMergeRequestNotManaged
	}

	writeback, writebackErr := o.writebacks.GetSucceededMergeRequest(ctx, request.ProjectPath, request.MergeRequestIID)
	// Gitea 等平台的评论载荷不携带 MR 分支;coordinator 自建 MR 的写回请求
	// 是分支信息的权威来源,先补齐再校验。
	request = enrichReworkRequestFromWriteback(request, writeback)
	// GitHub 的 PR 评论载荷连标记对象都不带分支,外部 PR 无写回记录可查,
	// 缺口只能回源平台 API 补(fail-closed,见 enrichReworkRequestFromPlatform)。
	var enrichErr error
	request, enrichErr = o.enrichReworkRequestFromPlatform(ctx, request)
	if enrichErr != nil {
		return nil, enrichErr
	}
	if err := validateMergeRequestReworkRequest(request); err != nil {
		return nil, err
	}
	origin, err := reworkOriginTask(o.tasks, ctx, writeback, writebackErr)
	if err != nil {
		return nil, err
	}
	if origin != nil {
		if origin.Role != "DeveloperWorker" || origin.Type != "develop_from_issue" || normalizedReworkProjectPath(origin.TargetRepo) != normalizedReworkProjectPath(request.ProjectPath) {
			return nil, ErrMergeRequestNotManaged
		}
	}
	if !request.RequesterPreAuthorized && !originIssueAssigneeAllowed(origin, request.RequesterID) {
		return nil, model.ErrMergeRequestReworkRequesterUnauthorized
	}

	latestReview, err := o.latestReviewerComment(ctx, request.ProjectPath, request.MergeRequestIID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	contextJSON, err := mergeRequestReworkContext(origin, request, latestReview)
	if err != nil {
		return nil, err
	}
	existing, err := o.tasks.GetByDedupeKey(ctx, request.DedupeKey)
	if err == nil {
		if existing.Type != model.TaskTypeReworkMergeRequest || existing.Role != "DeveloperWorker" || normalizedReworkProjectPath(existing.TargetRepo) != normalizedReworkProjectPath(request.ProjectPath) {
			return nil, repository.ErrTaskNotClaimable
		}
		if err := o.tasks.ResumeRework(ctx, request.DedupeKey, contextJSON); err != nil {
			return nil, err
		}
		return o.tasks.GetByDedupeKey(ctx, request.DedupeKey)
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}

	task := &model.Task{
		DedupeKey:          request.DedupeKey,
		Type:               model.TaskTypeReworkMergeRequest,
		Role:               "DeveloperWorker",
		RepositoryID:       reworkTaskRepositoryID(origin, request),
		SourceRepo:         request.ProjectPath,
		TargetRepo:         request.ProjectPath,
		Objective:          fmt.Sprintf("Continue development on %s in %s according to the latest authorized rework request: %s", mergeRequestDisplayName(request.Source, request.MergeRequestIID), request.ProjectPath, request.ResumeContext),
		AcceptanceCriteria: "只完成当前 MR 评论要求，运行相关验证，并将非 force commit push 到原 source branch；不创建新分支或新 MR。",
		Status:             model.TaskStatusPending,
		ContextJSON:        contextJSON,
		DeliveryBranch:     request.SourceBranch,
	}
	return o.CreateTask(ctx, task)
}

// reworkTaskRepositoryID 让 rework 任务继承仓库归属：优先取事件侧已确认注册的
// repository_id（denyUnregisteredRepository 设置），回退到起源任务，避免新建任务
// 因缺 repository_id 永久卡在派发门禁外（注册仓模式是唯一运行模式）。
func reworkTaskRepositoryID(origin *model.Task, request model.MergeRequestReworkRequest) *string {
	if request.RepositoryID != nil && strings.TrimSpace(*request.RepositoryID) != "" {
		return request.RepositoryID
	}
	if origin != nil && origin.RepositoryID != nil && strings.TrimSpace(*origin.RepositoryID) != "" {
		return origin.RepositoryID
	}
	return nil
}

// mergeRequestDisplayName 按事件来源平台命名业务对象:GitLab 叫 Merge Request 且
// 序号使用 !,Gitea/GitHub 叫 Pull Request 且序号使用 #;空来源按 Pull Request 处理。
func mergeRequestDisplayName(source string, iid int) string {
	if strings.EqualFold(strings.TrimSpace(source), "gitlab") {
		return fmt.Sprintf("GitLab MR !%d", iid)
	}
	return fmt.Sprintf("Pull Request #%d", iid)
}

type mergeRequestReworkFinalizer interface {
	FinalizeRework(context.Context, string, model.TaskStatus, string, time.Time) (*model.Task, error)
}

// reworkOriginTask 定位评论所指向 MR 的起源开发任务；无创建写回或起源任务缺失时返回
// (nil, nil)，调用方按"非 coordinator 自建 MR"降级（任务包省略 origin_issue）。
func reworkOriginTask(tasks TaskRepository, ctx context.Context, writeback *model.Writeback, writebackErr error) (*model.Task, error) {
	if writebackErr != nil {
		if errors.Is(writebackErr, repository.ErrNotFound) {
			return nil, nil
		}
		return nil, writebackErr
	}
	origin, err := tasks.GetByTaskID(ctx, writeback.TaskID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return origin, nil
}

func (o *Orchestrator) ApplyMergeRequestLifecycle(ctx context.Context, request model.MergeRequestLifecycleRequest) (*model.Task, error) {
	if strings.TrimSpace(request.DedupeKey) == "" || strings.TrimSpace(request.ProjectID) == "" || request.MergeRequestIID <= 0 {
		return nil, errors.New("merge request lifecycle identity is required")
	}
	finalizer, ok := o.tasks.(mergeRequestReworkFinalizer)
	if !ok {
		return nil, errors.New("merge request rework finalizer is required")
	}

	state := strings.ToLower(strings.TrimSpace(request.State))
	action := strings.ToLower(strings.TrimSpace(request.Action))
	switch {
	case state == "merged" && action == "merge":
		return finalizer.FinalizeRework(ctx, request.DedupeKey, model.TaskStatusSucceeded, "", o.now())
	case state == "closed" && action == "close":
		return finalizer.FinalizeRework(ctx, request.DedupeKey, model.TaskStatusBlocked, "merge request closed without merge", o.now())
	default:
		return nil, fmt.Errorf("unsupported merge request lifecycle %s/%s", action, state)
	}
}

// enrichReworkRequestFromWriteback 用 coordinator 自建 MR 的写回请求补齐
// 评论载荷缺失的分支信息(Gitea issue_comment 不携带 MR 元数据);
// writeback 缺失或请求载荷不可解析时保持原请求。
func enrichReworkRequestFromWriteback(request model.MergeRequestReworkRequest, writeback *model.Writeback) model.MergeRequestReworkRequest {
	if writeback == nil || strings.TrimSpace(writeback.RequestJSON) == "" {
		return request
	}
	var intent struct {
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
	}
	if json.Unmarshal([]byte(writeback.RequestJSON), &intent) != nil {
		return request
	}
	if strings.TrimSpace(request.SourceBranch) == "" {
		request.SourceBranch = strings.TrimSpace(intent.SourceBranch)
	}
	if strings.TrimSpace(request.TargetBranch) == "" {
		request.TargetBranch = strings.TrimSpace(intent.TargetBranch)
	}
	return request
}

func validateMergeRequestReworkRequest(request model.MergeRequestReworkRequest) error {
	for name, value := range map[string]string{
		"dedupe key":        request.DedupeKey,
		"project id":        request.ProjectID,
		"project path":      request.ProjectPath,
		"repository url":    request.RepositoryURL,
		"merge request url": request.MergeRequestURL,
		"source branch":     request.SourceBranch,
		"target branch":     request.TargetBranch,
		"resume context":    request.ResumeContext,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("merge request rework %s is required", name)
		}
	}
	if request.MergeRequestIID <= 0 {
		return errors.New("merge request rework iid is required")
	}
	if request.NoteID <= 0 {
		return errors.New("merge request rework note id is required")
	}
	return nil
}

func mergeRequestReworkContext(origin *model.Task, request model.MergeRequestReworkRequest, latestReview *model.Writeback) (string, error) {
	contextValue := map[string]any{
		"project": map[string]any{
			"id":   request.ProjectID,
			"path": request.ProjectPath,
		},
		"merge_request": map[string]any{
			"iid":               request.MergeRequestIID,
			"url":               request.MergeRequestURL,
			"title":             request.MergeRequestTitle,
			"description":       request.MergeRequestDescription,
			"state":             "opened",
			"source_branch":     request.SourceBranch,
			"target_branch":     request.TargetBranch,
			"expected_head_sha": request.ExpectedHeadSHA,
			"assignee_ids":      append([]int(nil), request.AssigneeIDs...),
			"reviewer_ids":      append([]int(nil), request.ReviewerIDs...),
		},
		"rework_request": map[string]any{
			"note_id":       request.NoteID,
			"note_url":      request.NoteURL,
			"note_type":     request.NoteType,
			"discussion_id": request.DiscussionID,
			"context":       strings.TrimSpace(request.ResumeContext),
			"requester": map[string]any{
				"id":       request.RequesterID,
				"name":     request.RequesterName,
				"username": request.RequesterUsername,
				"email":    request.RequesterEmail,
			},
		},
		"repos": []map[string]any{{
			"name":   repoNameFromPath(request.ProjectPath),
			"url":    request.RepositoryURL,
			"branch": request.TargetBranch,
			"access": "writable",
		}},
	}
	if origin != nil {
		originIssue, err := originIssueContext(origin)
		if err != nil {
			return "", err
		}
		contextValue["origin_issue"] = originIssue
	}
	if latestReview != nil {
		review, err := reviewContext(latestReview)
		if err == nil {
			contextValue["latest_review"] = review
		}
	}
	raw, err := json.Marshal(contextValue)
	if err != nil {
		return "", fmt.Errorf("encode merge request rework context: %w", err)
	}
	return string(raw), nil
}

func originIssueContext(origin *model.Task) (map[string]any, error) {
	var originContext map[string]any
	if err := json.Unmarshal([]byte(origin.ContextJSON), &originContext); err != nil {
		return nil, fmt.Errorf("decode origin developer task context: %w", err)
	}
	issue, ok := originContext["issue"].(map[string]any)
	if !ok || reworkNumberField(issue, "iid") <= 0 {
		return nil, errors.New("origin developer task issue context is required")
	}
	originIssue := make(map[string]any, len(issue)+2)
	for key, value := range issue {
		originIssue[key] = value
	}
	originIssue["objective"] = origin.Objective
	originIssue["acceptance_criteria"] = origin.AcceptanceCriteria
	return originIssue, nil
}

func originIssueAssigneeAllowed(origin *model.Task, requesterID int) bool {
	if requesterID <= 0 || origin == nil {
		return false
	}
	var context map[string]any
	if json.Unmarshal([]byte(origin.ContextJSON), &context) != nil {
		return false
	}
	issue, ok := context["issue"].(map[string]any)
	if !ok {
		return false
	}
	for _, assigneeID := range issueAssigneeIDs(issue["assignee_ids"]) {
		if assigneeID == requesterID {
			return true
		}
	}
	return false
}

func issueAssigneeIDs(value any) []int {
	switch values := value.(type) {
	case []int:
		return append([]int(nil), values...)
	case []any:
		ids := make([]int, 0, len(values))
		for _, raw := range values {
			switch number := raw.(type) {
			case int:
				if number > 0 {
					ids = append(ids, number)
				}
			case int64:
				if number > 0 {
					ids = append(ids, int(number))
				}
			case float64:
				if number > 0 && number == float64(int(number)) {
					ids = append(ids, int(number))
				}
			}
		}
		return ids
	default:
		return nil
	}
}

func (o *Orchestrator) latestReviewerComment(ctx context.Context, projectPath string, mergeRequestIID int) (*model.Writeback, error) {
	if o.writebacks == nil {
		return nil, repository.ErrNotFound
	}
	return o.writebacks.GetLatestSucceededReviewerComment(ctx, projectPath, mergeRequestIID)
}

func reviewContext(writeback *model.Writeback) (map[string]any, error) {
	if writeback == nil {
		return nil, errors.New("latest reviewer comment is nil")
	}
	var request struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal([]byte(writeback.RequestJSON), &request); err != nil {
		return nil, fmt.Errorf("decode latest reviewer comment: %w", err)
	}
	body := strings.TrimSpace(request.Body)
	marker := strings.TrimSpace(writeback.OperationMarker)
	if marker == "" || !strings.Contains(body, marker) {
		return nil, errors.New("latest reviewer comment marker is missing from body")
	}
	body = strings.TrimSpace(strings.ReplaceAll(body, marker, ""))
	if body == "" {
		return nil, errors.New("latest reviewer comment body is empty")
	}
	return map[string]any{
		"task_id":      writeback.TaskID,
		"run_id":       writeback.RunID,
		"note_id":      writeback.ExternalID,
		"comment_body": body,
	}, nil
}

func repoNameFromPath(projectPath string) string {
	parts := strings.Split(strings.Trim(projectPath, "/"), "/")
	return parts[len(parts)-1]
}

// reworkMergeRequestFetchTimeout 限制 rework 分支补链的出网调用:webhook 处理
// 在请求栈内同步执行,不能让平台 API 抖动拖住整个入站链路。
const reworkMergeRequestFetchTimeout = 10 * time.Second

// enrichReworkRequestFromPlatform 补写回记录也提供不了的信息。GitHub 的
// issue_comment(PR)载荷不携带 head/base/sha(平台设计差异),外部 PR 没有写回
// 记录,coordinator 自建 PR 的写回 intent 也不含 sha(创建请求非响应,评论时刻
// 的当前 head 只能回源平台取)。仅 GitHub 且仅补缺(不覆盖非空字段),分支与
// sha 全齐才免出网;Gitea 载荷直读路径零额外出网。API 失败 fail-closed:返回
// 错误让请求照旧被拒(inbound event 留痕),不静默放行、不重试。
func (o *Orchestrator) enrichReworkRequestFromPlatform(ctx context.Context, request model.MergeRequestReworkRequest) (model.MergeRequestReworkRequest, error) {
	if !strings.EqualFold(strings.TrimSpace(request.Source), "github") {
		return request, nil
	}
	if strings.TrimSpace(request.SourceBranch) != "" && strings.TrimSpace(request.TargetBranch) != "" && strings.TrimSpace(request.ExpectedHeadSHA) != "" {
		return request, nil
	}
	if o.mergeRequestReaderResolver == nil {
		return request, errors.New("merge request reader resolver is required to enrich github rework branches")
	}
	if request.RepositoryID == nil || strings.TrimSpace(*request.RepositoryID) == "" {
		return request, errors.New("repository id is required to enrich github rework branches")
	}
	reader, err := o.mergeRequestReaderResolver.ResolveMergeRequestReader(ctx, *request.RepositoryID)
	if err != nil {
		return request, fmt.Errorf("resolve github client for rework enrichment: %w", err)
	}
	fetchCtx, cancel := context.WithTimeout(ctx, o.reworkMergeRequestFetchTimeout)
	defer cancel()
	mergeRequest, err := reader.GetMergeRequest(fetchCtx, request.ProjectPath, request.MergeRequestIID)
	if err != nil {
		return request, fmt.Errorf("fetch github pull request %s#%d for rework enrichment: %w", request.ProjectPath, request.MergeRequestIID, err)
	}
	if strings.TrimSpace(request.SourceBranch) == "" {
		request.SourceBranch = strings.TrimSpace(mergeRequest.SourceBranch)
	}
	if strings.TrimSpace(request.TargetBranch) == "" {
		request.TargetBranch = strings.TrimSpace(mergeRequest.TargetBranch)
	}
	if strings.TrimSpace(request.ExpectedHeadSHA) == "" {
		request.ExpectedHeadSHA = strings.TrimSpace(mergeRequest.HeadSHA)
	}
	return request, nil
}

func normalizedReworkProjectPath(value string) string {
	return strings.Trim(strings.TrimSpace(value), "/")
}

func reworkNumberField(object map[string]any, key string) int {
	switch value := object[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}
