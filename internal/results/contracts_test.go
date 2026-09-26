package results

import (
	"encoding/json"
	"fmt"
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

func reviewerApprovalFixture() (*model.Task, *model.Run) {
	task, run := resultContractFixture("ReviewerWorker")
	task.ContextJSON = `{
        "project":{"id":"15594","path":"group/project"},
        "merge_request":{"iid":12,"head_sha":"abc123","base_sha":"def456","source_branch":"agent/task-1","target_branch":"main","review_mode":"incremental"},
        "repos":[{"name":"project","url":"https://git.example/group/project.git","branch":"main","access":"readable"}]
    }`
	return task, run
}

func TestReviewerApprovalDerivesVerificationTask(t *testing.T) {
	contract := NewReviewerContract()
	task, run := reviewerApprovalFixture()
	input, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1","run_id":"run-1","verdict":"approved",
        "summary":"looks good","findings":[],"test_summary":"passed","comment_body":"approved, nice work"
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
	if len(result.NextTasks) != 1 {
		t.Fatalf("approval must derive exactly one verification task, got %d", len(result.NextTasks))
	}
	intent := result.NextTasks[0]
	if intent.Role != "TesterWorker" || intent.Type != "verify_merge_request" {
		t.Fatalf("unexpected intent role/type: %s/%s", intent.Role, intent.Type)
	}
	if intent.DedupeKey != "verify:group/project:12:abc123" {
		t.Fatalf("dedupe key must bind repo, MR and head SHA, got %q", intent.DedupeKey)
	}
	if intent.SourceRepo != "group/project" || intent.TargetRepo != "group/project" {
		t.Fatalf("unexpected intent repos: %s -> %s", intent.SourceRepo, intent.TargetRepo)
	}
	if intent.DependencyRelation != "verification" {
		t.Fatalf("unexpected dependency relation %q", intent.DependencyRelation)
	}
	if !intent.FireAndForget {
		t.Fatal("verification must derive fire-and-forget: the reviewer task must not wait upstream for it")
	}
	if intent.AllowedRepos.Readable == nil || len(intent.AllowedRepos.Readable) != 1 || intent.AllowedRepos.Readable[0] != "group/project" {
		t.Fatalf("verification must only read the review target: %#v", intent.AllowedRepos)
	}
	mergeRequest, _ := intent.Context["merge_request"].(map[string]any)
	if mergeRequest == nil || mergeRequest["head_sha"] != "abc123" {
		t.Fatalf("verification context must carry the reviewed head SHA: %#v", intent.Context["merge_request"])
	}
}

func TestReviewerNoNewChangesDoesNotAuthorizeVerificationByItself(t *testing.T) {
	contract := NewReviewerContract()
	task, run := reviewerApprovalFixture()
	input, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1","run_id":"run-1","verdict":"no_new_changes",
        "summary":"nothing reviewable in this push","findings":[],"test_summary":"n/a",
        "comment_body":"本轮无新增可审内容，不产生新的合并建议；以最近一次实质审查结论为准"
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
	if len(result.NextTasks) != 0 {
		t.Fatalf("no_new_changes preserves an earlier substantive verdict and must not authorize verification by itself, got %#v", result.NextTasks)
	}
}

func TestReviewerRejectedVerdictsDeriveNoVerificationTask(t *testing.T) {
	contract := NewReviewerContract()
	for _, verdict := range []string{"changes_requested", "human_required"} {
		task, run := reviewerApprovalFixture()
		input, err := contract.DecodeStrict(json.RawMessage(fmt.Sprintf(`{
            "task_id":"task-1","run_id":"run-1","verdict":%q,
            "summary":"needs work","findings":[],"test_summary":"failed","comment_body":"please fix"
        }`, verdict)))
		if err != nil {
			t.Fatalf("decode reviewer result: %v", err)
		}
		if err := contract.Validate(task, run, input); err != nil {
			t.Fatalf("validate reviewer result (%s): %v", verdict, err)
		}
		result, err := contract.Normalize(task, run, input)
		if err != nil {
			t.Fatalf("normalize reviewer result (%s): %v", verdict, err)
		}
		if len(result.NextTasks) != 0 {
			t.Fatalf("verdict %s must not derive verification tasks, got %#v", verdict, result.NextTasks)
		}
	}
}

func verificationTaskFixture() (*model.Task, *model.Run) {
	task, run := resultContractFixture("TesterWorker")
	task.ContextJSON = `{
        "project":{"id":"15594","path":"group/project"},
        "merge_request":{"iid":12,"head_sha":"abc123","source_branch":"agent/task-1","target_branch":"main"},
        "repos":[{"name":"project","url":"https://git.example/group/project.git","branch":"main","access":"readable"}]
    }`
	run.InputJSON = `{"verification":{"floor_command":"go test ./..."}}`
	return task, run
}

func TestTesterContractRejectsEvidenceThatOmitsPrescribedCommand(t *testing.T) {
	contract := NewTesterContract()
	task, run := verificationTaskFixture()
	run.InputJSON = `{"verification":{"floor_command":"go test ./...","replay_commands":["npm test -- --run"]}}`
	input, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1","run_id":"run-1","outcome":"verification_passed",
        "summary":"only the floor was executed",
        "tests":[{"command":"go test ./...","passed":true,"summary":"ok"}]
    }`))
	if err != nil {
		t.Fatalf("decode tester result: %v", err)
	}
	err = contract.Validate(task, run, input)
	if err == nil || !strings.Contains(err.Error(), "npm test -- --run") {
		t.Fatalf("expected omitted replay command to be rejected, got %v", err)
	}
}

func TestTesterContractAcceptsCompletePrescriptionRegardlessOfEvidenceOrder(t *testing.T) {
	contract := NewTesterContract()
	task, run := verificationTaskFixture()
	run.InputJSON = `{"verification":{"floor_command":"go test ./...","replay_commands":["npm test -- --run"]}}`
	input, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1","run_id":"run-1","outcome":"verification_passed",
        "summary":"the complete frozen prescription is green",
        "tests":[
            {"command":"npm test -- --run","passed":true,"summary":"189 passed"},
            {"command":"go test ./...","passed":true,"summary":"ok"}
        ]
    }`))
	if err != nil {
		t.Fatalf("decode tester result: %v", err)
	}
	if err := contract.Validate(task, run, input); err != nil {
		t.Fatalf("complete prescription must validate independent of evidence order: %v", err)
	}
}

func TestTesterContractVerificationPassedBindsHeadSHAAndWritesReport(t *testing.T) {
	contract := NewTesterContract()
	task, run := verificationTaskFixture()
	input, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1","run_id":"run-1","outcome":"verification_passed",
        "summary":"floor and replay commands are green",
        "tests":[{"command":"go test ./...","passed":true,"summary":"ok","attempts":1}]
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
	if result.Status != StatusSucceeded {
		t.Fatalf("verification result status=%q", result.Status)
	}
	if result.Artifacts["verification_verdict"] != "verification_passed" {
		t.Fatalf("verdict artifact=%v", result.Artifacts["verification_verdict"])
	}
	if result.Artifacts["verification_head_sha"] != "abc123" {
		t.Fatalf("head sha artifact=%v", result.Artifacts["verification_head_sha"])
	}
	if len(result.Writebacks) != 1 {
		t.Fatalf("expected one writeback, got %d", len(result.Writebacks))
	}
	writeback := result.Writebacks[0]
	if writeback.Kind != "create_merge_request_note" || writeback.ProjectID != "group/project" || writeback.MergeRequestIID != 12 {
		t.Fatalf("unexpected writeback identity: %#v", writeback)
	}
	for _, want := range []string{"abc123", "go test ./..."} {
		if !strings.Contains(writeback.Body, want) {
			t.Fatalf("report body must contain %q, got: %s", want, writeback.Body)
		}
	}
}

func TestTesterContractVerificationFailedKeepsTaskSucceededWithRedVerdict(t *testing.T) {
	contract := NewTesterContract()
	task, run := verificationTaskFixture()
	input, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1","run_id":"run-1","outcome":"verification_failed",
        "summary":"one command stays red after retries",
        "tests":[
            {"command":"go test ./...","passed":true,"summary":"ok","attempts":1},
            {"command":"make e2e","passed":false,"summary":"timeout on attempt 1, 2, 3","attempts":3}
        ]
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
	if result.Status != StatusSucceeded {
		t.Fatalf("a red verdict still completes the verification task, got %q", result.Status)
	}
	if result.Artifacts["verification_verdict"] != "verification_failed" {
		t.Fatalf("verdict artifact=%v", result.Artifacts["verification_verdict"])
	}
	if !strings.Contains(result.Writebacks[0].Body, "未通过") {
		t.Fatalf("report body must carry the failed verdict, got: %s", result.Writebacks[0].Body)
	}
}

func TestTesterContractVerificationFailedRequiresTestsEvidence(t *testing.T) {
	contract := NewTesterContract()
	task, run := verificationTaskFixture()
	input, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1","run_id":"run-1","outcome":"verification_failed",
        "summary":"red without evidence"
    }`))
	if err != nil {
		t.Fatalf("decode tester result: %v", err)
	}
	err = contract.Validate(task, run, input)
	if err == nil || !strings.Contains(err.Error(), "tests evidence is required") {
		t.Fatalf("expected missing tests evidence error, got %v", err)
	}
}

func TestTesterContractVerificationPassedRejectsRedEntry(t *testing.T) {
	contract := NewTesterContract()
	task, run := verificationTaskFixture()
	input, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1","run_id":"run-1","outcome":"verification_passed",
        "summary":"claiming green with a red entry",
        "tests":[{"command":"go test ./...","passed":false,"summary":"flaky"}]
    }`))
	if err != nil {
		t.Fatalf("decode tester result: %v", err)
	}
	err = contract.Validate(task, run, input)
	if err == nil || !strings.Contains(err.Error(), "must pass") {
		t.Fatalf("expected passed enforcement error, got %v", err)
	}
}

func TestTesterContractBlockersRequireBlockerReason(t *testing.T) {
	contract := NewTesterContract()
	task, run := verificationTaskFixture()
	missing, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1","run_id":"run-1","outcome":"environment_blocker",
        "summary":"runner cannot start"
    }`))
	if err != nil {
		t.Fatalf("decode tester result: %v", err)
	}
	err = contract.Validate(task, run, missing)
	if err == nil || !strings.Contains(err.Error(), "blocker_reason is required") {
		t.Fatalf("expected blocker reason requirement, got %v", err)
	}

	reason := "docker daemon unreachable"
	blocked, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1","run_id":"run-1","outcome":"environment_blocker",
        "summary":"runner cannot start","blocker_reason":"docker daemon unreachable"
    }`))
	if err != nil {
		t.Fatalf("decode tester result: %v", err)
	}
	if err := contract.Validate(task, run, blocked); err != nil {
		t.Fatalf("validate tester result: %v", err)
	}
	result, err := contract.Normalize(task, run, blocked)
	if err != nil {
		t.Fatalf("normalize tester result: %v", err)
	}
	// 环境阻塞走失败重试路径(健壮性审查修复轮定稿):orchestrator 侧给
	// retry_pending,预算耗尽才转人工;判停会静默杀死环境瞬态下的验证链。
	if result.Status != StatusFailed || result.BlockerReason == nil || *result.BlockerReason != reason {
		t.Fatalf("environment blocker must enter the failure retry path with reason, got %#v", result)
	}

	unclear, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1","run_id":"run-1","outcome":"insufficient_context",
        "summary":"no verification commands available","blocker_reason":"no prescription"
    }`))
	if err != nil {
		t.Fatalf("decode tester result: %v", err)
	}
	if err := contract.Validate(task, run, unclear); err != nil {
		t.Fatalf("validate tester result: %v", err)
	}
	result, err = contract.Normalize(task, run, unclear)
	if err != nil {
		t.Fatalf("normalize tester result: %v", err)
	}
	if result.Status != StatusHumanRequired {
		t.Fatalf("insufficient context must require human, got %q", result.Status)
	}
}

func TestTesterContractEnvironmentBlockerWalksFailureRetryPath(t *testing.T) {
	contract := NewTesterContract()
	task, run := verificationTaskFixture()
	blocked, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1","run_id":"run-1","outcome":"environment_blocker",
        "summary":"docker daemon unreachable","blocker_reason":"docker daemon unreachable"
    }`))
	if err != nil {
		t.Fatalf("decode tester result: %v", err)
	}
	if err := contract.Validate(task, run, blocked); err != nil {
		t.Fatalf("validate tester result: %v", err)
	}
	result, err := contract.Normalize(task, run, blocked)
	if err != nil {
		t.Fatalf("normalize tester result: %v", err)
	}
	// 验证自身没跑成=无证据,按用户拍板走失败重试路径:orchestrator 侧
	// policy.NextFailureStatus 给 retry_pending,预算耗尽才转人工。
	if result.Status != StatusFailed {
		t.Fatalf("environment blocker must enter the failure retry path, got %q", result.Status)
	}
	if result.Artifacts["verification_verdict"] != "environment_blocker" {
		t.Fatalf("verdict artifact=%v", result.Artifacts["verification_verdict"])
	}
	if result.Artifacts["verification_head_sha"] != "abc123" {
		t.Fatalf("head sha artifact=%v", result.Artifacts["verification_head_sha"])
	}
}

func TestTesterReportBodyEscapesTableCells(t *testing.T) {
	contract := NewTesterContract()
	task, run := verificationTaskFixture()
	input, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1","run_id":"run-1","outcome":"verification_passed",
        "summary":"floor is green",
        "tests":[{"command":"go test ./...","passed":true,"summary":"cell a|b and rare|cell"}]
    }`))
	if err != nil {
		t.Fatalf("decode tester result: %v", err)
	}
	result, err := contract.Normalize(task, run, input)
	if err != nil {
		t.Fatalf("normalize tester result: %v", err)
	}
	body := result.Writebacks[0].Body
	if strings.Contains(body, "cell a|b and") || strings.Contains(body, "rare|cell") {
		t.Fatalf("raw pipes must be escaped inside table cells, got: %s", body)
	}
	if !strings.Contains(body, `a\|b`) || !strings.Contains(body, `rare\|cell`) {
		t.Fatalf("escaped cells expected, got: %s", body)
	}
}

func TestTesterContractSchemaDeclaresVerificationOutcomes(t *testing.T) {
	schema := NewTesterContract().InputSchema()
	properties := schema["properties"].(map[string]any)
	outcome := properties["outcome"].(map[string]any)
	values := outcome["enum"].([]string)
	for _, want := range []string{"verification_passed", "verification_failed", "environment_blocker", "insufficient_context"} {
		if !slices.Contains(values, want) {
			t.Fatalf("tester outcome schema must expose %s, got %#v", want, values)
		}
	}
	if slices.Contains(values, "test_patch_passed") {
		t.Fatalf("legacy outcome must not survive the rework, got %#v", values)
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
		{name: "tester", contract: NewTesterContract(), fields: []string{"tests"}},
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
