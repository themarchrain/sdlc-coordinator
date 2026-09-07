package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
)

type developerReworkContext struct {
	OriginIssue struct {
		IID         int    `json:"iid"`
		URL         string `json:"url"`
		AssigneeIDs []int  `json:"assignee_ids"`
	} `json:"origin_issue"`
	MergeRequest struct {
		IID          int    `json:"iid"`
		URL          string `json:"url"`
		SourceBranch string `json:"source_branch"`
		AssigneeIDs  []int  `json:"assignee_ids"`
		ReviewerIDs  []int  `json:"reviewer_ids"`
	} `json:"merge_request"`
	ReworkRequest struct {
		Requester struct {
			ID       int    `json:"id"`
			Name     string `json:"name"`
			Username string `json:"username"`
			Email    string `json:"email"`
		} `json:"requester"`
	} `json:"rework_request"`
}

func (o *Orchestrator) withDeveloperReworkFollowups(ctx context.Context, task *model.Task, payload results.CanonicalResult) (results.CanonicalResult, error) {
	if task == nil || task.Role != "DeveloperWorker" || task.Type != model.TaskTypeReworkMergeRequest {
		return payload, nil
	}
	contextValue, err := decodeDeveloperReworkContext(task.ContextJSON)
	if err != nil {
		return results.CanonicalResult{}, err
	}
	mentions := o.developerReworkMentions(ctx, task, contextValue)

	switch payload.Status {
	case results.StatusSucceeded:
		body, err := developerReworkSuccessBody(contextValue, payload)
		if err != nil {
			return results.CanonicalResult{}, err
		}
		payload.Effects = append(payload.Effects, results.EffectIntent{
			DedupeKey: fmt.Sprintf("developer-rework-im:%s:%s", payload.TaskID, payload.RunID),
			Kind:      "send_im_message",
			Target:    o.developerIMTarget,
			Critical:  false,
			Request: map[string]any{
				"body":     body,
				"mentions": mentions,
				"format":   "markdown",
			},
		})
	case results.StatusHumanRequired:
		body, err := developerReworkClarificationBody(payload)
		if err != nil {
			return results.CanonicalResult{}, err
		}
		payload.Writebacks = append(payload.Writebacks, results.CodeHostWritebackIntent{
			DedupeKey:       fmt.Sprintf("developer-rework-clarification:%s:%s", payload.TaskID, payload.RunID),
			Kind:            CodeHostWritebackCreateMergeRequestNote,
			ProjectID:       task.TargetRepo,
			MergeRequestIID: contextValue.MergeRequest.IID,
			Body:            body,
		})
		// IM 专属的链接拼接放在评论写回之后：即使未来该函数引入错误路径，
		// 也不会阻塞 GitLab 评论写回。
		imBody, err := developerReworkClarificationIMBody(contextValue, body)
		if err != nil {
			return results.CanonicalResult{}, err
		}
		payload.Effects = append(payload.Effects, results.EffectIntent{
			DedupeKey: fmt.Sprintf("developer-rework-clarification-im:%s:%s", payload.TaskID, payload.RunID),
			Kind:      "send_im_message",
			Target:    o.developerIMTarget,
			Critical:  false,
			Request: map[string]any{
				"body":     imBody,
				"mentions": mentions,
				"format":   "markdown",
			},
		})
	}
	return payload, nil
}

func decodeDeveloperReworkContext(raw string) (developerReworkContext, error) {
	var contextValue developerReworkContext
	if err := json.Unmarshal([]byte(raw), &contextValue); err != nil {
		return developerReworkContext{}, fmt.Errorf("decode developer rework task context: %w", err)
	}
	if contextValue.OriginIssue.IID <= 0 || contextValue.MergeRequest.IID <= 0 || strings.TrimSpace(contextValue.MergeRequest.URL) == "" || strings.TrimSpace(contextValue.MergeRequest.SourceBranch) == "" {
		return developerReworkContext{}, errors.New("developer rework origin issue and merge request context are required")
	}
	return contextValue, nil
}

func (o *Orchestrator) developerReworkMentions(ctx context.Context, task *model.Task, contextValue developerReworkContext) []string {
	ids := append([]int(nil), contextValue.OriginIssue.AssigneeIDs...)
	ids = append(ids, contextValue.MergeRequest.AssigneeIDs...)
	ids = append(ids, contextValue.MergeRequest.ReviewerIDs...)
	mentions := o.assigneeMentionsFor(ctx, taskRepositoryID(task), ids)
	// 请求者提及键与 mentions 契约对齐:显示名 → username → 邮箱兜底。
	requesterMention := firstNonEmptyMention(
		contextValue.ReworkRequest.Requester.Name,
		contextValue.ReworkRequest.Requester.Username,
		contextValue.ReworkRequest.Requester.Email,
	)
	if requesterMention != "" {
		mentions = append(mentions, requesterMention)
	}
	return normalizeIMMentions(mentions)
}

func firstNonEmptyMention(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func developerReworkSuccessBody(contextValue developerReworkContext, payload results.CanonicalResult) (string, error) {
	commitSHA := artifactString(payload.Artifacts, "commit_sha")
	testSummary := developerReworkTestSummary(payload.Artifacts)
	if commitSHA == "" || testSummary == "" || strings.TrimSpace(payload.Summary) == "" {
		return "", errors.New("developer rework IM requires commit sha, test results, and summary")
	}
	return fmt.Sprintf(
		"Developer Agent 已完成 MR 接续开发：\n\n- MR: [!%d](%s)\n- 源分支: `%s`\n- 新提交: `%s`\n- 修改摘要: %s\n- 验证结果: %s\n\n原 MR 源分支已更新，将进入增量代码审查。",
		contextValue.MergeRequest.IID,
		strings.TrimSpace(contextValue.MergeRequest.URL),
		strings.TrimSpace(contextValue.MergeRequest.SourceBranch),
		shortSHA(commitSHA),
		strings.TrimSpace(payload.Summary),
		testSummary,
	), nil
}

func developerReworkTestSummary(artifacts map[string]any) string {
	if artifacts == nil || artifacts["test_results"] == nil {
		return ""
	}
	raw, err := json.Marshal(artifacts["test_results"])
	if err != nil {
		return ""
	}
	var tests []results.TestEvidence
	if err := json.Unmarshal(raw, &tests); err != nil || len(tests) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tests))
	for _, test := range tests {
		parts = append(parts, fmt.Sprintf("`%s`: %s", strings.TrimSpace(test.Command), strings.TrimSpace(test.Summary)))
	}
	return strings.Join(parts, "；")
}

func developerReworkClarificationBody(payload results.CanonicalResult) (string, error) {
	reason := ""
	if payload.BlockerReason != nil {
		reason = strings.TrimSpace(*payload.BlockerReason)
	}
	if reason == "" {
		reason = strings.TrimSpace(payload.Summary)
	}
	if reason == "" {
		return "", errors.New("developer rework clarification reason is required")
	}
	return "Developer Agent 在本 MR 接续开发中需要补充以下信息：\n\n" + reason + "\n\n请在本 MR 下新建评论，直接补充必要信息即可。", nil
}

// developerReworkClarificationIMBody 在澄清评论正文基础上，为 IM 消息追加
// MR 与原 Issue 的可点击链接行（引用块样式，紧跟首行之后）。链接由本地
// 任务上下文拼接，不经过 AI；URL 缺失时降级为裸 IID。
func developerReworkClarificationIMBody(contextValue developerReworkContext, body string) (string, error) {
	if strings.TrimSpace(body) == "" {
		return "", errors.New("developer rework clarification IM body requires base body")
	}
	// decodeDeveloperReworkContext 已强校验 MR URL 非空，此降级分支不可达，
	// 仅作为防御性兜底保留。
	mrReference := fmt.Sprintf("!%d", contextValue.MergeRequest.IID)
	if mrURL := strings.TrimSpace(contextValue.MergeRequest.URL); mrURL != "" {
		mrReference = fmt.Sprintf("[!%d](%s)", contextValue.MergeRequest.IID, mrURL)
	}
	issueReference := fmt.Sprintf("#%d", contextValue.OriginIssue.IID)
	if issueURL := strings.TrimSpace(contextValue.OriginIssue.URL); issueURL != "" {
		issueReference = fmt.Sprintf("[#%d](%s)", contextValue.OriginIssue.IID, issueURL)
	}
	parts := strings.SplitN(strings.TrimSpace(body), "\n\n", 2)
	imBody := parts[0] + "\n\n> - MR: " + mrReference + "\n> - Issue: " + issueReference
	if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
		imBody += "\n\n" + strings.TrimSpace(parts[1])
	}
	return imBody, nil
}
