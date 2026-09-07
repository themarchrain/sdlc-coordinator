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

const developerClarificationEffectKind = "send_im_message"

func (o *Orchestrator) withDeveloperClarificationFollowups(task *model.Task, payload results.CanonicalResult) (results.CanonicalResult, error) {
	if task == nil || task.Role != "DeveloperWorker" || task.Type == model.TaskTypeReworkMergeRequest || payload.Status != results.StatusHumanRequired {
		return payload, nil
	}

	issue, err := developerIssueContext(task.ContextJSON)
	if err != nil {
		return results.CanonicalResult{}, err
	}
	projectID := strings.TrimSpace(task.TargetRepo)
	if projectID == "" {
		return results.CanonicalResult{}, errors.New("developer clarification target repository is required")
	}
	body, err := developerClarificationBody(payload)
	if err != nil {
		return results.CanonicalResult{}, err
	}

	payload.Writebacks = append(payload.Writebacks, results.CodeHostWritebackIntent{
		DedupeKey: fmt.Sprintf("developer-clarification:%s:%s", payload.TaskID, payload.RunID),
		Kind:      CodeHostWritebackCreateIssueNote,
		ProjectID: projectID,
		IssueIID:  issue.IID,
		Body:      body,
	})

	// mentions gate 翻转:通知恒发,收件人解析不出时退化为无 @ 的纯文本,
	// 人员目录缺席不再吞掉整条通知(缺席降级语义)。
	mentions := o.assigneeMentionsFor(context.Background(), taskRepositoryID(task), issue.AssigneeIDs)
	// IM 专属的链接拼接放在 effect 组装之前：即使未来该函数引入错误路径，
	// 也不会阻塞 GitLab 评论写回。
	imBody, err := developerClarificationIMBody(issue, body)
	if err != nil {
		return results.CanonicalResult{}, err
	}
	payload.Effects = append(payload.Effects, results.EffectIntent{
		DedupeKey: fmt.Sprintf("developer-clarification-im:%s:%s", payload.TaskID, payload.RunID),
		Kind:      developerClarificationEffectKind,
		Target:    o.developerIMTarget,
		Request: map[string]any{
			"body":     imBody,
			"mentions": mentions,
			"format":   "markdown",
		},
	})
	return payload, nil
}

func (o *Orchestrator) withDeveloperDeliveryFollowups(ctx context.Context, task *model.Task, payload results.CanonicalResult) (results.CanonicalResult, error) {
	if task == nil || task.Role != "DeveloperWorker" || task.Type == model.TaskTypeReworkMergeRequest || payload.Status != results.StatusSucceeded || o.writebacks == nil {
		return payload, nil
	}

	issue, err := developerIssueContext(task.ContextJSON)
	if err != nil {
		return payload, nil
	}
	// mentions gate 翻转:交付通知恒发,mentions 为空时退化为无 @ 的纯文本。
	mentions := o.assigneeMentionsFor(ctx, taskRepositoryID(task), issue.AssigneeIDs)

	writebacks, err := BuildCodeHostWritebacksFromResult(payload)
	if err != nil {
		return results.CanonicalResult{}, err
	}
	for _, desired := range writebacks {
		if desired.Kind != CodeHostWritebackCreateMergeRequest {
			continue
		}
		writeback, err := o.writebacks.GetByDedupeKey(ctx, desired.DedupeKey)
		if errors.Is(err, repository.ErrNotFound) {
			continue
		}
		if err != nil {
			return results.CanonicalResult{}, err
		}
		if writeback.Status != model.WritebackStatusSucceeded || strings.TrimSpace(writeback.ExternalURL) == "" {
			continue
		}
		intent, err := decodeCodeHostWritebackIntent(writeback)
		if err != nil {
			return results.CanonicalResult{}, err
		}
		body, err := developerDeliveryBody(issue, intent.Title, writeback.ExternalURL, payload.Summary)
		if err != nil {
			return results.CanonicalResult{}, err
		}
		payload.Effects = append(payload.Effects, results.EffectIntent{
			DedupeKey: fmt.Sprintf("developer-delivery-im:%s:%s", payload.TaskID, payload.RunID),
			Kind:      developerClarificationEffectKind,
			Target:    o.developerIMTarget,
			Critical:  false,
			Request: map[string]any{
				"body":     body,
				"mentions": mentions,
				"format":   "markdown",
			},
		})
		return payload, nil
	}
	return payload, nil
}

type developerIssue struct {
	IID         int    `json:"iid"`
	URL         string `json:"url"`
	AssigneeIDs []int  `json:"assignee_ids"`
}

func developerIssueContext(raw string) (developerIssue, error) {
	var context struct {
		Issue developerIssue `json:"issue"`
	}
	if err := json.Unmarshal([]byte(raw), &context); err != nil {
		return developerIssue{}, fmt.Errorf("decode developer task context: %w", err)
	}
	if context.Issue.IID <= 0 {
		return developerIssue{}, errors.New("developer task issue iid is required")
	}
	return context.Issue, nil
}

func developerClarificationBody(payload results.CanonicalResult) (string, error) {
	reason := ""
	if payload.BlockerReason != nil {
		reason = strings.TrimSpace(*payload.BlockerReason)
	}
	if reason == "" {
		reason = strings.TrimSpace(payload.Summary)
	}
	if reason == "" {
		return "", errors.New("developer clarification reason is required")
	}
	return "Developer Agent 需要补充以下需求上下文后才能继续：\n\n" + reason + "\n\n请在本 Issue 下新建评论，直接补充必要信息即可。", nil
}

// developerClarificationIMBody 在澄清评论正文基础上，为 IM 消息追加可点击的
// Issue 链接行（紧跟首行之后，引用块样式）。链接由本地任务上下文拼接，
// 不经过 AI；URL 缺失时降级为裸 IID。
func developerClarificationIMBody(issue developerIssue, body string) (string, error) {
	if strings.TrimSpace(body) == "" {
		return "", errors.New("developer clarification IM body requires base body")
	}
	issueReference := fmt.Sprintf("#%d", issue.IID)
	if issueURL := strings.TrimSpace(issue.URL); issueURL != "" {
		issueReference = fmt.Sprintf("[#%d](%s)", issue.IID, issueURL)
	}
	parts := strings.SplitN(strings.TrimSpace(body), "\n\n", 2)
	imBody := parts[0] + "\n\n> - Issue: " + issueReference
	if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
		imBody += "\n\n" + strings.TrimSpace(parts[1])
	}
	return imBody, nil
}

func developerDeliveryBody(issue developerIssue, mergeRequestTitle, mergeRequestURL, summary string) (string, error) {
	if issue.IID <= 0 {
		return "", errors.New("developer delivery issue iid is required")
	}
	if strings.TrimSpace(mergeRequestURL) == "" {
		return "", errors.New("developer delivery merge request url is required")
	}
	if strings.TrimSpace(mergeRequestTitle) == "" {
		mergeRequestTitle = "Developer Agent 创建的合并请求"
	}
	if strings.TrimSpace(summary) == "" {
		summary = "Developer Agent 已完成开发并提交合并请求。"
	}
	issueReference := fmt.Sprintf("#%d", issue.IID)
	if issueURL := strings.TrimSpace(issue.URL); issueURL != "" {
		issueReference = fmt.Sprintf("[#%d](%s)", issue.IID, issueURL)
	}
	return fmt.Sprintf(
		"Developer Agent 已完成开发并创建合并请求：\n\n- Issue: %s\n- MR: [%s](%s)\n- 摘要: %s\n\nReviewer Agent 即将进行代码审查，稍后可点击 MR 查看详情。",
		issueReference,
		strings.TrimSpace(mergeRequestTitle),
		strings.TrimSpace(mergeRequestURL),
		strings.TrimSpace(summary),
	), nil
}
