package picli

import (
	"encoding/json"
	"strings"
	"testing"

	"sdlc-coordinator/internal/runner"
)

func TestParserCapturesFinalAssistantText(t *testing.T) {
	parser := newTestParser(nil)
	for _, line := range []string{
		`{"type":"session","version":3,"id":"session-1","cwd":"/workspaces/task"}`,
		`{"type":"message_end","message":{"role":"user","content":[{"type":"text","text":"do the task"}]}}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"working on it"}]}}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"{\"task_id\":\"t\",\"run_id\":\"r\"}"}],"stopReason":"stop"}}`,
		`{"type":"agent_settled","aborted":false}`,
	} {
		if err := parser.consume([]byte(line)); err != nil {
			t.Fatalf("consume %s: %v", line, err)
		}
	}
	if parser.finalText != `{"task_id":"t","run_id":"r"}` {
		t.Fatalf("final text = %q", parser.finalText)
	}
	if parser.aborted {
		t.Fatal("a settled run must not be marked aborted")
	}
}

func TestParserRecordsStopReasonError(t *testing.T) {
	parser := newTestParser(nil)
	line := `{"type":"message_end","message":{"role":"assistant","content":[],"stopReason":"error","errorMessage":"429: {\"error\":{\"code\":\"rate_limit_exceeded\",\"message\":\"slow down\"}}"}}`
	if err := parser.consume([]byte(line)); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if !strings.Contains(parser.failureMessage, "rate_limit_exceeded") {
		t.Fatalf("terminal error was not recorded: %q", parser.failureMessage)
	}
}

func TestParserRecordsFinalRetryError(t *testing.T) {
	parser := newTestParser(nil)
	line := `{"type":"auto_retry_end","success":false,"attempt":3,"finalError":"connection reset"}`
	if err := parser.consume([]byte(line)); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if !strings.Contains(parser.failureMessage, "connection reset") {
		t.Fatalf("final retry error was not recorded: %q", parser.failureMessage)
	}
}

func TestParserMarksAborted(t *testing.T) {
	parser := newTestParser(nil)
	if err := parser.consume([]byte(`{"type":"agent_settled","aborted":true}`)); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if !parser.aborted {
		t.Fatal("aborted agent_settled must mark the run aborted")
	}
}

func TestParserObservesAcceptedReceipt(t *testing.T) {
	parser := newTestParser(&runner.ResultSubmissionChannel{
		EndpointURL: "http://coordinator/mcp", BearerToken: "run-token",
	})
	parser.expectedTool = "submit_reviewer_result"
	line := `{"type":"tool_execution_end","toolCallId":"call_1","toolName":"mcp__coordinator__submit_reviewer_result","isError":false,"result":{"content":[{"type":"text","text":"{\"status\":\"accepted\",\"task_id\":\"task-pi\",\"run_id\":\"run-pi\",\"run_status\":\"succeeded\"}"}],"details":{"server":"coordinator","tool":"submit_reviewer_result"}}}`
	if err := parser.consume([]byte(line)); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if !parser.acceptedReceipt {
		t.Fatal("accepted receipt was not observed")
	}
	marker := parser.deliveryMarker()
	if marker == nil || !runner.IsDeliveryMarker(marker) {
		t.Fatalf("delivery marker missing: %s", marker)
	}
}

func TestParserIgnoresReceiptForWrongIdentityOrTool(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "wrong task",
			line: `{"type":"tool_execution_end","isError":false,"result":{"content":[{"type":"text","text":"{\"status\":\"accepted\",\"task_id\":\"task-other\",\"run_id\":\"run-pi\",\"run_status\":\"succeeded\"}"}],"details":{"server":"coordinator","tool":"submit_reviewer_result"}}}`,
		},
		{
			name: "wrong tool",
			line: `{"type":"tool_execution_end","isError":false,"result":{"content":[{"type":"text","text":"{\"status\":\"accepted\",\"task_id\":\"task-pi\",\"run_id\":\"run-pi\",\"run_status\":\"succeeded\"}"}],"details":{"server":"coordinator","tool":"submit_developer_result"}}}`,
		},
		{
			name: "tool error",
			line: `{"type":"tool_execution_end","isError":true,"result":{"content":[{"type":"text","text":"{\"status\":\"accepted\",\"task_id\":\"task-pi\",\"run_id\":\"run-pi\",\"run_status\":\"succeeded\"}"}],"details":{"server":"coordinator","tool":"submit_reviewer_result"}}}`,
		},
		{
			name: "not accepted",
			line: `{"type":"tool_execution_end","isError":false,"result":{"content":[{"type":"text","text":"{\"status\":\"rejected\",\"task_id\":\"task-pi\",\"run_id\":\"run-pi\",\"run_status\":\"running\"}"}],"details":{"server":"coordinator","tool":"submit_reviewer_result"}}}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parser := newTestParser(&runner.ResultSubmissionChannel{EndpointURL: "http://coordinator/mcp", BearerToken: "run-token"})
			parser.expectedTool = "submit_reviewer_result"
			if err := parser.consume([]byte(test.line)); err != nil {
				t.Fatalf("consume: %v", err)
			}
			if parser.acceptedReceipt || parser.deliveryMarker() != nil {
				t.Fatal("weak or foreign receipt must not be accepted")
			}
		})
	}
}

func TestParserEmitsObservabilityEvents(t *testing.T) {
	var events []capturedEvent
	parser := newTestParser(nil)
	parser.emit = func(eventType string, payload map[string]any) {
		events = append(events, capturedEvent{eventType: eventType, payload: payload})
	}
	for _, line := range []string{
		`{"type":"session","version":3,"id":"session-1","cwd":"/workspaces/task"}`,
		`{"type":"tool_execution_start","toolCallId":"call_1","toolName":"bash","args":{}}`,
		`{"type":"tool_execution_end","toolCallId":"call_1","toolName":"bash","isError":false,"result":{"content":[]}}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"done"}],"usage":{"input":10,"output":2,"cacheRead":3,"cacheWrite":0,"totalTokens":12,"cost":{"input":0}},"stopReason":"stop"}}`,
		`{"type":"auto_retry_start","attempt":1,"maxAttempts":3,"delayMs":2000,"errorMessage":"connection reset"}`,
		`{"type":"compaction_start","reason":"threshold"}`,
		`{"type":"compaction_end","reason":"threshold","aborted":false}`,
	} {
		if err := parser.consume([]byte(line)); err != nil {
			t.Fatalf("consume %s: %v", line, err)
		}
	}
	wantTypes := []string{"pi_session", "pi_progress", "pi_progress", "pi_usage", "pi_retry", "pi_compaction", "pi_compaction"}
	if len(events) != len(wantTypes) {
		t.Fatalf("unexpected event count %d: %#v", len(events), events)
	}
	for index, want := range wantTypes {
		if events[index].eventType != want {
			t.Fatalf("event %d = %q, want %q", index, events[index].eventType, want)
		}
	}
	if events[0].payload["session_id"] != "session-1" || events[0].payload["cwd"] != "/workspaces/task" {
		t.Fatalf("unexpected session payload: %#v", events[0].payload)
	}
	if events[2].payload["tool"] != "bash" || events[2].payload["status"] != "completed" || events[2].payload["item_type"] != "tool_call" {
		t.Fatalf("unexpected tool payload: %#v", events[2].payload)
	}
	usage, ok := events[3].payload["usage"].(map[string]any)
	if !ok || usage["input"] != float64(10) || usage["totalTokens"] != float64(12) || events[3].payload["stop_reason"] != "stop" {
		t.Fatalf("unexpected usage payload: %#v", events[3].payload)
	}
	if events[4].payload["attempt"] != float64(1) || events[4].payload["max_attempts"] != float64(3) || events[4].payload["delay_ms"] != float64(2000) {
		t.Fatalf("unexpected retry payload: %#v", events[4].payload)
	}
}

func TestParserEmitsRedactedErrorText(t *testing.T) {
	var events []capturedEvent
	parser := newTestParser(&runner.ResultSubmissionChannel{EndpointURL: "http://coordinator/mcp", BearerToken: "run-token-secret-value"})
	parser.provider.APIKey = "sk-secret-value"
	parser.emit = func(eventType string, payload map[string]any) {
		events = append(events, capturedEvent{eventType: eventType, payload: payload})
	}
	line := `{"type":"message_end","message":{"role":"assistant","content":[],"stopReason":"error","errorMessage":"request with sk-secret-value and run-token-secret-value failed"}}`
	if err := parser.consume([]byte(line)); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if strings.Contains(parser.failureMessage, "sk-secret-value") || strings.Contains(parser.failureMessage, "run-token-secret-value") {
		t.Fatalf("recorded failure leaked a secret: %q", parser.failureMessage)
	}
	for _, event := range events {
		raw, err := json.Marshal(event.payload)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "sk-secret-value") || strings.Contains(string(raw), "run-token-secret-value") {
			t.Fatalf("event leaked a secret: %s", raw)
		}
	}
}

type capturedEvent struct {
	eventType string
	payload   map[string]any
}

func newTestParser(channel *runner.ResultSubmissionChannel) *streamParser {
	return &streamParser{
		provider: runner.ProviderConfig{Name: "pi-cli"}, channel: channel,
		taskID: "task-pi", runID: "run-pi",
	}
}
