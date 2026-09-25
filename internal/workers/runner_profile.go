package workers

import (
	"encoding/json"
	"errors"
	"strings"

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
- 不调用代码托管平台 API（GitLab / Gitea / GitHub），不负责 Comment、IM、审查或全局状态流转。`

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

func BuildTesterRunnerTaskBundle(task *model.Task, run *model.Run) (agent.TaskBundle, error) {
	pkg, err := testerRunnerTaskPackage(task, run)
	if err != nil {
		return agent.TaskBundle{}, err
	}
	prompt, err := BuildTesterRunnerPrompt(pkg)
	if err != nil {
		return agent.TaskBundle{}, err
	}
	return agent.TaskBundle{
		TaskPackage:  pkg,
		ResultSchema: results.NewTesterContract().InputSchema(),
		Prompt:       prompt,
	}, nil
}

// testerRunnerTaskPackage 优先复用派发时持久化进 run.InputJSON 的任务包
// （其中携带验证处方），投影重建等路径据此恢复同一处方；InputJSON 缺失或
// 不含有效任务包时退回按 task+run 现构（无处方段）。
func testerRunnerTaskPackage(task *model.Task, run *model.Run) (agent.TaskPackage, error) {
	if strings.TrimSpace(run.InputJSON) != "" {
		var pkg agent.TaskPackage
		if err := json.Unmarshal([]byte(run.InputJSON), &pkg); err == nil && pkg.TaskID == task.TaskID && pkg.Role == task.Role {
			return pkg, nil
		}
	}
	return BuildRunnerTaskPackage(task, run)
}

func BuildTesterRunnerPrompt(pkg agent.TaskPackage) (string, error) {
	if pkg.Role != "TesterWorker" {
		return "", errors.New("tester runner prompt requires TesterWorker task package")
	}
	raw, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return "", err
	}
	return testerRunnerInstruction +
		"\n\n执行现场：\n" +
		"- 当前目录由 Coordinator 按本次 Run 独立准备；代码和 workspace-manifest.json 是唯一 Git 事实来源，head_sha 即本次验证对象。\n" +
		resultSubmissionPolicy(results.SubmitTesterResultTool) +
		"\n任务包如下：\n```json\n" + string(raw) + "\n```", nil
}

const testerRunnerInstruction = `你是 TesterWorker，职责是对任务包指定的 MR 头部执行验证命令并回传真实执行证据。红绿判定只认命令退出码，你的分析只能写进 summary 作归因说明。

任务边界：
- 只读纪律：不得修改、暂存、提交或推送任何文件，不得切换提交，不得读取或修改 Git 凭据；测试运行产生的构建产物与临时文件除外。
- 处方执行：任务包 verification.floor_command 是注册仓声明的验证命令下限（存在时必须执行）；verification.replay_commands 是开发记录登记的测试命令复放清单，逐条照单执行。
- 机械重试：命令失败时自动重跑，同一条命令最多再执行 2 次；任一次绿即记 passed=true 并在 attempts 写实际执行次数，次次红才记 passed=false。AI 的 flaky/环境怀疑只能写进 summary，不改变红绿判定。
- 判定：全部处方命令最终通过 → outcome=verification_passed；任一命令次次红 → outcome=verification_failed；环境导致命令无法执行 → outcome=environment_blocker 与具体 blocker_reason；处方缺失或上下文不足以执行 → outcome=insufficient_context 与具体 blocker_reason。
- tests 逐条记录实际执行：command 必填、passed 为最终红绿、summary 写关键输出摘要、attempts 写执行次数；不得虚构条目，不得把未执行的命令写进 tests。
- 不创建 Issue、MR、Comment 或 Label，不负责调度、重试、恢复或状态推进。`
