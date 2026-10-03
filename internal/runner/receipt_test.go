package runner_test

import (
	"testing"

	"sdlc-coordinator/internal/runner"
)

func TestParseMCPAcceptedReceiptRequiresStrictIdentityAndTerminalStatus(t *testing.T) {
	valid := `{"status":"accepted","max_attempts":5,"task_id":"task-1","run_id":"run-1","task_status":"succeeded","run_status":"succeeded","next_step":"stop"}`
	receipt, ok := runner.ParseMCPAcceptedReceipt(valid, "task-1", "run-1")
	if !ok {
		t.Fatal("accepted receipt with matching identity and terminal status must be parsed")
	}
	if receipt.TaskID != "task-1" || receipt.RunID != "run-1" || receipt.RunStatus != "succeeded" {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
	tests := []struct {
		name   string
		text   string
		taskID string
		runID  string
	}{
		{name: "empty expected identity", text: valid, taskID: "", runID: "run-1"},
		{name: "rejected status", text: `{"status":"rejected","task_id":"task-1","run_id":"run-1","run_status":"running"}`, taskID: "task-1", runID: "run-1"},
		{name: "already final status", text: `{"status":"already_final","task_id":"task-1","run_id":"run-1","run_status":"succeeded"}`, taskID: "task-1", runID: "run-1"},
		{name: "case variant key", text: `{"Status":"accepted","task_id":"task-1","run_id":"run-1","run_status":"succeeded"}`, taskID: "task-1", runID: "run-1"},
		{name: "task mismatch", text: valid, taskID: "task-other", runID: "run-1"},
		{name: "run mismatch", text: valid, taskID: "task-1", runID: "run-other"},
		{name: "run still running", text: `{"status":"accepted","task_id":"task-1","run_id":"run-1","run_status":"running"}`, taskID: "task-1", runID: "run-1"},
		{name: "run pending", text: `{"status":"accepted","task_id":"task-1","run_id":"run-1","run_status":"pending"}`, taskID: "task-1", runID: "run-1"},
		{name: "run finalizing", text: `{"status":"accepted","task_id":"task-1","run_id":"run-1","run_status":"finalizing"}`, taskID: "task-1", runID: "run-1"},
		{name: "missing run status", text: `{"status":"accepted","task_id":"task-1","run_id":"run-1"}`, taskID: "task-1", runID: "run-1"},
		{name: "non string status", text: `{"status":true,"task_id":"task-1","run_id":"run-1","run_status":"succeeded"}`, taskID: "task-1", runID: "run-1"},
		{name: "trailing garbage", text: valid + ` null`, taskID: "task-1", runID: "run-1"},
		{name: "not an object", text: `["accepted"]`, taskID: "task-1", runID: "run-1"},
		{name: "malformed", text: `{"status":"accepted"`, taskID: "task-1", runID: "run-1"},
		{name: "empty text", text: "", taskID: "task-1", runID: "run-1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, ok := runner.ParseMCPAcceptedReceipt(test.text, test.taskID, test.runID); ok {
				t.Fatalf("receipt must be refused: %s", test.text)
			}
		})
	}
}

func TestDeliveryMarkerRecognitionIsExact(t *testing.T) {
	marker := runner.NewDeliveryMarker("task-1", "run-1")
	if !runner.IsDeliveryMarker(marker) {
		t.Fatalf("generated marker was not recognized: %s", marker)
	}
	for _, raw := range []string{
		`{"verdict":"approved"}`,
		`{}`,
		`{"delivery_marker":null}`,
		`{"delivery_marker":{}}`,
		`{"delivery_marker":{"task_id":"task-1"}}`,
		`{"delivery_marker":{"task_id":"task-1","run_id":"run-1"}}`,
		`{"delivery_marker":{"task_id":"task-1","run_id":"run-1","reason":"other"}}`,
	} {
		if runner.IsDeliveryMarker([]byte(raw)) {
			t.Fatalf("non-marker payload was recognized as a delivery marker: %s", raw)
		}
	}
}
