package results

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
)

func TestDeveloperContractStrictDecodeRejectsCoordinatorOwnedField(t *testing.T) {
	contract := NewDeveloperContract()
	_, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1",
        "run_id":"run-1",
        "outcome":"change_ready",
        "summary":"fixed",
        "project_id":"group/wrong"
    }`))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestDeveloperContractDerivesMergeRequestTargetFromTask(t *testing.T) {
	contract := NewDeveloperContract()
	task, run := resultContractFixture("DeveloperWorker")
	task.DeliveryBranch = "alice/issue-13-20260727"
	input, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1",
        "run_id":"run-1",
        "outcome":"change_ready",
        "summary":"fixed normalize_status",
        "delivery":{"commit_sha":"abc123","changed_files":["status.go"],"diff_summary":"normalize return value"},
        "tests":[{"command":"go test ./...","passed":true,"summary":"all passed"}],
        "merge_request":{"title":"fix: normalize status","description":"Fixes issue"}
    }`))
	if err != nil {
		t.Fatalf("decode developer result: %v", err)
	}
	if err := contract.Validate(task, run, input); err != nil {
		t.Fatalf("validate developer result: %v", err)
	}

	result, err := contract.Normalize(task, run, input)
	if err != nil {
		t.Fatalf("normalize developer result: %v", err)
	}
	if len(result.Writebacks) != 1 {
		t.Fatalf("expected one writeback, got %d", len(result.Writebacks))
	}
	writeback := result.Writebacks[0]
	if writeback.Kind != "create_merge_request" || writeback.ProjectID != "group/project" {
		t.Fatalf("unexpected writeback identity: %#v", writeback)
	}
	if writeback.SourceBranch != "alice/issue-13-20260727" || writeback.TargetBranch != "main" {
		t.Fatalf("unexpected derived branches: %#v", writeback)
	}
}

func TestDeveloperChangeReadyAppendsClosingReferenceForSameProjectGitLabIssue(t *testing.T) {
	contract := NewDeveloperContract()
	task, run := resultContractFixture("DeveloperWorker")
	task.ContextJSON = `{
        "event":{"source":"gitlab"},
        "project":{"id":"15594","path":"group/project"},
        "issue":{"iid":42},
        "repos":[{"name":"project","branch":"main","access":"writable"}]
    }`
	input := developerChangeReadyInput(t, contract, "Implements the requested fix.")

	result, err := contract.Normalize(task, run, input)
	if err != nil {
		t.Fatalf("normalize developer result: %v", err)
	}
	description := result.Writebacks[0].Description
	if !strings.Contains(description, "Closes #42") {
		t.Fatalf("expected same-project GitLab closing reference, got %q", description)
	}
	if strings.Count(description, "Closes #42") != 1 {
		t.Fatalf("expected exactly one closing reference, got %q", description)
	}

	input = developerChangeReadyInput(t, contract, description)
	result, err = contract.Normalize(task, run, input)
	if err != nil {
		t.Fatalf("normalize developer result with existing closing reference: %v", err)
	}
	if count := strings.Count(result.Writebacks[0].Description, "Closes #42"); count != 1 {
		t.Fatalf("expected existing closing reference to remain unique, got %d in %q", count, result.Writebacks[0].Description)
	}
}

func TestDeveloperChangeReadyDoesNotAppendClosingReferenceForOtherProjectOrSource(t *testing.T) {
	contract := NewDeveloperContract()
	t.Run("cross project", func(t *testing.T) {
		task, run := resultContractFixture("DeveloperWorker")
		task.ContextJSON = `{
            "event":{"source":"gitlab"},
            "project":{"path":"group/source"},
            "issue":{"iid":42},
            "repos":[{"name":"project","branch":"main","access":"writable"}]
        }`
		input := developerChangeReadyInput(t, contract, "Implements the requested fix.")

		result, err := contract.Normalize(task, run, input)
		if err != nil {
			t.Fatalf("normalize developer result: %v", err)
		}
		if strings.Contains(result.Writebacks[0].Description, "Closes #42") {
			t.Fatalf("unexpected closing reference for cross project: %q", result.Writebacks[0].Description)
		}
	})

	t.Run("gitea also closes issue", func(t *testing.T) {
		task, run := resultContractFixture("DeveloperWorker")
		task.ContextJSON = `{
            "event":{"source":"gitea"},
            "project":{"path":"group/project"},
            "issue":{"iid":42},
            "repos":[{"name":"project","branch":"main","access":"writable"}]
        }`
		input := developerChangeReadyInput(t, contract, "Implements the requested fix.")

		result, err := contract.Normalize(task, run, input)
		if err != nil {
			t.Fatalf("normalize developer result: %v", err)
		}
		if !strings.Contains(result.Writebacks[0].Description, "Closes #42") {
			t.Fatalf("expected gitea closing reference: %q", result.Writebacks[0].Description)
		}
	})
}

func TestDeveloperContractRejectsFailedTestForChangeReady(t *testing.T) {
	contract := NewDeveloperContract()
	task, run := resultContractFixture("DeveloperWorker")
	input, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1","run_id":"run-1","outcome":"change_ready","summary":"fixed",
        "delivery":{"commit_sha":"abc123","changed_files":["status.go"],"diff_summary":"changed"},
        "tests":[{"command":"go test ./...","passed":false,"summary":"failed"}],
        "merge_request":{"title":"fix: status","description":"Fix"}
    }`))
	if err != nil {
		t.Fatalf("decode developer result: %v", err)
	}
	if err := contract.Validate(task, run, input); err == nil || !strings.Contains(err.Error(), "tests") {
		t.Fatalf("expected failed test validation error, got %v", err)
	}
}

func TestDeveloperReworkReadyProducesArtifactsWithoutWriteback(t *testing.T) {
	contract := NewDeveloperContract()
	task, run := resultContractFixture("DeveloperWorker")
	task.Type = model.TaskTypeReworkMergeRequest
	task.DeliveryBranch = "developer/issue-19-20260806"
	input, err := contract.DecodeStrict(json.RawMessage(`{
		"task_id":"task-1","run_id":"run-1","outcome":"rework_ready","summary":"fixed requested null handling",
		"delivery":{"commit_sha":"def456","changed_files":["status.go","status_test.go"],"diff_summary":"handle nil and blank values"},
		"tests":[{"command":"go test ./...","passed":true,"summary":"all passed"}]
	}`))
	if err != nil {
		t.Fatalf("decode developer rework result: %v", err)
	}
	if err := contract.Validate(task, run, input); err != nil {
		t.Fatalf("validate developer rework result: %v", err)
	}
	result, err := contract.Normalize(task, run, input)
	if err != nil {
		t.Fatalf("normalize developer rework result: %v", err)
	}
	if result.Status != StatusSucceeded || result.Artifacts["commit_sha"] != "def456" {
		t.Fatalf("unexpected canonical rework result: %#v", result)
	}
	if len(result.Writebacks) != 0 || len(result.Effects) != 0 || len(result.NextTasks) != 0 {
		t.Fatalf("rework_ready must not create coordinator side effects: %#v", result)
	}
}

func TestDeveloperContractEnforcesReworkOutcomeByTaskType(t *testing.T) {
	contract := NewDeveloperContract()
	tests := []struct {
		name     string
		taskType string
		payload  string
		errorHas string
	}{
		{
			name:     "ordinary task rejects rework_ready",
			taskType: "develop_from_issue",
			payload:  `{"task_id":"task-1","run_id":"run-1","outcome":"rework_ready","summary":"done","delivery":{"commit_sha":"abc","changed_files":["a.go"],"diff_summary":"changed"},"tests":[{"command":"go test ./...","passed":true,"summary":"passed"}]}`,
			errorHas: "only valid for rework_merge_request",
		},
		{
			name:     "rework task rejects change_ready",
			taskType: model.TaskTypeReworkMergeRequest,
			payload:  `{"task_id":"task-1","run_id":"run-1","outcome":"change_ready","summary":"done","delivery":{"commit_sha":"abc","changed_files":["a.go"],"diff_summary":"changed"},"tests":[{"command":"go test ./...","passed":true,"summary":"passed"}],"merge_request":{"title":"new MR","description":"wrong"}}`,
			errorHas: "does not allow change_ready",
		},
		{
			name:     "rework task rejects upstream defect",
			taskType: model.TaskTypeReworkMergeRequest,
			payload:  `{"task_id":"task-1","run_id":"run-1","outcome":"upstream_dependency_defect","summary":"upstream","defect":{"repository":"group/upstream","title":"bug","description":"broken"}}`,
			errorHas: "does not allow upstream_dependency_defect",
		},
		{
			name:     "rework_ready rejects merge request",
			taskType: model.TaskTypeReworkMergeRequest,
			payload:  `{"task_id":"task-1","run_id":"run-1","outcome":"rework_ready","summary":"done","delivery":{"commit_sha":"abc","changed_files":["a.go"],"diff_summary":"changed"},"tests":[{"command":"go test ./...","passed":true,"summary":"passed"}],"merge_request":{"title":"new MR","description":"wrong"}}`,
			errorHas: "must not include merge_request or defect",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, run := resultContractFixture("DeveloperWorker")
			task.Type = tt.taskType
			input, err := contract.DecodeStrict(json.RawMessage(tt.payload))
			if err != nil {
				t.Fatalf("decode result: %v", err)
			}
			err = contract.Validate(task, run, input)
			if err == nil || !strings.Contains(err.Error(), tt.errorHas) {
				t.Fatalf("expected error containing %q, got %v", tt.errorHas, err)
			}
		})
	}
}

func TestDeveloperContractUnsupportedOutcomeListsAllowedValues(t *testing.T) {
	contract := NewDeveloperContract()
	task, run := resultContractFixture("DeveloperWorker")
	input, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1","run_id":"run-1","outcome":"completed","summary":"implemented"
    }`))
	if err != nil {
		t.Fatalf("decode developer result: %v", err)
	}

	err = contract.Validate(task, run, input)
	if err == nil {
		t.Fatal("expected unsupported outcome to be rejected")
	}
	for _, outcome := range []string{"change_ready", "upstream_dependency_defect", "fix_failed", "environment_blocker", "insufficient_context"} {
		if !strings.Contains(err.Error(), outcome) {
			t.Fatalf("unsupported-outcome error must list %q, got %v", outcome, err)
		}
	}
}

func TestReviewerContractDerivesMergeRequestNoteTargetFromContext(t *testing.T) {
	contract := NewReviewerContract()
	task, run := resultContractFixture("ReviewerWorker")
	input, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1","run_id":"run-1","verdict":"approved",
        "summary":"looks good","findings":[],"test_summary":"tests passed",
        "comment_body":"Review approved."
    }`))
	if err != nil {
		t.Fatalf("decode reviewer result: %v", err)
	}
	if err := contract.Validate(task, run, input); err != nil {
		t.Fatalf("validate reviewer result: %v", err)
	}
	result, err := contract.Normalize(task, run, input)
	if err != nil {
		t.Fatalf("normalize reviewer result: %v", err)
	}
	writeback := result.Writebacks[0]
	if writeback.ProjectID != "group/project" || writeback.MergeRequestIID != 12 || writeback.Kind != "create_merge_request_note" {
		t.Fatalf("unexpected reviewer writeback: %#v", writeback)
	}
	if result.Artifacts["review_verdict"] != "approved" {
		t.Fatalf("unexpected review artifact: %#v", result.Artifacts)
	}
}

func TestReviewerContractAcceptsNoNewChangesForIncrementalReview(t *testing.T) {
	contract := NewReviewerContract()
	task, run := resultContractFixture("ReviewerWorker")
	task.ContextJSON = strings.Replace(task.ContextJSON, `"target_branch":"main"`, `"target_branch":"main","review_mode":"incremental"`, 1)
	input, err := contract.DecodeStrict(json.RawMessage(`{
		"task_id":"task-1","run_id":"run-1","verdict":"no_new_changes",
		"summary":"target branch sync introduced no reviewable changes","findings":[],
		"test_summary":"not run because there is no reviewable diff",
		"comment_body":"本轮无新增可审内容，不产生新的合并建议；以最近一次实质审查结论为准。"
	}`))
	if err != nil {
		t.Fatalf("decode reviewer result: %v", err)
	}
	if err := contract.Validate(task, run, input); err != nil {
		t.Fatalf("validate no-new-changes reviewer result: %v", err)
	}
	result, err := contract.Normalize(task, run, input)
	if err != nil {
		t.Fatalf("normalize no-new-changes reviewer result: %v", err)
	}
	if result.Status != StatusSucceeded || result.Artifacts["review_verdict"] != "no_new_changes" {
		t.Fatalf("unexpected normalized result: %#v", result)
	}
	if len(result.Writebacks) != 1 || result.Writebacks[0].Kind != "create_merge_request_note" {
		t.Fatalf("expected one merge request note, got %#v", result.Writebacks)
	}
}

func TestReviewerContractRejectsNoNewChangesOutsideIncrementalReview(t *testing.T) {
	contract := NewReviewerContract()
	task, run := resultContractFixture("ReviewerWorker")
	input, err := contract.DecodeStrict(json.RawMessage(`{
		"task_id":"task-1","run_id":"run-1","verdict":"no_new_changes",
		"summary":"no changes","findings":[],"test_summary":"not run",
		"comment_body":"no changes"
	}`))
	if err != nil {
		t.Fatalf("decode reviewer result: %v", err)
	}
	if err := contract.Validate(task, run, input); err == nil || !strings.Contains(err.Error(), "incremental") {
		t.Fatalf("expected full review to reject no_new_changes, got %v", err)
	}
}

func TestReviewerContractRejectsNoNewChangesWithFindings(t *testing.T) {
	contract := NewReviewerContract()
	task, run := resultContractFixture("ReviewerWorker")
	task.ContextJSON = strings.Replace(task.ContextJSON, `"target_branch":"main"`, `"target_branch":"main","review_mode":"incremental"`, 1)
	input, err := contract.DecodeStrict(json.RawMessage(`{
		"task_id":"task-1","run_id":"run-1","verdict":"no_new_changes",
		"summary":"no changes","findings":[{"message":"unexpected issue"}],"test_summary":"not run",
		"comment_body":"no changes"
	}`))
	if err != nil {
		t.Fatalf("decode reviewer result: %v", err)
	}
	if err := contract.Validate(task, run, input); err == nil || !strings.Contains(err.Error(), "findings") {
		t.Fatalf("expected no_new_changes with findings to be rejected, got %v", err)
	}
}

func TestReviewerContractSchemaExposesNoNewChangesVerdict(t *testing.T) {
	schema := NewReviewerContract().InputSchema()
	properties := schema["properties"].(map[string]any)
	verdict := properties["verdict"].(map[string]any)
	values := verdict["enum"].([]string)
	if !slices.Contains(values, "no_new_changes") {
		t.Fatalf("reviewer verdict schema must expose no_new_changes, got %#v", values)
	}
}

func TestTesterContractBuildsUpstreamDependencyResult(t *testing.T) {
	contract := NewTesterContract()
	task, run := resultContractFixture("TesterWorker")
	task.SourceRepo = "group/framework"
	input, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1","run_id":"run-1","outcome":"upstream_dependency_defect",
        "summary":"downstream failure belongs to framework",
        "tests":[{"command":"python -m unittest","passed":false,"summary":"normalization failed"}],
        "evidence":{"files":["framework/status.py"],"failure_excerpt":"expected ok got OK"},
        "defect":{"repository":"group/framework","title":"fix status normalization","description":"normalize to lowercase","dependency_relation":"upstream_fix"}
    }`))
	if err != nil {
		t.Fatalf("decode tester result: %v", err)
	}
	if err := contract.Validate(task, run, input); err != nil {
		t.Fatalf("validate tester result: %v", err)
	}
	result, err := contract.Normalize(task, run, input)
	if err != nil {
		t.Fatalf("normalize tester result: %v", err)
	}
	if len(result.NextTasks) != 1 || result.NextTasks[0].Role != "DeveloperWorker" {
		t.Fatalf("unexpected next tasks: %#v", result.NextTasks)
	}
	if result.NextTasks[0].TargetRepo != "group/framework" || result.NextTasks[0].DependencyRelation != "upstream_fix" {
		t.Fatalf("unexpected dependency task: %#v", result.NextTasks[0])
	}
	if len(result.Writebacks) != 1 || result.Writebacks[0].Kind != "create_issue" || result.Writebacks[0].ProjectID != "group/framework" {
		t.Fatalf("unexpected upstream issue intent: %#v", result.Writebacks)
	}
}

func TestContractRejectsRoleMismatch(t *testing.T) {
	contract := NewReviewerContract()
	task, run := resultContractFixture("DeveloperWorker")
	input, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1","run_id":"run-1","verdict":"approved",
        "summary":"ok","findings":[],"test_summary":"passed","comment_body":"approved"
    }`))
	if err != nil {
		t.Fatalf("decode reviewer result: %v", err)
	}
	if err := contract.Validate(task, run, input); err == nil || !strings.Contains(err.Error(), "ReviewerWorker") {
		t.Fatalf("expected role mismatch, got %v", err)
	}
}

func TestDefaultContractRegistryContainsEveryWorkerTool(t *testing.T) {
	registry := NewDefaultContractRegistry()
	for _, toolName := range []string{SubmitTesterResultTool, SubmitDeveloperResultTool, SubmitReviewerResultTool} {
		if _, ok := registry.Get(toolName); !ok {
			t.Fatalf("missing contract for %s", toolName)
		}
	}
}

func TestRoleContractSchemasDescribeNestedObjectsStrictly(t *testing.T) {
	tests := []struct {
		name     string
		contract RoleResultContract
		fields   []string
	}{
		{name: "developer", contract: NewDeveloperContract(), fields: []string{"delivery", "tests", "merge_request", "defect"}},
		{name: "tester", contract: NewTesterContract(), fields: []string{"tests", "evidence", "delivery", "merge_request", "defect"}},
		{name: "reviewer", contract: NewReviewerContract(), fields: []string{"findings"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := tt.contract.InputSchema()
			properties, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("schema properties are missing: %#v", schema)
			}
			for _, field := range tt.fields {
				fieldSchema, ok := properties[field].(map[string]any)
				if !ok {
					t.Fatalf("schema for %s is missing", field)
				}
				objectSchema := fieldSchema
				if fieldSchema["type"] == "array" {
					objectSchema, _ = fieldSchema["items"].(map[string]any)
				}
				if objectSchema == nil || objectSchema["additionalProperties"] != false {
					t.Fatalf("schema for %s must reject unknown nested fields: %#v", field, fieldSchema)
				}
			}
		})
	}
}

func resultContractFixture(role string) (*model.Task, *model.Run) {
	runID := "run-1"
	return &model.Task{
			TaskID:       "task-1",
			Role:         role,
			TargetRepo:   "group/project",
			SourceRepo:   "group/project",
			Status:       model.TaskStatusRunning,
			CurrentRunID: &runID,
			ContextJSON: `{
            "workspace_strategy":"agent_managed_clone",
            "project":{"id":15594,"path":"group/project"},
            "merge_request":{"iid":12,"source_branch":"agent/task-1","target_branch":"main"},
            "repos":[
                {"name":"project","url":"https://git.example/group/project.git","branch":"main","access":"writable"},
                {"name":"framework","url":"https://git.example/group/framework.git","branch":"main","access":"readable"}
            ]
        }`,
		}, &model.Run{
			RunID:      runID,
			TaskID:     "task-1",
			WorkerRole: role,
			Status:     model.RunStatusRunning,
		}
}

func developerChangeReadyInput(t *testing.T, contract DeveloperContract, description string) RoleResultInput {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"task_id": "task-1",
		"run_id":  "run-1",
		"outcome": "change_ready",
		"summary": "fixed requested behavior",
		"delivery": map[string]any{
			"commit_sha":    "abc123",
			"changed_files": []string{"status.go"},
			"diff_summary":  "normalize return value",
		},
		"tests":         []map[string]any{{"command": "go test ./...", "passed": true, "summary": "all passed"}},
		"merge_request": map[string]any{"title": "fix: requested behavior", "description": description},
	})
	if err != nil {
		t.Fatalf("marshal developer result: %v", err)
	}
	input, err := contract.DecodeStrict(raw)
	if err != nil {
		t.Fatalf("decode developer result: %v", err)
	}
	return input
}
