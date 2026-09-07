package operationmarker

import "testing"

func TestNewUsesAgentCoordinatorNamespace(t *testing.T) {
	if got := New("codehost:task-1:run-1:0:create_merge_request_note"); got != "<!-- agent-coordinator-operation:codehost:task-1:run-1:0:create_merge_request_note -->" {
		t.Fatalf("unexpected operation marker %q", got)
	}
}

func TestContainsRecognizesCurrentAndLegacyCoordinatorMarkers(t *testing.T) {
	for _, body := range []string{
		"review\n\n<!-- agent-coordinator-operation:codehost:test -->",
		"review\n\n<!-- sdlc-coordinator-operation:codehost:test -->",
	} {
		if !Contains(body) {
			t.Fatalf("coordinator marker was not recognized: %q", body)
		}
	}
	if Contains("<!-- another-system-operation:codehost:test -->") {
		t.Fatal("unrelated marker must not be recognized")
	}
}
