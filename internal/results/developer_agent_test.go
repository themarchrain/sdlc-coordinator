package results

import (
	"encoding/json"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
)

func TestDeveloperAgentContractAcceptsChangeReadyWithoutCommitEvidence(t *testing.T) {
	task, run := resultContractFixture("DeveloperWorker")
	contract := NewDeveloperAgentContract()
	input, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1","run_id":"run-1","outcome":"change_ready","summary":"implemented fix",
        "tests":[{"command":"go test ./...","passed":true,"summary":"passed"}],
        "merge_request":{"title":"Fix issue","description":"Implements the requested fix."}
    }`))
	if err != nil {
		t.Fatalf("decode developer agent result: %v", err)
	}
	if err := contract.Validate(task, run, input); err != nil {
		t.Fatalf("validate developer agent result: %v", err)
	}
	converted, err := contract.WithDelivery(input, DeliveryEvidence{
		CommitSHA: "abc123", ChangedFiles: []string{"main.go"}, DiffSummary: "1 file changed",
	})
	if err != nil {
		t.Fatalf("attach coordinator delivery: %v", err)
	}
	if converted.Delivery == nil || converted.Delivery.CommitSHA != "abc123" || converted.MergeRequest == nil {
		t.Fatalf("unexpected converted result: %#v", converted)
	}
}

func TestDeveloperAgentContractPreservesReworkAndBlockerRules(t *testing.T) {
	task, run := resultContractFixture("DeveloperWorker")
	task.Type = model.TaskTypeReworkMergeRequest
	contract := NewDeveloperAgentContract()

	valid, err := contract.DecodeStrict(json.RawMessage(`{
        "task_id":"task-1","run_id":"run-1","outcome":"rework_ready","summary":"updated branch",
        "tests":[{"command":"go test ./...","passed":true,"summary":"passed"}]
    }`))
	if err != nil || contract.Validate(task, run, valid) != nil {
		t.Fatalf("valid rework result rejected: input=%#v decode=%v validate=%v", valid, err, contract.Validate(task, run, valid))
	}

	for _, raw := range []string{
		`{"task_id":"task-1","run_id":"run-1","outcome":"change_ready","summary":"wrong","tests":[{"command":"go test ./...","passed":true,"summary":"passed"}],"merge_request":{"title":"new MR","description":"wrong"}}`,
		`{"task_id":"task-1","run_id":"run-1","outcome":"rework_ready","summary":"tests failed","tests":[{"command":"go test ./...","passed":false,"summary":"failed"}]}`,
		`{"task_id":"task-1","run_id":"run-1","outcome":"insufficient_context","summary":"blocked"}`,
	} {
		input, decodeErr := contract.DecodeStrict(json.RawMessage(raw))
		if decodeErr != nil {
			t.Fatalf("decode invalid fixture: %v", decodeErr)
		}
		if err := contract.Validate(task, run, input); err == nil {
			t.Fatalf("expected invalid result to be rejected: %s", raw)
		}
	}
}

func TestDeveloperAgentSchemaDoesNotAskAgentForDelivery(t *testing.T) {
	raw, err := json.Marshal(NewDeveloperAgentContract().InputSchema())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"delivery"`) || strings.Contains(string(raw), `"commit_sha"`) {
		t.Fatalf("agent schema still owns git delivery: %s", raw)
	}
}
