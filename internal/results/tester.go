package results

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"

	"sdlc-coordinator/internal/model"
)

// Tester 的结果契约围绕"对 MR 头部执行验证命令"定义:证据本体是执行事实
// (命令、尝试次数、退出码红绿),AI 只做归因性的 summary 标注;红绿判定
// 永远以退出码为准。
const (
	TesterOutcomeVerificationPassed  = "verification_passed"
	TesterOutcomeVerificationFailed  = "verification_failed"
	TesterOutcomeEnvironmentBlocker  = "environment_blocker"
	TesterOutcomeInsufficientContext = "insufficient_context"
)

type TesterResultInput struct {
	TaskID        string         `json:"task_id"`
	RunID         string         `json:"run_id"`
	Outcome       string         `json:"outcome"`
	Summary       string         `json:"summary"`
	Tests         []TestEvidence `json:"tests,omitempty"`
	BlockerReason *string        `json:"blocker_reason,omitempty"`
}

func (i *TesterResultInput) Identity() (string, string) { return i.TaskID, i.RunID }

type TesterContract struct{}

func NewTesterContract() TesterContract { return TesterContract{} }
func (TesterContract) ToolName() string { return SubmitTesterResultTool }
func (TesterContract) Role() string     { return "TesterWorker" }
func (TesterContract) DecodeStrict(raw json.RawMessage) (RoleResultInput, error) {
	return decodeStrict[TesterResultInput](raw)
}

func (TesterContract) Validate(task *model.Task, run *model.Run, raw RoleResultInput) error {
	if err := validateContractContext("TesterWorker", task, run, raw); err != nil {
		return err
	}
	input, ok := raw.(*TesterResultInput)
	if !ok {
		return errors.New("tester result input type is invalid")
	}
	if strings.TrimSpace(input.Summary) == "" {
		return errors.New("summary is required")
	}
	switch input.Outcome {
	case TesterOutcomeVerificationPassed:
		return validateTests(input.Tests, true)
	case TesterOutcomeVerificationFailed:
		return validateTests(input.Tests, false)
	case TesterOutcomeEnvironmentBlocker, TesterOutcomeInsufficientContext:
		if input.BlockerReason == nil || strings.TrimSpace(*input.BlockerReason) == "" {
			return fmt.Errorf("blocker_reason is required for %s", input.Outcome)
		}
		return nil
	default:
		return fmt.Errorf("unsupported tester outcome %q", input.Outcome)
	}
}

func (TesterContract) Normalize(task *model.Task, _ *model.Run, raw RoleResultInput) (CanonicalResult, error) {
	input, ok := raw.(*TesterResultInput)
	if !ok {
		return CanonicalResult{}, errors.New("tester result input type is invalid")
	}
	artifacts := map[string]any{"verification_verdict": input.Outcome}
	if head := verificationHeadSHA(task); head != "" {
		artifacts["verification_head_sha"] = head
	}
	if len(input.Tests) > 0 {
		artifacts["test_results"] = input.Tests
	}
	result := CanonicalResult{
		TaskID: input.TaskID, RunID: input.RunID, Summary: input.Summary,
		Artifacts: artifacts, BlockerReason: input.BlockerReason,
	}
	switch input.Outcome {
	case TesterOutcomeVerificationPassed, TesterOutcomeVerificationFailed:
		// 红绿是证据不是任务成败:验证任务跑完并如实报告即为成功,与
		// reviewer 发现缺陷但任务成功的语义同构。
		projectID, mrIID, err := testerTarget(task)
		if err != nil {
			return CanonicalResult{}, err
		}
		result.Status = StatusSucceeded
		result.Writebacks = []CodeHostWritebackIntent{{
			Kind: "create_merge_request_note", ProjectID: projectID, MergeRequestIID: mrIID,
			Body: buildVerificationReportBody(task, input),
		}}
	case TesterOutcomeEnvironmentBlocker:
		// 验证自身没跑成=无证据,按失败重试路径走:编排器按重试预算给
		// retry_pending 循环重派,预算耗尽停靠 blocked 并对 tester 物化 IM
		// 通知;判停 parked 会让环境瞬态问题静默杀死整条验证链。
		result.Status = StatusFailed
	case TesterOutcomeInsufficientContext:
		result.Status = StatusHumanRequired
	}
	return result, nil
}

func (TesterContract) InputSchema() map[string]any {
	return baseInputSchema(
		[]string{"task_id", "run_id", "outcome", "summary"},
		map[string]any{
			"task_id": map[string]any{"type": "string"}, "run_id": map[string]any{"type": "string"},
			"outcome": map[string]any{"type": "string", "enum": []string{
				TesterOutcomeVerificationPassed, TesterOutcomeVerificationFailed,
				TesterOutcomeEnvironmentBlocker, TesterOutcomeInsufficientContext,
			}},
			"summary":        map[string]any{"type": "string"},
			"tests":          testEvidenceArraySchema(),
			"blocker_reason": map[string]any{"type": []string{"string", "null"}},
		},
	)
}

// verificationHeadSHA 取验证对象头部 SHA:来源是任务上下文(任务创建时冻结),
// 不信任 agent 回显,证据与 SHA 的绑定由系统侧完成。
func verificationHeadSHA(task *model.Task) string {
	context, err := parseTaskContext(task)
	if err != nil {
		return ""
	}
	return stringField(objectField(context, "merge_request"), "head_sha")
}

func testerTarget(task *model.Task) (string, int, error) {
	return reviewerTarget(task)
}

func buildVerificationReportBody(task *model.Task, input *TesterResultInput) string {
	var body strings.Builder
	body.WriteString("## 自动验证报告\n\n")
	if input.Outcome == TesterOutcomeVerificationPassed {
		body.WriteString("- 结论：通过\n")
	} else {
		body.WriteString("- 结论：未通过\n")
	}
	if head := verificationHeadSHA(task); head != "" {
		fmt.Fprintf(&body, "- 验证对象：head `%s`\n", head)
	}
	body.WriteString("\n| 命令 | 尝试 | 结果 | 说明 |\n|---|---|---|---|\n")
	for _, test := range input.Tests {
		verdict := "未通过"
		if test.Passed {
			verdict = "通过"
		}
		attempts := test.Attempts
		if attempts <= 0 {
			attempts = 1
		}
		summary := strings.ReplaceAll(test.Summary, "\n", " ")
		fmt.Fprintf(&body, "| `%s` | %d | %s | %s |\n", escapeMarkdownTableCell(test.Command), attempts, verdict, escapeMarkdownTableCell(summary))
	}
	fmt.Fprintf(&body, "\n%s\n", input.Summary)
	return body.String()
}

// escapeMarkdownTableCell 转义表格单元内的管道符并抹掉反引号,防止命令或
// 摘要里的特殊字符把报告表格打穿(纯展示防御,不影响证据本体)。
func escapeMarkdownTableCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.ReplaceAll(value, "`", "'")
}

func validateDefect(defect *DefectResult) error {
	if defect == nil || strings.TrimSpace(defect.Repository) == "" || strings.TrimSpace(defect.Title) == "" || strings.TrimSpace(defect.Description) == "" {
		return errors.New("defect repository, title, and description are required")
	}
	return nil
}

func validateUpstreamDefect(task *model.Task, defect *DefectResult) error {
	if err := validateDefect(defect); err != nil {
		return err
	}
	if defect.Repository == task.TargetRepo {
		return errors.New("upstream defect repository must differ from task target")
	}
	if defect.Repository != task.SourceRepo {
		return fmt.Errorf("upstream defect repository %q is not authorized by task source %q", defect.Repository, task.SourceRepo)
	}
	return nil
}

func normalizeUpstreamDefect(task *model.Task, _ *model.Run, taskID, runID, summary string, tests []TestEvidence, evidence *FailureEvidence, defect *DefectResult) (CanonicalResult, error) {
	context, err := parseTaskContext(task)
	if err != nil {
		return CanonicalResult{}, err
	}
	relation := strings.TrimSpace(defect.DependencyRelation)
	if relation == "" {
		relation = "upstream_fix"
	}
	childContext := map[string]any{
		"repos":          reposForUpstreamChild(context["repos"], task.TargetRepo, defect.Repository),
		"parent_task_id": task.TaskID,
		"defect":         defect,
	}
	artifacts := resultArtifacts(nil, tests, nil)
	if evidence != nil {
		artifacts["failure_evidence"] = evidence
	}
	return CanonicalResult{
		TaskID: taskID, RunID: runID, Status: StatusSucceeded, Summary: summary, Artifacts: artifacts,
		Writebacks: []CodeHostWritebackIntent{{Kind: "create_issue", ProjectID: defect.Repository, Title: defect.Title, Description: defect.Description}},
		NextTasks: []NextTaskIntent{{
			DedupeKey: fmt.Sprintf("worker:%s:upstream_fix:%s", task.TaskID, defect.Repository),
			Type:      "upstream_fix", Role: "DeveloperWorker", SourceRepo: task.TargetRepo, TargetRepo: defect.Repository,
			Objective: defect.Description, AcceptanceCriteria: "修复上游缺陷并运行上游与下游验证。", DependencyRelation: relation,
			AllowedRepos: AllowedRepos{Writable: []string{defect.Repository}, Readable: []string{task.TargetRepo}},
			Context:      childContext,
		}},
	}, nil
}

func reposForUpstreamChild(raw any, downstreamRepo, upstreamRepo string) any {
	repos, ok := raw.([]any)
	if !ok {
		return raw
	}
	downstreamName := path.Base(strings.Trim(downstreamRepo, "/"))
	upstreamName := path.Base(strings.Trim(upstreamRepo, "/"))
	result := make([]any, 0, len(repos))
	for _, value := range repos {
		repo, ok := value.(map[string]any)
		if !ok {
			result = append(result, value)
			continue
		}
		copyRepo := make(map[string]any, len(repo))
		for key, field := range repo {
			copyRepo[key] = field
		}
		name := stringField(copyRepo, "name")
		switch name {
		case upstreamName:
			copyRepo["access"] = "writable"
		case downstreamName:
			copyRepo["access"] = "readable"
		}
		result = append(result, copyRepo)
	}
	return result
}
