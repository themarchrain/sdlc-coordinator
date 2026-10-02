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
// 证据来源必须与验证任务同仓:同租户内不同仓库可存在同名交付分支,分支
// 相等不构成同仓证据,错仓命令混入处方会让验证依据错配。
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
	// id 降序,最新任务优先;只认同仓的 developer 任务,防同分支异物混入
	// 处方。错仓候选只跳过不抢占:反查继续向更早候选进行,同仓最新证据
	// 仍然生效。
	for _, candidate := range candidates {
		if candidate.Role != "DeveloperWorker" {
			continue
		}
		if !sameRepositoryEvidence(&candidate, task) {
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

// sameRepositoryEvidence 判定复放候选与验证任务是否同仓。双侧注册仓 ID
// 都存在时严格相等;任一侧缺失(历史任务)退回仓库路径身份相等——与验证
// 任务去重键同一身份口径,绝非分支名相等;路径也缺失即视为未证实同仓,
// 宁可损失复放覆盖(下限兜底),不引入跨仓命令。
func sameRepositoryEvidence(candidate, task *model.Task) bool {
	if candidate == nil || task == nil {
		return false
	}
	if candidate.RepositoryID != nil && task.RepositoryID != nil {
		return strings.TrimSpace(*candidate.RepositoryID) == strings.TrimSpace(*task.RepositoryID)
	}
	candidateRepo := strings.TrimSpace(candidate.TargetRepo)
	taskRepo := strings.TrimSpace(task.TargetRepo)
	return candidateRepo != "" && candidateRepo == taskRepo
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
// 清单。developer 的失败轮(fix_failed 等)会携带红测试入 artifacts,复放只取
// 通过记录——它们才是开发者作为交付依据登记的验证。
func decodeTestResultCommands(metadataJSON string) []string {
	var metadata struct {
		Value []results.TestEvidence `json:"value"`
	}
	if strings.TrimSpace(metadataJSON) == "" || json.Unmarshal([]byte(metadataJSON), &metadata) != nil {
		return nil
	}
	commands := make([]string, 0, len(metadata.Value))
	for _, evidence := range metadata.Value {
		if !evidence.Passed {
			continue
		}
		command := strings.TrimSpace(evidence.Command)
		if command == "" {
			continue
		}
		commands = append(commands, command)
	}
	return commands
}
