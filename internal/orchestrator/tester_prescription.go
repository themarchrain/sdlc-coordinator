package orchestrator

import (
	"context"
	"encoding/json"
	"strings"

	"sdlc-coordinator/internal/agent"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/results"
)

// buildVerificationPlan 组装验证处方 = 下限 ∪ 复放(去重保序,floor 在前)。
// 下限是注册仓声明的验证命令,靠它堵住"开发者少跑、复放就跟着少跑"的放水洞;
// 复放是从开发记录反查的测试命令,属增强面:任何一步查不到或读失败都降级为
// 空清单,只损失覆盖广度,不损失下限,也不阻塞派发。
func (o *Orchestrator) buildVerificationPlan(ctx context.Context, task *model.Task, repoRow *registeredrepo.RegisteredRepository) *agent.VerificationPlan {
	plan := &agent.VerificationPlan{FloorCommand: strings.TrimSpace(repoRow.VerifyCommand)}
	seen := map[string]struct{}{}
	if plan.FloorCommand != "" {
		seen[plan.FloorCommand] = struct{}{}
	}
	for _, command := range o.replayCommandsFromDeveloperTask(ctx, task) {
		if _, duplicate := seen[command]; duplicate {
			continue
		}
		seen[command] = struct{}{}
		plan.ReplayCommands = append(plan.ReplayCommands, command)
	}
	if plan.FloorCommand == "" && len(plan.ReplayCommands) == 0 {
		return nil
	}
	return plan
}

// replayCommandsFromDeveloperTask 反查产生该 MR 源分支的开发任务并提取其
// 最新一轮记录的测试命令。链接口径:任务的 DeliveryBranch 与 MR 源分支
// 相等(主流程 develop_from_issue 的交付分支即 MR 源分支);接续开发
// (rework)轮次的交付分支不等于源分支,本期不参与复放,由下限兜底。
func (o *Orchestrator) replayCommandsFromDeveloperTask(ctx context.Context, task *model.Task) []string {
	if o.verificationArtifacts == nil {
		return nil
	}
	sourceBranch := testerSourceBranch(task)
	if sourceBranch == "" {
		return nil
	}
	candidates, err := o.tasks.ListByDeliveryBranch(ctx, sourceBranch)
	if err != nil {
		return nil
	}
	// id 降序,最新任务优先;只认 developer 任务,防同分支异物混入处方。
	for _, candidate := range candidates {
		if candidate.Role != "DeveloperWorker" {
			continue
		}
		artifacts, readErr := o.verificationArtifacts.ListByTaskAndName(ctx, candidate.TaskID, "test_result", "test_results")
		if readErr != nil || len(artifacts) == 0 {
			continue
		}
		commands := decodeTestResultCommands(artifacts[len(artifacts)-1].MetadataJSON)
		if len(commands) > 0 {
			return commands
		}
	}
	return nil
}

func testerSourceBranch(task *model.Task) string {
	if task == nil || strings.TrimSpace(task.ContextJSON) == "" {
		return ""
	}
	var parsed struct {
		MergeRequest struct {
			SourceBranch string `json:"source_branch"`
		} `json:"merge_request"`
	}
	if json.Unmarshal([]byte(task.ContextJSON), &parsed) != nil {
		return ""
	}
	return strings.TrimSpace(parsed.MergeRequest.SourceBranch)
}

// decodeTestResultCommands 从 test_results artifact 的 MetadataJSON 提取命令
// 清单。developer 契约只放行 passed=true 的条目,复放清单天然全绿。
func decodeTestResultCommands(metadataJSON string) []string {
	var metadata struct {
		Value []results.TestEvidence `json:"value"`
	}
	if strings.TrimSpace(metadataJSON) == "" || json.Unmarshal([]byte(metadataJSON), &metadata) != nil {
		return nil
	}
	commands := make([]string, 0, len(metadata.Value))
	for _, evidence := range metadata.Value {
		command := strings.TrimSpace(evidence.Command)
		if command == "" {
			continue
		}
		commands = append(commands, command)
	}
	return commands
}
