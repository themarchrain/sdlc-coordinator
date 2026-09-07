package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/model"
)

type TaskFactory struct{}

func NewTaskFactory() TaskFactory {
	return TaskFactory{}
}

func (TaskFactory) BuildTask(event NormalizedEvent) (*model.Task, error) {
	if err := validateTaskEvent(event); err != nil {
		return nil, err
	}

	switch event.Kind {
	case EventKindChangeRequestUpdated:
		return buildReviewMRTask(event)
	case EventKindWorkItemUpdated:
		return buildDevelopFromIssueTask(event)
	default:
		return nil, fmt.Errorf("unsupported event kind: %s", event.Kind)
	}
}

func buildReviewMRTask(event NormalizedEvent) (*model.Task, error) {
	if strings.TrimSpace(event.HeadSHA) == "" {
		return nil, errors.New("merge request head sha is required")
	}
	if strings.TrimSpace(event.SourceBranch) == "" {
		return nil, errors.New("merge request source branch is required")
	}

	reviewMode := "full"
	if strings.EqualFold(strings.TrimSpace(event.EventAction), "update") && strings.TrimSpace(event.BaseSHA) != "" {
		reviewMode = "incremental"
	}
	context, err := marshalTaskContext(map[string]any{
		"event": event,
		"project": map[string]any{
			"id":   event.ProjectID,
			"path": event.ProjectPath,
		},
		"merge_request": map[string]any{
			"iid":           event.ObjectIID,
			"url":           event.ObjectURL,
			"title":         event.Title,
			"description":   event.Description,
			"event_action":  event.EventAction,
			"review_mode":   reviewMode,
			"source_branch": event.SourceBranch,
			"target_branch": event.TargetBranch,
			"head_sha":      event.HeadSHA,
			"base_sha":      event.BaseSHA,
			"author": map[string]any{
				"name":     stringMetadata(event.Metadata, "author_name"),
				"username": stringMetadata(event.Metadata, "author_username"),
				"email":    stringMetadata(event.Metadata, "author_email"),
			},
		},
		"repos": []map[string]any{
			{
				"name":   repoNameFromPath(event.ProjectPath),
				"url":    event.RepositoryURL,
				"branch": event.SourceBranch,
				"access": "readable",
			},
		},
	})
	if err != nil {
		return nil, err
	}

	return &model.Task{
		DedupeKey:          event.DedupeKey,
		Type:               "review_mr",
		Role:               "ReviewerWorker",
		SourceRepo:         event.ProjectPath,
		TargetRepo:         event.ProjectPath,
		Objective:          fmt.Sprintf("Review %s MR !%d in %s at head %s.", platformDisplayName(event.Source), event.ObjectIID, event.ProjectPath, event.HeadSHA),
		AcceptanceCriteria: "输出结构化 review verdict、主要风险、测试结论和需要写回 MR 的评论意图。",
		ContextJSON:        context,
	}, nil
}

func stringMetadata(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}

func buildDevelopFromIssueTask(event NormalizedEvent) (*model.Task, error) {
	branch := strings.TrimSpace(event.TargetBranch)
	if branch == "" {
		branch = "master"
	}

	issueContext := map[string]any{
		"iid":          event.ObjectIID,
		"url":          event.ObjectURL,
		"title":        event.Title,
		"description":  event.Description,
		"version":      event.Version,
		"assignee_ids": gitLabAssigneeIDs(event.Metadata),
		"assignees":    metadataAssignees(event.Metadata),
	}

	context, err := marshalTaskContext(map[string]any{
		"event": event,
		"project": map[string]any{
			"id":   event.ProjectID,
			"path": event.ProjectPath,
		},
		"issue": issueContext,
		"repos": []map[string]any{
			{
				"name":   repoNameFromPath(event.ProjectPath),
				"url":    event.RepositoryURL,
				"branch": branch,
				"access": "writable",
			},
		},
	})
	if err != nil {
		return nil, err
	}

	return &model.Task{
		DedupeKey:          event.DedupeKey,
		Type:               "develop_from_issue",
		Role:               "DeveloperWorker",
		SourceRepo:         event.ProjectPath,
		TargetRepo:         event.ProjectPath,
		Objective:          fmt.Sprintf("Implement %s Issue #%d in %s: %s", platformDisplayName(event.Source), event.ObjectIID, event.ProjectPath, event.Title),
		AcceptanceCriteria: "完成代码修改、运行相关测试，并通过结构化结果输出 MR 创建意图、验证结果和必要写回意图。",
		ContextJSON:        context,
	}, nil
}

func metadataAssignees(metadata map[string]any) any {
	if metadata == nil {
		return []map[string]any{}
	}
	if assignees, ok := metadata["assignees"]; ok {
		return assignees
	}
	return []map[string]any{}
}

func gitLabAssigneeIDs(metadata map[string]any) []int {
	if metadata == nil {
		return nil
	}
	value, ok := metadata["assignee_ids"]
	if !ok {
		return nil
	}

	switch assigneeIDs := value.(type) {
	case []int:
		return append([]int(nil), assigneeIDs...)
	case []any:
		result := make([]int, 0, len(assigneeIDs))
		for _, value := range assigneeIDs {
			switch id := value.(type) {
			case int:
				if id > 0 {
					result = append(result, id)
				}
			case float64:
				if id > 0 && id == float64(int(id)) {
					result = append(result, int(id))
				}
			}
		}
		return result
	default:
		return nil
	}
}

func platformDisplayName(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "gitea":
		return "Gitea"
	case "github":
		return "GitHub"
	default:
		return "GitLab"
	}
}

func validateTaskEvent(event NormalizedEvent) error {
	if strings.TrimSpace(event.DedupeKey) == "" {
		return errors.New("event dedupe key is required")
	}
	if strings.TrimSpace(event.ProjectPath) == "" {
		return errors.New("event project path is required")
	}
	if strings.TrimSpace(event.RepositoryURL) == "" {
		return errors.New("event repository url is required")
	}
	if event.ObjectIID <= 0 {
		return errors.New("event object iid is required")
	}
	return nil
}

func marshalTaskContext(context map[string]any) (string, error) {
	raw, err := json.Marshal(context)
	if err != nil {
		return "", fmt.Errorf("marshal task context: %w", err)
	}
	return string(raw), nil
}

func repoNameFromPath(projectPath string) string {
	parts := strings.Split(strings.Trim(projectPath, "/"), "/")
	if len(parts) == 0 {
		return projectPath
	}
	return parts[len(parts)-1]
}
