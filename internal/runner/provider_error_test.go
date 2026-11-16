package runner_test

import (
	"testing"

	"sdlc-coordinator/internal/runner"
)

func TestExtractProviderErrorUsesOneFieldPriority(t *testing.T) {
	tests := []struct {
		name        string
		event       map[string]any
		wantCode    string
		wantMessage string
	}{
		{
			name:     "error code wins",
			event:    map[string]any{"error": map[string]any{"code": "invalid_request_error", "type": "ignored_type"}, "error_code": "ignored_top_level"},
			wantCode: "invalid_request_error",
		},
		{
			name:     "error type is read when code is absent",
			event:    map[string]any{"error": map[string]any{"type": "rate_limit_error", "message": "slow down"}},
			wantCode: "rate_limit_error", wantMessage: "slow down",
		},
		{
			name:     "top level error code is the fallback",
			event:    map[string]any{"error_code": "authentication_error"},
			wantCode: "authentication_error",
		},
		{
			name:     "embedded envelope code is read last",
			event:    map[string]any{"error": map[string]any{"message": `{"error":{"type":"invalid_request_error","message":"bad schema"}}`}},
			wantCode: "invalid_request_error", wantMessage: "bad schema",
		},
		{
			name:     "outer code wins over an embedded one",
			event:    map[string]any{"error": map[string]any{"code": "overloaded_error", "message": `{"error":{"code":"invalid_request_error","message":"inner"}}`}},
			wantCode: "overloaded_error", wantMessage: "inner",
		},
		{
			name:     "plain message is kept",
			event:    map[string]any{"error": map[string]any{"type": "overloaded_error", "message": "overloaded"}},
			wantCode: "overloaded_error", wantMessage: "overloaded",
		},
		{
			name:  "no error evidence",
			event: map[string]any{"type": "turn.completed"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := runner.ExtractProviderError(test.event)
			if got.Code != test.wantCode {
				t.Fatalf("code = %q, want %q", got.Code, test.wantCode)
			}
			if got.Message != test.wantMessage {
				t.Fatalf("message = %q, want %q", got.Message, test.wantMessage)
			}
		})
	}
}
