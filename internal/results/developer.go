package results

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"sdlc-coordinator/internal/model"
)

type DeveloperResultInput struct {
	TaskID        string              `json:"task_id"`
	RunID         string              `json:"run_id"`
	Outcome       string              `json:"outcome"`
	Summary       string              `json:"summary"`
	Delivery      *DeliveryEvidence   `json:"delivery,omitempty"`
	Tests         []TestEvidence      `json:"tests,omitempty"`
	MergeRequest  *MergeRequestResult `json:"merge_request,omitempty"`
	Defect        *DefectResult       `json:"defect,omitempty"`
	BlockerReason *string             `json:"blocker_reason,omitempty"`
}

var developerOutcomeValues = []string{
	"change_ready",
	"rework_ready",
	"upstream_dependency_defect",
	"fix_failed",
	"environment_blocker",
	"insufficient_context",
}

func (i *DeveloperResultInput) Identity() (string, string) { return i.TaskID, i.RunID }

type DeveloperContract struct{}

func NewDeveloperContract() DeveloperContract { return DeveloperContract{} }
func (DeveloperContract) ToolName() string    { return SubmitDeveloperResultTool }
func (DeveloperContract) Role() string        { return "DeveloperWorker" }

func (DeveloperContract) DecodeStrict(raw json.RawMessage) (RoleResultInput, error) {
	return decodeStrict[DeveloperResultInput](raw)
}

func (DeveloperContract) Validate(task *model.Task, run *model.Run, raw RoleResultInput) error {
	if err := validateContractContext("DeveloperWorker", task, run, raw); err != nil {
		return err
	}
	input, ok := raw.(*DeveloperResultInput)
	if !ok {
		return errors.New("developer result input type is invalid")
	}
	if strings.TrimSpace(input.Summary) == "" {
		return errors.New("summary is required")
	}
	isReworkTask := task.Type == model.TaskTypeReworkMergeRequest
	if input.Outcome == "rework_ready" && !isReworkTask {
		return errors.New("rework_ready is only valid for rework_merge_request tasks")
	}
	if isReworkTask && (input.Outcome == "change_ready" || input.Outcome == "upstream_dependency_defect") {
		return fmt.Errorf("rework_merge_request does not allow %s", input.Outcome)
	}
	switch input.Outcome {
	case "change_ready":
		if input.Delivery == nil || strings.TrimSpace(input.Delivery.CommitSHA) == "" || strings.TrimSpace(input.Delivery.DiffSummary) == "" {
			return errors.New("delivery commit_sha and diff_summary are required for change_ready")
		}
		if len(input.Delivery.ChangedFiles) == 0 {
			return errors.New("delivery changed_files are required for change_ready")
		}
		if err := validateTests(input.Tests, true); err != nil {
			return err
		}
		if input.MergeRequest == nil || strings.TrimSpace(input.MergeRequest.Title) == "" {
			return errors.New("merge_request title is required for change_ready")
		}
		_, _, err := deliveryTarget(task)
		return err
	case "rework_ready":
		if input.Delivery == nil || strings.TrimSpace(input.Delivery.CommitSHA) == "" || strings.TrimSpace(input.Delivery.DiffSummary) == "" {
			return errors.New("delivery commit_sha and diff_summary are required for rework_ready")
		}
		if len(input.Delivery.ChangedFiles) == 0 {
			return errors.New("delivery changed_files are required for rework_ready")
		}
		if err := validateTests(input.Tests, true); err != nil {
			return err
		}
		if input.MergeRequest != nil || input.Defect != nil {
			return errors.New("rework_ready must not include merge_request or defect")
		}
		_, _, err := deliveryTarget(task)
		return err
	case "upstream_dependency_defect":
		return validateUpstreamDefect(task, input.Defect)
	case "fix_failed", "environment_blocker", "insufficient_context":
		if input.BlockerReason == nil || strings.TrimSpace(*input.BlockerReason) == "" {
			return fmt.Errorf("blocker_reason is required for %s", input.Outcome)
		}
		return nil
	default:
		return fmt.Errorf("unsupported developer outcome %q; supported outcomes: %s", input.Outcome, strings.Join(developerOutcomeValues, ", "))
	}
}

func (DeveloperContract) Normalize(task *model.Task, run *model.Run, raw RoleResultInput) (CanonicalResult, error) {
	input, ok := raw.(*DeveloperResultInput)
	if !ok {
		return CanonicalResult{}, errors.New("developer result input type is invalid")
	}
	result := CanonicalResult{TaskID: input.TaskID, RunID: input.RunID, Summary: input.Summary, BlockerReason: input.BlockerReason}
	switch input.Outcome {
	case "change_ready":
		sourceBranch, targetBranch, err := deliveryTarget(task)
		if err != nil {
			return CanonicalResult{}, err
		}
		result.Status = StatusSucceeded
		result.Artifacts = resultArtifacts(input.Delivery, input.Tests, map[string]any{"delivery_branch": sourceBranch})
		result.Writebacks = []CodeHostWritebackIntent{{
			Kind:               "create_merge_request",
			ProjectID:          task.TargetRepo,
			Title:              input.MergeRequest.Title,
			Description:        appendGitLabIssueClosingReference(task, input.MergeRequest.Description),
			SourceBranch:       sourceBranch,
			TargetBranch:       targetBranch,
			RemoveSourceBranch: input.MergeRequest.RemoveSourceBranch,
		}}
	case "rework_ready":
		result.Status = StatusSucceeded
		result.Artifacts = resultArtifacts(input.Delivery, input.Tests, map[string]any{"delivery_branch": model.DeliveryBranch(task)})
	case "upstream_dependency_defect":
		return normalizeUpstreamDefect(task, run, input.TaskID, input.RunID, input.Summary, input.Tests, nil, input.Defect)
	case "fix_failed":
		result.Status = StatusFailed
		result.Artifacts = resultArtifacts(nil, input.Tests, nil)
	case "environment_blocker":
		result.Status = StatusBlocked
		result.Artifacts = resultArtifacts(nil, input.Tests, nil)
	case "insufficient_context":
		result.Status = StatusHumanRequired
	}
	return result, nil
}

func appendGitLabIssueClosingReference(task *model.Task, description string) string {
	context, err := parseTaskContext(task)
	if err != nil {
		return description
	}
	issueIID := numberField(objectField(context, "issue"), "iid")
	if issueIID <= 0 || normalizeProjectPath(stringField(objectField(context, "project"), "path")) != normalizeProjectPath(task.TargetRepo) {
		return description
	}

	closingReference := "Closes #" + strconv.Itoa(issueIID)
	if strings.Contains(strings.ToLower(description), strings.ToLower(closingReference)) {
		return description
	}
	if strings.TrimSpace(description) == "" {
		return closingReference
	}
	return strings.TrimRight(description, "\r\n") + "\n\n" + closingReference
}

func normalizeProjectPath(project string) string {
	return strings.Trim(strings.TrimSpace(project), "/")
}

func (DeveloperContract) InputSchema() map[string]any {
	return baseInputSchema(
		[]string{"task_id", "run_id", "outcome", "summary"},
		map[string]any{
			"task_id": map[string]any{"type": "string"}, "run_id": map[string]any{"type": "string"},
			"outcome": map[string]any{"type": "string", "enum": append([]string(nil), developerOutcomeValues...)},
			"summary": map[string]any{"type": "string"}, "delivery": deliveryEvidenceSchema(),
			"tests": testEvidenceArraySchema(), "merge_request": mergeRequestResultSchema(),
			"defect": defectResultSchema(), "blocker_reason": map[string]any{"type": []string{"string", "null"}},
		},
	)
}
