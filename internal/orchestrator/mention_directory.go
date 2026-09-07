package orchestrator

import (
	"context"
	"strings"

	"sdlc-coordinator/internal/model"
)

// AssigneeMention 是 IM 提及渲染所需的单个收件人解析结果。
type AssigneeMention struct {
	DisplayName string // 人员显示名;缺席时回退平台 username
	Username    string // 平台用户名(交付分支命名兜底)
}

// AssigneeMentionDirectory 把平台 assignee ID 按仓作用域解析为提及材料:人员
// 目录的查询作用域(provider/base_url)由任务所属仓行决定,跨平台同 id 互不串。
// 人员目录缺席时相关功能降级(mention 退化为纯文本、分支命名退化为默认段),
// 不得硬依赖报错。
type AssigneeMentionDirectory interface {
	ResolveMentionNames(ctx context.Context, repositoryID string, assigneeIDs []int) map[int]AssigneeMention
}

func WithAssigneeMentionDirectory(directory AssigneeMentionDirectory) Option {
	return func(o *Orchestrator) {
		o.assigneeMentionDirectory = directory
	}
}

// taskRepositoryID 取任务绑定的注册仓 ID;未绑定时返回空串(解析方按缺席降级)。
func taskRepositoryID(task *model.Task) string {
	if task == nil || task.RepositoryID == nil {
		return ""
	}
	return strings.TrimSpace(*task.RepositoryID)
}

// assigneeMentionsFor 解析指派人为 IM 提及键:显示名优先,username 兜底,去重保序。
// 提及键来自人员身份目录;解析不出即返回空,
// 由 effect 的 mentions gate 之后的链路决定降级形态。
func (o *Orchestrator) assigneeMentionsFor(ctx context.Context, repositoryID string, assigneeIDs []int) []string {
	if o.assigneeMentionDirectory == nil || len(assigneeIDs) == 0 {
		return nil
	}
	resolved := o.assigneeMentionDirectory.ResolveMentionNames(ctx, repositoryID, assigneeIDs)
	mentions := make([]string, 0, len(assigneeIDs))
	seen := make(map[string]struct{}, len(assigneeIDs))
	for _, id := range assigneeIDs {
		mention := resolved[id]
		name := strings.TrimSpace(mention.DisplayName)
		if name == "" {
			name = strings.TrimSpace(mention.Username)
		}
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		mentions = append(mentions, name)
	}
	return mentions
}

// assigneeBranchUsernamesFor 解析指派人的平台用户名,供交付分支命名兜底。
func (o *Orchestrator) assigneeBranchUsernamesFor(ctx context.Context, repositoryID string, assigneeIDs []int) []string {
	if o.assigneeMentionDirectory == nil || len(assigneeIDs) == 0 {
		return nil
	}
	resolved := o.assigneeMentionDirectory.ResolveMentionNames(ctx, repositoryID, assigneeIDs)
	usernames := make([]string, 0, len(assigneeIDs))
	for _, id := range assigneeIDs {
		if username := strings.TrimSpace(resolved[id].Username); username != "" {
			usernames = append(usernames, username)
		}
	}
	return usernames
}
