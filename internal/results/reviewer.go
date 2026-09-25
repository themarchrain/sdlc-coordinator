package results

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/model"
)

type ReviewerFinding struct {
	Severity string `json:"severity,omitempty"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
}

type ReviewerResultInput struct {
	TaskID      string            `json:"task_id"`
	RunID       string            `json:"run_id"`
	Verdict     string            `json:"verdict"`
	Summary     string            `json:"summary"`
	Findings    []ReviewerFinding `json:"findings"`
	TestSummary string            `json:"test_summary"`
	CommentBody string            `json:"comment_body"`
}

func (i *ReviewerResultInput) Identity() (string, string) { return i.TaskID, i.RunID }

type ReviewerContract struct{}

func NewReviewerContract() ReviewerContract { return ReviewerContract{} }
func (ReviewerContract) ToolName() string   { return SubmitReviewerResultTool }
func (ReviewerContract) Role() string       { return "ReviewerWorker" }
func (ReviewerContract) DecodeStrict(raw json.RawMessage) (RoleResultInput, error) {
	return decodeStrict[ReviewerResultInput](raw)
}

func (ReviewerContract) Validate(task *model.Task, run *model.Run, raw RoleResultInput) error {
	if err := validateContractContext("ReviewerWorker", task, run, raw); err != nil {
		return err
	}
	input, ok := raw.(*ReviewerResultInput)
	if !ok {
		return errors.New("reviewer result input type is invalid")
	}
	if strings.TrimSpace(input.Summary) == "" || strings.TrimSpace(input.TestSummary) == "" || strings.TrimSpace(input.CommentBody) == "" {
		return errors.New("summary, test_summary, and comment_body are required")
	}
	if input.Verdict != "approved" && input.Verdict != "changes_requested" && input.Verdict != "human_required" && input.Verdict != "no_new_changes" {
		return fmt.Errorf("unsupported reviewer verdict %q", input.Verdict)
	}
	if input.Verdict == "no_new_changes" {
		reviewMode, err := reviewerReviewMode(task)
		if err != nil {
			return err
		}
		if !strings.EqualFold(reviewMode, "incremental") {
			return errors.New("no_new_changes is only valid for incremental review")
		}
		if len(input.Findings) != 0 {
			return errors.New("no_new_changes findings must be empty")
		}
		const conclusion = "本轮无新增可审内容，不产生新的合并建议；以最近一次实质审查结论为准"
		if !strings.Contains(input.CommentBody, conclusion) {
			return errors.New("no_new_changes comment_body must preserve the previous substantive review conclusion")
		}
	}
	projectID, mrIID, err := reviewerTarget(task)
	if err != nil {
		return err
	}
	if projectID != task.TargetRepo || mrIID <= 0 {
		return errors.New("review target does not match task repository")
	}
	return nil
}

func (ReviewerContract) Normalize(task *model.Task, _ *model.Run, raw RoleResultInput) (CanonicalResult, error) {
	input, ok := raw.(*ReviewerResultInput)
	if !ok {
		return CanonicalResult{}, errors.New("reviewer result input type is invalid")
	}
	projectID, mrIID, err := reviewerTarget(task)
	if err != nil {
		return CanonicalResult{}, err
	}
	status := StatusSucceeded
	var blockerReason *string
	if input.Verdict == "human_required" {
		status = StatusHumanRequired
		reason := input.Summary
		blockerReason = &reason
	}
	result := CanonicalResult{
		TaskID: input.TaskID, RunID: input.RunID, Status: status, Summary: input.Summary, BlockerReason: blockerReason,
		Artifacts:  map[string]any{"review_verdict": input.Verdict, "review_findings": input.Findings, "review_test_summary": input.TestSummary},
		Writebacks: []CodeHostWritebackIntent{{Kind: "create_merge_request_note", ProjectID: projectID, MergeRequestIID: mrIID, Body: input.CommentBody}},
	}
	// 审查放行(approved/no_new_changes)即派生验证子任务:changes_requested
	// 与 human_required 意味着 MR 还会变化,验证留给放行后的那一轮头部。
	if input.Verdict == "approved" || input.Verdict == "no_new_changes" {
		result.NextTasks = reviewerVerificationNextTasks(task)
	}
	return result, nil
}

// reviewerVerificationNextTasks 构造验证子任务意图:上下文整块继承审查任务
// 的 project/merge_request/repos(头部 SHA 在任务创建时冻结),同 MR 同头部
// 的重复放行经确定性去重键幂等,不会重复建任务。
func reviewerVerificationNextTasks(task *model.Task) []NextTaskIntent {
	context, err := parseTaskContext(task)
	if err != nil {
		return nil
	}
	project := objectField(context, "project")
	mergeRequest := objectField(context, "merge_request")
	projectPath := stringField(project, "path")
	if projectPath == "" {
		projectPath = task.TargetRepo
	}
	headSHA := stringField(mergeRequest, "head_sha")
	mrIID := numberField(mergeRequest, "iid")
	if projectPath == "" || headSHA == "" || mrIID <= 0 {
		return nil
	}
	childContext := map[string]any{}
	if raw, ok := context["project"]; ok {
		childContext["project"] = raw
	}
	if raw, ok := context["merge_request"]; ok {
		childContext["merge_request"] = raw
	}
	if raw, ok := context["repos"]; ok {
		childContext["repos"] = raw
	}
	return []NextTaskIntent{{
		DedupeKey:          fmt.Sprintf("verify:%s:%d:%s", projectPath, mrIID, headSHA),
		Type:               "verify_merge_request",
		Role:               "TesterWorker",
		SourceRepo:         projectPath,
		TargetRepo:         projectPath,
		Objective:          fmt.Sprintf("Verify MR !%d in %s at head %s by running the prescribed verification commands.", mrIID, projectPath, headSHA),
		AcceptanceCriteria: "按处方执行验证命令并回传真实执行证据（命令、尝试次数、红绿、摘要）；红绿以退出码为准。",
		DependencyRelation: "verification",
		AllowedRepos:       AllowedRepos{Readable: []string{projectPath}},
		Context:            childContext,
	}}
}

func (ReviewerContract) InputSchema() map[string]any {
	return baseInputSchema(
		[]string{"task_id", "run_id", "verdict", "summary", "findings", "test_summary", "comment_body"},
		map[string]any{
			"task_id": map[string]any{"type": "string"}, "run_id": map[string]any{"type": "string"},
			"verdict": map[string]any{"type": "string", "enum": []string{"approved", "changes_requested", "human_required", "no_new_changes"}},
			"summary": map[string]any{"type": "string"}, "findings": map[string]any{
				"type": "array",
				"items": strictObjectSchema(
					[]string{"message"},
					map[string]any{
						"severity": map[string]any{"type": "string"}, "file": map[string]any{"type": "string"},
						"line": map[string]any{"type": "integer"}, "message": map[string]any{"type": "string"},
					},
				),
			},
			"test_summary": map[string]any{"type": "string"}, "comment_body": map[string]any{"type": "string"},
		},
	)
}

func reviewerReviewMode(task *model.Task) (string, error) {
	context, err := parseTaskContext(task)
	if err != nil {
		return "", err
	}
	return stringField(objectField(context, "merge_request"), "review_mode"), nil
}

func reviewerTarget(task *model.Task) (string, int, error) {
	context, err := parseTaskContext(task)
	if err != nil {
		return "", 0, err
	}
	project := objectField(context, "project")
	mergeRequest := objectField(context, "merge_request")
	projectID := stringField(project, "path")
	if projectID == "" {
		projectID = task.TargetRepo
	}
	mrIID := numberField(mergeRequest, "iid")
	if projectID == "" || mrIID <= 0 {
		return "", 0, errors.New("task context project.path and merge_request.iid are required")
	}
	return projectID, mrIID, nil
}
