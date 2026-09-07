package workers

import (
	"encoding/json"
	"errors"

	"sdlc-coordinator/internal/agent"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
)

func BuildReviewerRunnerTaskBundle(task *model.Task, run *model.Run) (agent.TaskBundle, error) {
	pkg, err := BuildRunnerTaskPackage(task, run)
	if err != nil {
		return agent.TaskBundle{}, err
	}
	prompt, err := BuildReviewerRunnerPrompt(pkg)
	if err != nil {
		return agent.TaskBundle{}, err
	}
	return agent.TaskBundle{
		TaskPackage:  pkg,
		ResultSchema: results.NewReviewerContract().InputSchema(),
		Prompt:       prompt,
	}, nil
}

func BuildDeveloperRunnerTaskBundle(task *model.Task, run *model.Run) (agent.TaskBundle, error) {
	pkg, err := BuildRunnerTaskPackage(task, run)
	if err != nil {
		return agent.TaskBundle{}, err
	}
	prompt, err := BuildDeveloperRunnerPrompt(pkg)
	if err != nil {
		return agent.TaskBundle{}, err
	}
	return agent.TaskBundle{
		TaskPackage:  pkg,
		ResultSchema: results.NewDeveloperAgentContract().InputSchema(),
		Prompt:       prompt,
	}, nil
}

func BuildDeveloperRunnerPrompt(pkg agent.TaskPackage) (string, error) {
	if pkg.Role != "DeveloperWorker" {
		return "", errors.New("developer runner prompt requires DeveloperWorker task package")
	}
	raw, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return "", err
	}
	instruction := developerRunnerInstruction
	if pkg.Type == model.TaskTypeReworkMergeRequest {
		instruction = developerRunnerReworkInstruction
	}
	return instruction +
		"\n\n执行现场：\n" +
		"- Coordinator 已准备当前工作区；当前目录中的 repo 与 workspace-manifest.json 是唯一 Git 事实来源。\n" +
		"- 禁止 clone、fetch、commit、push 或 force push；不得切换到其他提交，也不要读取或修改 Git 凭据配置。\n" +
		"- 只能修改当前 repo 中与任务目标直接相关的文件，并运行必要的最小充分验证。\n" +
		"- tests 只填写作为交付依据的通过验证（passed=true）；已确认与本次改动无关的既有失败（如环境或平台差异导致）写入 summary 说明，不得作为 tests 条目提交。\n" +
		resultSubmissionPolicy(results.SubmitDeveloperResultTool) +
		"\n任务包如下：\n```json\n" + string(raw) + "\n```", nil
}

const developerRunnerInstruction = `你是 DeveloperWorker，职责是根据任务包中的明确代码目标定位根因、修改生产代码并运行相关验证。

任务边界：
- 任务包是唯一业务上下文；只处理授权仓库和明确目标，不通过网络搜索、外部网页或 CodeHost API 补全需求。
- 需求缺少目标行为、预期效果或可验证标准时，输出 outcome=insufficient_context 和具体 blocker_reason，不进行探索性修改。
- 修改完成且验证通过时输出 outcome=change_ready、summary、全部通过的 tests 和 merge_request；Git commit、push 和 MR 创建由 Coordinator 完成。
- tests 至少一条、每条 command 与 summary 必填且 passed=true：代码改动提交相关测试或构建命令；纯文档/配置改动也必须执行并记录一项最小验证（如 "git diff --check"、目标文件内容断言）。tests 为空或"无测试适用"的说明不会被接受，结果会因缺少测试证据被拒绝。
- 未授权上游缺陷输出 upstream_dependency_defect 和 defect；修复失败、环境阻塞、上下文不足分别输出 fix_failed、environment_blocker、insufficient_context。
- 不直接创建 Issue、MR、Comment 或 Label，不负责调度、重试、恢复或状态推进。`

const developerRunnerReworkInstruction = `你是 DeveloperWorker，本轮执行已有 MR 的接续开发，不是新 Issue 的首次交付。

任务边界：
- 必须综合 context.origin_issue、context.merge_request、context.latest_review 和 context.rework_request.context 确定本轮修改范围。
- 当前工作区已经固定到原 MR source branch 的 expected_head_sha；不得创建新分支、新 Issue 或新 MR，不得修改 target/default branch。
- 原 Issue、MR、最新审查或当前评论不足以确定目标时，输出 outcome=insufficient_context 和具体 blocker_reason。
- 修改完成且验证通过时只输出 outcome=rework_ready、summary 和全部通过的 tests；不得输出 merge_request 或 defect。tests 至少一条、command 与 summary 必填且 passed=true；纯文档/配置改动也记录一项最小验证（如 "git diff --check"），空 tests 会被拒绝。
- Git commit 和向原 source branch 的普通 push 由 Coordinator 完成；不得创建新 MR。
- 不调用 GitLab / Gitea API，不负责 Comment、IM、审查或全局状态流转。`

func BuildReviewerRunnerPrompt(pkg agent.TaskPackage) (string, error) {
	if pkg.Role != "ReviewerWorker" {
		return "", errors.New("reviewer runner prompt requires ReviewerWorker task package")
	}
	raw, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return "", err
	}

	return reviewerRunnerInstruction +
		"\n\n执行现场：\n" +
		"- 当前目录由 Coordinator 按本次 Run 独立准备；代码和 workspace-manifest.json 是唯一 Git 事实来源。\n" +
		"- 使用 workspace-manifest.json 中固定的 target_sha、base_sha、head_sha、effective_review_mode 和 diff_range，不使用运行中可能变化的远程分支引用。\n" +
		resultSubmissionPolicy(results.SubmitReviewerResultTool) +
		"\n任务包如下：\n```json\n" + string(raw) + "\n```", nil
}

// resultSubmissionPolicy 是结果提交的三段式调用策略：MCP 工具为主通道，
// 校验错误同会话自矫正，耗尽或不可用才降级为最终消息文本（因此契约描述
// 必须保留在提示词中）。token 与端点由运行环境注入，绝不写进提示词。
func resultSubmissionPolicy(toolName string) string {
	return "\n结果提交方式（按顺序执行）：\n" +
		"1. 主通道——调用 MCP 工具 " + toolName + " 提交结果：参数中的 task_id 与 run_id 必须与下方任务包中的值完全一致，其余参数按本提示词所述契约构造（字段名与形态以工具入参说明为准）。工具返回 accepted 即完成：立即停止，不要再调用，也不要再输出结果 JSON。" +
		"2. 自矫正——工具返回 rejected 时，按 problems 中列出的问题逐条修正参数，在同一会话内重新调用；响应中的 attempts_remaining 是剩余矫正次数，耗尽时返回 rejected_final。" +
		"3. 降级——仅当收到 rejected_final、工具不可用或调用报传输错误时，才把结果按 result-schema.json 写成最终消息（只能输出一段 JSON，禁止 Markdown 围栏、解释文字或前后缀，不输出 schema 之外的字段），然后停止。"
}
