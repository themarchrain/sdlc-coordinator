package model

import (
	"testing"
	"time"
)

func TestExecutionExitFingerprintTracksEveryEvidenceField(t *testing.T) {
	finished := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	base := AgentExecution{
		RunID: "run-1", ExecutionID: "execution-1", Status: AgentExecutionStatusInterrupted,
		ErrorCode: "runner_restarted", RequestHash: "request-hash", FinishedAt: &finished,
	}
	fingerprint := ExecutionExitFingerprint(&base)
	if fingerprint == "" {
		t.Fatal("fingerprint is empty")
	}
	if again := ExecutionExitFingerprint(&base); again != fingerprint {
		t.Fatalf("fingerprint is not stable: %q != %q", again, fingerprint)
	}
	other := time.Date(2026, 11, 16, 10, 0, 1, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		mutate func(*AgentExecution)
	}{
		{name: "run", mutate: func(e *AgentExecution) { e.RunID = "run-2" }},
		{name: "execution", mutate: func(e *AgentExecution) { e.ExecutionID = "execution-2" }},
		{name: "status", mutate: func(e *AgentExecution) { e.Status = AgentExecutionStatusFailed }},
		{name: "error code", mutate: func(e *AgentExecution) { e.ErrorCode = "dispatch_not_accepted_after_run_terminal" }},
		{name: "finished at", mutate: func(e *AgentExecution) { e.FinishedAt = &other }},
		{name: "missing finish", mutate: func(e *AgentExecution) { e.FinishedAt = nil }},
		{name: "request hash", mutate: func(e *AgentExecution) { e.RequestHash = "other-hash" }},
	} {
		changed := base
		tc.mutate(&changed)
		if got := ExecutionExitFingerprint(&changed); got == fingerprint {
			t.Fatalf("%s change kept the fingerprint", tc.name)
		}
	}
	if got := ExecutionExitFingerprint(nil); got != "" {
		t.Fatalf("nil execution fingerprint = %q", got)
	}
}
