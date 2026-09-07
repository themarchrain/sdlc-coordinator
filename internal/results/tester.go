package results

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"

	"sdlc-coordinator/internal/model"
)

type TesterResultInput struct {
	TaskID        string              `json:"task_id"`
	RunID         string              `json:"run_id"`
	Outcome       string              `json:"outcome"`
	Summary       string              `json:"summary"`
	Tests         []TestEvidence      `json:"tests,omitempty"`
	Evidence      *FailureEvidence    `json:"evidence,omitempty"`
	Delivery      *DeliveryEvidence   `json:"delivery,omitempty"`
	MergeRequest  *MergeRequestResult `json:"merge_request,omitempty"`
	Defect        *DefectResult       `json:"defect,omitempty"`
	BlockerReason *string             `json:"blocker_reason,omitempty"`
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
	case "test_patch_passed":
		if input.Delivery == nil || strings.TrimSpace(input.Delivery.CommitSHA) == "" || len(input.Delivery.ChangedFiles) == 0 {
			return errors.New("delivery evidence is required for test_patch_passed")
		}
		if err := validateTests(input.Tests, true); err != nil {
			return err
		}
		if input.MergeRequest == nil || strings.TrimSpace(input.MergeRequest.Title) == "" {
			return errors.New("merge_request title is required for test_patch_passed")
		}
		_, _, err := deliveryTarget(task)
		return err
	case "local_code_defect":
		if input.Defect == nil || input.Defect.Repository != task.TargetRepo {
			return fmt.Errorf("local defect repository must be task target %q", task.TargetRepo)
		}
		return validateDefect(input.Defect)
	case "upstream_dependency_defect":
		return validateUpstreamDefect(task, input.Defect)
	case "environment_blocker", "insufficient_context":
		if input.BlockerReason == nil || strings.TrimSpace(*input.BlockerReason) == "" {
			return fmt.Errorf("blocker_reason is required for %s", input.Outcome)
		}
		return nil
	default:
		return fmt.Errorf("unsupported tester outcome %q", input.Outcome)
	}
}

func (TesterContract) Normalize(task *model.Task, run *model.Run, raw RoleResultInput) (CanonicalResult, error) {
	input, ok := raw.(*TesterResultInput)
	if !ok {
		return CanonicalResult{}, errors.New("tester result input type is invalid")
	}
	evidence := map[string]any{}
	if input.Evidence != nil {
		evidence["failure_evidence"] = input.Evidence
	}
	result := CanonicalResult{
		TaskID: input.TaskID, RunID: input.RunID, Summary: input.Summary,
		Artifacts: resultArtifacts(input.Delivery, input.Tests, evidence), BlockerReason: input.BlockerReason,
	}
	switch input.Outcome {
	case "test_patch_passed":
		sourceBranch, targetBranch, err := deliveryTarget(task)
		if err != nil {
			return CanonicalResult{}, err
		}
		result.Status = StatusSucceeded
		result.Artifacts["delivery_branch"] = sourceBranch
		result.Writebacks = []CodeHostWritebackIntent{{
			Kind: "create_merge_request", ProjectID: task.TargetRepo, SourceBranch: sourceBranch, TargetBranch: targetBranch,
			Title: input.MergeRequest.Title, Description: input.MergeRequest.Description, RemoveSourceBranch: input.MergeRequest.RemoveSourceBranch,
		}}
	case "local_code_defect":
		result.Status = StatusSucceeded
		result.Writebacks = []CodeHostWritebackIntent{{Kind: "create_issue", ProjectID: task.TargetRepo, Title: input.Defect.Title, Description: input.Defect.Description}}
	case "upstream_dependency_defect":
		return normalizeUpstreamDefect(task, run, input.TaskID, input.RunID, input.Summary, input.Tests, input.Evidence, input.Defect)
	case "environment_blocker":
		result.Status = StatusBlocked
	case "insufficient_context":
		result.Status = StatusHumanRequired
	}
	return result, nil
}

func (TesterContract) InputSchema() map[string]any {
	return baseInputSchema(
		[]string{"task_id", "run_id", "outcome", "summary"},
		map[string]any{
			"task_id": map[string]any{"type": "string"}, "run_id": map[string]any{"type": "string"},
			"outcome": map[string]any{"type": "string", "enum": []string{"test_patch_passed", "local_code_defect", "upstream_dependency_defect", "environment_blocker", "insufficient_context"}},
			"summary": map[string]any{"type": "string"}, "tests": testEvidenceArraySchema(),
			"evidence": strictObjectSchema(
				[]string{},
				map[string]any{
					"files":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"failure_excerpt": map[string]any{"type": "string"},
				},
			), "delivery": deliveryEvidenceSchema(),
			"merge_request": mergeRequestResultSchema(), "defect": defectResultSchema(),
			"blocker_reason": map[string]any{"type": []string{"string", "null"}},
		},
	)
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
		"repos":              reposForUpstreamChild(context["repos"], task.TargetRepo, defect.Repository),
		"parent_task_id":     task.TaskID,
		"defect":             defect,
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
