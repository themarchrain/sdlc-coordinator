package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/store/repository"
)

func (o *Orchestrator) withReviewerFollowups(ctx context.Context, task *model.Task, payload results.CanonicalResult) (results.CanonicalResult, error) {
	if task == nil || task.Role != "ReviewerWorker" || o.writebacks == nil {
		return payload, nil
	}

	desiredWritebacks, err := BuildCodeHostWritebacksFromResult(payload)
	if err != nil {
		return results.CanonicalResult{}, err
	}
	for _, desired := range desiredWritebacks {
		if desired.Kind != CodeHostWritebackCreateMergeRequestNote {
			continue
		}
		writeback, err := o.writebacks.GetByDedupeKey(ctx, desired.DedupeKey)
		if errors.Is(err, repository.ErrNotFound) {
			continue
		}
		if err != nil {
			return payload, nil
		}
		if writeback.Status != model.WritebackStatusSucceeded {
			continue
		}

		mergeRequest, err := reviewerMergeRequestContext(task.ContextJSON)
		if err != nil {
			return payload, nil
		}
		body, err := reviewerResultIMBody(mergeRequest, payload)
		if err != nil {
			return payload, nil
		}
		mentions := append([]string(nil), o.reviewerMentionsFor(ctx, task)...)
		mentions = append(mentions, o.reviewerDynamicMentions(ctx, task, mergeRequest)...)
		payload.Effects = append(payload.Effects, results.EffectIntent{
			DedupeKey: fmt.Sprintf("reviewer-result-im:%s:%s", payload.TaskID, payload.RunID),
			Kind:      "send_im_message",
			Target:    o.developerIMTarget,
			Critical:  false,
			Request: map[string]any{
				"body":     body,
				"mentions": normalizeIMMentions(mentions),
				"format":   "markdown",
			},
		})
		return payload, nil
	}
	return payload, nil
}

func (o *Orchestrator) reviewerDynamicMentions(ctx context.Context, task *model.Task, mr reviewerMergeRequest) []string {
	fallback := normalizeIMMentions([]string{mr.Author.Name})
	if o.writebacks == nil || o.tasks == nil || task == nil || mr.IID <= 0 {
		return fallback
	}
	projectID := strings.TrimSpace(task.TargetRepo)
	if projectID == "" {
		return fallback
	}
	writeback, err := o.writebacks.GetSucceededMergeRequest(ctx, projectID, mr.IID)
	if err != nil {
		return fallback
	}
	originTask, err := o.tasks.GetByTaskID(ctx, writeback.TaskID)
	if err != nil || originTask.Role != "DeveloperWorker" {
		return fallback
	}
	issue, err := developerIssueContext(originTask.ContextJSON)
	if err != nil {
		return fallback
	}
	mentions := o.assigneeMentionsFor(ctx, taskRepositoryID(originTask), issue.AssigneeIDs)
	if len(mentions) == 0 {
		return fallback
	}
	return mentions
}

type reviewerMergeRequest struct {
	IID        int    `json:"iid"`
	URL        string `json:"url"`
	Title      string `json:"title"`
	ReviewMode string `json:"review_mode"`
	HeadSHA    string `json:"head_sha"`
	BaseSHA    string `json:"base_sha"`
	Author     struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"author"`
}

func reviewerMergeRequestContext(raw string) (reviewerMergeRequest, error) {
	var context struct {
		MergeRequest reviewerMergeRequest `json:"merge_request"`
	}
	if err := json.Unmarshal([]byte(raw), &context); err != nil {
		return reviewerMergeRequest{}, fmt.Errorf("decode reviewer task context: %w", err)
	}
	mr := context.MergeRequest
	if mr.IID <= 0 || strings.TrimSpace(mr.URL) == "" || strings.TrimSpace(mr.HeadSHA) == "" {
		return reviewerMergeRequest{}, errors.New("reviewer task merge_request iid, url, and head_sha are required")
	}
	return mr, nil
}

func reviewerResultIMBody(mr reviewerMergeRequest, payload results.CanonicalResult) (string, error) {
	verdict := artifactString(payload.Artifacts, "review_verdict")
	testSummary := artifactString(payload.Artifacts, "review_test_summary")
	if verdict == "" || testSummary == "" || strings.TrimSpace(payload.Summary) == "" {
		return "", errors.New("reviewer IM requires verdict, test summary, and summary")
	}

	reviewType := "新建 MR 完整审查"
	base := "无"
	if strings.EqualFold(strings.TrimSpace(mr.ReviewMode), "incremental") {
		reviewType = "源分支新 Push 后增量审查"
		base = shortSHA(mr.BaseSHA)
	}
	title := strings.TrimSpace(mr.Title)
	if title == "" {
		title = fmt.Sprintf("MR !%d", mr.IID)
	}
	// IM 消息的收尾内容
	closing := "请进入 MR 查看完整审查评论，并由人工决定后续修改或合并。"
	if verdict == "no_new_changes" {
		closing = "本轮不产生新的合并建议；以最近一次实质审查结论为准。"
	}
	return fmt.Sprintf(
		"## Reviewer Agent 代码审查完成 — MR !%d\n\n- 类型：%s\n- MR：[%s](%s)\n- 当前审查提交：`%s`\n- 增量起点：`%s`\n- 审查结果：%s\n- 测试结果：%s\n- 摘要：%s\n\n%s",
		mr.IID,
		reviewType,
		title,
		strings.TrimSpace(mr.URL),
		shortSHA(mr.HeadSHA),
		base,
		reviewerVerdictLabel(verdict),
		testSummary,
		strings.TrimSpace(payload.Summary),
		closing,
	), nil
}

func reviewerVerdictLabel(verdict string) string {
	switch verdict {
	case "approved":
		return "✅ 可合并"
	case "changes_requested":
		return "❕ 暂不建议合并"
	case "human_required":
		return "❔ 需人工确认"
	case "no_new_changes":
		return "☑️ 本轮无新增可审内容"
	default:
		return verdict
	}
}

func artifactString(artifacts map[string]any, key string) string {
	if artifacts == nil {
		return ""
	}
	value, _ := artifacts[key].(string)
	return strings.TrimSpace(value)
}

func shortSHA(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 8 {
		return value
	}
	return value[:8]
}

func normalizeIMMentions(mentions []string) []string {
	result := make([]string, 0, len(mentions))
	seen := make(map[string]struct{}, len(mentions))
	for _, mention := range mentions {
		mention = strings.TrimSpace(mention)
		if mention == "" {
			continue
		}
		if _, exists := seen[mention]; exists {
			continue
		}
		seen[mention] = struct{}{}
		result = append(result, mention)
	}
	return result
}
