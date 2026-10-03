package results

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/model"
)

type DeveloperAgentResultInput struct {
	TaskID  string `json:"task_id"`
	RunID   string `json:"run_id"`
	Outcome string `json:"outcome"`
	Summary string `json:"summary"`
	// CommitMessage 是 ready 类结果必填的提交说明(标题+可选正文);Coordinator
	// 校验后作为 Git 提交正文并统一追加追溯尾注。summary 保持任务结果摘要
	// 用途,不再派生提交标题。
	CommitMessage string              `json:"commit_message,omitempty"`
	Tests         []TestEvidence      `json:"tests,omitempty"`
	MergeRequest  *MergeRequestResult `json:"merge_request,omitempty"`
	Defect        *DefectResult       `json:"defect,omitempty"`
	BlockerReason *string             `json:"blocker_reason,omitempty"`
}

func (i *DeveloperAgentResultInput) Identity() (string, string) { return i.TaskID, i.RunID }

type DeveloperAgentContract struct{}

func NewDeveloperAgentContract() DeveloperAgentContract { return DeveloperAgentContract{} }

func (DeveloperAgentContract) DecodeStrict(raw json.RawMessage) (RoleResultInput, error) {
	return decodeStrict[DeveloperAgentResultInput](raw)
}

func (DeveloperAgentContract) Validate(task *model.Task, run *model.Run, raw RoleResultInput) error {
	if err := validateContractContext("DeveloperWorker", task, run, raw); err != nil {
		return err
	}
	input, ok := raw.(*DeveloperAgentResultInput)
	if !ok {
		return errors.New("developer agent result input type is invalid")
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
		if err := validateTests(input.Tests, true); err != nil {
			return err
		}
		if input.MergeRequest == nil || strings.TrimSpace(input.MergeRequest.Title) == "" {
			return errors.New("merge_request title is required for change_ready")
		}
		if _, err := NormalizeCommitMessage(input.CommitMessage); err != nil {
			return err
		}
		if input.Defect != nil || input.BlockerReason != nil {
			return errors.New("change_ready must not include defect or blocker_reason")
		}
		_, _, err := deliveryTarget(task)
		return err
	case "rework_ready":
		if err := validateTests(input.Tests, true); err != nil {
			return err
		}
		if input.MergeRequest != nil || input.Defect != nil || input.BlockerReason != nil {
			return errors.New("rework_ready must not include merge_request, defect or blocker_reason")
		}
		if _, err := NormalizeCommitMessage(input.CommitMessage); err != nil {
			return err
		}
		_, _, err := deliveryTarget(task)
		return err
	case "upstream_dependency_defect":
		if strings.TrimSpace(input.CommitMessage) != "" {
			return fmt.Errorf("outcome %s must not include commit_message", input.Outcome)
		}
		return validateUpstreamDefect(task, input.Defect)
	case "fix_failed", "environment_blocker", "insufficient_context":
		if strings.TrimSpace(input.CommitMessage) != "" {
			return fmt.Errorf("outcome %s must not include commit_message", input.Outcome)
		}
		if input.BlockerReason == nil || strings.TrimSpace(*input.BlockerReason) == "" {
			return fmt.Errorf("blocker_reason is required for %s", input.Outcome)
		}
		return nil
	default:
		return fmt.Errorf("unsupported developer outcome %q; supported outcomes: %s", input.Outcome, strings.Join(developerOutcomeValues, ", "))
	}
}

func (DeveloperAgentContract) WithDelivery(raw RoleResultInput, delivery DeliveryEvidence) (*DeveloperResultInput, error) {
	input, ok := raw.(*DeveloperAgentResultInput)
	if !ok {
		return nil, errors.New("developer agent result input type is invalid")
	}
	if input.Outcome != "change_ready" && input.Outcome != "rework_ready" {
		return nil, fmt.Errorf("developer outcome %s does not accept delivery evidence", input.Outcome)
	}
	if strings.TrimSpace(delivery.CommitSHA) == "" || len(delivery.ChangedFiles) == 0 || strings.TrimSpace(delivery.DiffSummary) == "" {
		return nil, errors.New("coordinator delivery evidence is incomplete")
	}
	copy := delivery
	return &DeveloperResultInput{
		TaskID: input.TaskID, RunID: input.RunID, Outcome: input.Outcome, Summary: input.Summary,
		Delivery: &copy, Tests: append([]TestEvidence(nil), input.Tests...), MergeRequest: input.MergeRequest,
		Defect: input.Defect, BlockerReason: input.BlockerReason,
	}, nil
}

func (DeveloperAgentContract) WithoutDelivery(raw RoleResultInput) (*DeveloperResultInput, error) {
	input, ok := raw.(*DeveloperAgentResultInput)
	if !ok {
		return nil, errors.New("developer agent result input type is invalid")
	}
	if input.Outcome == "change_ready" || input.Outcome == "rework_ready" {
		return nil, fmt.Errorf("developer outcome %s requires coordinator delivery evidence", input.Outcome)
	}
	return &DeveloperResultInput{
		TaskID: input.TaskID, RunID: input.RunID, Outcome: input.Outcome, Summary: input.Summary,
		Tests: append([]TestEvidence(nil), input.Tests...), MergeRequest: input.MergeRequest,
		Defect: input.Defect, BlockerReason: input.BlockerReason,
	}, nil
}

func (DeveloperAgentContract) InputSchema() map[string]any {
	return baseInputSchema(
		[]string{"task_id", "run_id", "outcome", "summary"},
		map[string]any{
			"task_id": map[string]any{"type": "string"}, "run_id": map[string]any{"type": "string"},
			"outcome": map[string]any{"type": "string", "enum": append([]string(nil), developerOutcomeValues...)},
			"summary": map[string]any{"type": "string"}, "tests": testEvidenceArraySchema(),
			"commit_message": map[string]any{"type": "string", "maxLength": MaxCommitMessageBytes},
			"merge_request":  mergeRequestResultSchema(), "defect": defectResultSchema(),
			"blocker_reason": map[string]any{"type": []string{"string", "null"}},
		},
	)
}
