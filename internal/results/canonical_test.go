package results

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCanonicalizeProducesStableJSONAndHash(t *testing.T) {
	firstJSON, firstHash, err := Canonicalize(CanonicalResult{
		TaskID:    "task-1",
		RunID:     "run-1",
		Status:    StatusSucceeded,
		Summary:   "completed",
		Artifacts: map[string]any{"b": float64(2), "a": float64(1)},
	})
	if err != nil {
		t.Fatalf("canonicalize first result: %v", err)
	}

	secondJSON, secondHash, err := Canonicalize(CanonicalResult{
		TaskID:    "task-1",
		RunID:     "run-1",
		Status:    StatusSucceeded,
		Summary:   "completed",
		Artifacts: map[string]any{"a": float64(1), "b": float64(2)},
	})
	if err != nil {
		t.Fatalf("canonicalize second result: %v", err)
	}

	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("expected stable canonical JSON, got %s and %s", firstJSON, secondJSON)
	}
	if firstHash != secondHash {
		t.Fatalf("expected stable hash, got %s and %s", firstHash, secondHash)
	}
	if len(firstHash) != 64 {
		t.Fatalf("expected SHA-256 hex hash, got %q", firstHash)
	}
}

func TestDecodeCanonicalStrictRejectsUnknownField(t *testing.T) {
	_, err := DecodeCanonicalStrict(json.RawMessage(`{
        "task_id":"task-1",
        "run_id":"run-1",
        "status":"succeeded",
        "summary":"done",
        "invented":"not allowed"
    }`))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected strict unknown field error, got %v", err)
	}
}

func TestCanonicalizeRejectsMissingIdentity(t *testing.T) {
	_, _, err := Canonicalize(CanonicalResult{Status: StatusSucceeded})
	if err == nil || !strings.Contains(err.Error(), "task_id") {
		t.Fatalf("expected task_id validation error, got %v", err)
	}
}

func TestCanonicalizeRejectsInvalidStatus(t *testing.T) {
	_, _, err := Canonicalize(CanonicalResult{
		TaskID: "task-1",
		RunID:  "run-1",
		Status: ResultStatus("invented"),
	})
	if err == nil || !strings.Contains(err.Error(), "invalid result status") {
		t.Fatalf("expected invalid status error, got %v", err)
	}
}
