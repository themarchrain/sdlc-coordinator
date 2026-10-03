package results

import (
	"encoding/json"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
)

func commitMessageFixture(taskType string) (*model.Task, *model.Run) {
	task, run := resultContractFixture("DeveloperWorker")
	task.Type = taskType
	return task, run
}

func encodeJSON(t *testing.T, payload map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func validCommitMessage() string {
	return "fix: reject invalid repository state\n\n- Validate the state before delivery\n- Cover the failure path with a regression test"
}

func decodeDeveloperInput(t *testing.T, payload map[string]any) *DeveloperAgentResultInput {
	t.Helper()
	input, err := NewDeveloperAgentContract().DecodeStrict(encodeJSON(t, payload))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	agentInput, ok := input.(*DeveloperAgentResultInput)
	if !ok {
		t.Fatalf("unexpected input type %T", input)
	}
	return agentInput
}

func TestDeveloperAgentContractRequiresCommitMessageForChangeReady(t *testing.T) {
	task, run := commitMessageFixture("develop_from_issue")
	input := decodeDeveloperInput(t, map[string]any{
		"task_id": task.TaskID, "run_id": run.RunID, "outcome": "change_ready",
		"summary": "done", "merge_request": map[string]any{"title": "MR"}, "tests": []any{map[string]any{"command": "go test ./...", "passed": true, "summary": "ok"}},
	})
	if err := NewDeveloperAgentContract().Validate(task, run, input); err == nil || !strings.Contains(err.Error(), "commit_message") {
		t.Fatalf("change_ready without commit_message must be rejected: %v", err)
	}
}

func TestDeveloperAgentContractRequiresCommitMessageForReworkReady(t *testing.T) {
	task, run := commitMessageFixture(model.TaskTypeReworkMergeRequest)
	input := decodeDeveloperInput(t, map[string]any{
		"task_id": task.TaskID, "run_id": run.RunID, "outcome": "rework_ready",
		"summary": "done", "tests": []any{map[string]any{"command": "go test ./...", "passed": true, "summary": "ok"}},
	})
	if err := NewDeveloperAgentContract().Validate(task, run, input); err == nil || !strings.Contains(err.Error(), "commit_message") {
		t.Fatalf("rework_ready without commit_message must be rejected: %v", err)
	}
}

func TestDeveloperAgentContractAcceptsCompliantCommitMessage(t *testing.T) {
	cases := map[string]string{
		"title and bullets": validCommitMessage(),
		"title only":        "fix: reject invalid repository state",
		"prose body":        "feat: add feature flag\n\nThe flag gates the new path behind configuration.",
		"crlf normalized":   strings.ReplaceAll(validCommitMessage(), "\n", "\r\n"),
	}
	for name, message := range cases {
		task, run := commitMessageFixture("develop_from_issue")
		input := decodeDeveloperInput(t, map[string]any{
			"task_id": task.TaskID, "run_id": run.RunID, "outcome": "change_ready",
			"summary": "done", "merge_request": map[string]any{"title": "MR"}, "tests": []any{map[string]any{"command": "go test ./...", "passed": true, "summary": "ok"}},
			"commit_message": message,
		})
		if err := NewDeveloperAgentContract().Validate(task, run, input); err != nil {
			t.Fatalf("%s must be accepted: %v", name, err)
		}
	}
}

func TestDeveloperAgentContractRejectsMalformedCommitMessage(t *testing.T) {
	cases := map[string]string{
		"scope segment":      "fix(auth): reject invalid state\n\n- Body",
		"legacy ai prefix":   "ai: reject invalid state",
		"unknown type":       "feature: reject invalid state\n\n- Body",
		"empty summary":      "fix: ",
		"missing blank line": "fix: reject invalid state\n- Body without separator",
		"oversized title":    "fix: " + strings.Repeat("长", 100),
		"oversized message":  "fix: subject\n\n" + strings.Repeat("b", 9000),
		"control character":  "fix: reject\x07 invalid state",
		"stray carriage":     "fix: reject invalid state\r\n\rbody",
		"forged trailer":     validCommitMessage() + "\n\nCoordinator-Run-ID: run-forged",
		"blank message":      "   \n  ",
	}
	for name, message := range cases {
		task, run := commitMessageFixture("develop_from_issue")
		input := decodeDeveloperInput(t, map[string]any{
			"task_id": task.TaskID, "run_id": run.RunID, "outcome": "change_ready",
			"summary": "done", "merge_request": map[string]any{"title": "MR"}, "tests": []any{map[string]any{"command": "go test ./...", "passed": true, "summary": "ok"}},
			"commit_message": message,
		})
		if err := NewDeveloperAgentContract().Validate(task, run, input); err == nil || !strings.Contains(err.Error(), "commit_message") {
			t.Fatalf("%s must be rejected on commit_message: %v", name, err)
		}
	}
}

func TestDeveloperAgentContractRejectsCommitMessageOutsideReadyOutcomes(t *testing.T) {
	for _, outcome := range []string{"upstream_dependency_defect", "fix_failed", "environment_blocker", "insufficient_context"} {
		task, run := commitMessageFixture("develop_from_issue")
		payload := map[string]any{
			"task_id": task.TaskID, "run_id": run.RunID, "outcome": outcome,
			"summary": "done",
		}
		switch outcome {
		case "upstream_dependency_defect":
			payload["defect"] = map[string]any{"repository": "group/lib", "title": "upstream bug", "description": "blocks this task"}
		case "fix_failed", "environment_blocker", "insufficient_context":
			payload["blocker_reason"] = "blocked"
		}
		payload["commit_message"] = validCommitMessage()
		input := decodeDeveloperInput(t, payload)
		if err := NewDeveloperAgentContract().Validate(task, run, input); err == nil || !strings.Contains(err.Error(), "commit_message") {
			t.Fatalf("outcome %s with commit_message must be rejected: %v", outcome, err)
		}
	}
}

func TestNormalizeCommitMessageCanonicalForm(t *testing.T) {
	normalized, err := NormalizeCommitMessage("  fix: subject  \r\n\r\n- one\r\n- two  \n")
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if normalized != "fix: subject\n\n- one\n- two" {
		t.Fatalf("unexpected canonical form: %q", normalized)
	}
}
