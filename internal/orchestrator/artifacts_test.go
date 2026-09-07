package orchestrator

import (
	"encoding/json"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
)

func TestBuildArtifactsFromResultMapsKnownArtifacts(t *testing.T) {
	payload := results.CanonicalResult{
		TaskID: "task-artifact-builder",
		RunID:  "run-artifact-builder",
		Artifacts: map[string]any{
			"mr_url":       "https://gitlab.example/group/project/-/merge_requests/1",
			"branch":       "feature/fix-framework",
			"test_results": map[string]any{"passed": true, "total": float64(12)},
			"notes_url":    "https://gitlab.example/group/project/-/issues/3#note_1",
		},
	}

	artifacts, err := BuildArtifactsFromResult(payload)
	if err != nil {
		t.Fatalf("build artifacts: %v", err)
	}

	if len(artifacts) != 4 {
		t.Fatalf("expected four artifacts, got %d", len(artifacts))
	}

	expectedOrder := []string{"branch", "mr_url", "notes_url", "test_results"}
	for i, expectedName := range expectedOrder {
		if artifacts[i].Name != expectedName {
			t.Fatalf("expected artifact %d name %s, got %s", i, expectedName, artifacts[i].Name)
		}
		if artifacts[i].TaskID != payload.TaskID {
			t.Fatalf("expected task id %s, got %s", payload.TaskID, artifacts[i].TaskID)
		}
		if artifacts[i].RunID != payload.RunID {
			t.Fatalf("expected run id %s, got %s", payload.RunID, artifacts[i].RunID)
		}
	}

	byName := artifactsByName(artifacts)
	if byName["mr_url"].Kind != "merge_request" {
		t.Fatalf("expected mr_url kind merge_request, got %s", byName["mr_url"].Kind)
	}
	if byName["mr_url"].URL != "https://gitlab.example/group/project/-/merge_requests/1" {
		t.Fatalf("unexpected mr_url artifact URL: %s", byName["mr_url"].URL)
	}
	if byName["branch"].Kind != "branch" {
		t.Fatalf("expected branch kind, got %s", byName["branch"].Kind)
	}
	if byName["branch"].URL != "" {
		t.Fatalf("expected branch URL to be empty, got %s", byName["branch"].URL)
	}
	if byName["test_results"].Kind != "test_result" {
		t.Fatalf("expected test_result kind, got %s", byName["test_results"].Kind)
	}
	if byName["notes_url"].Kind != "generic" {
		t.Fatalf("expected generic kind, got %s", byName["notes_url"].Kind)
	}
	if byName["notes_url"].URL != "https://gitlab.example/group/project/-/issues/3#note_1" {
		t.Fatalf("expected generic URL to be captured, got %s", byName["notes_url"].URL)
	}

	var metadata map[string]any
	if err := json.Unmarshal([]byte(byName["branch"].MetadataJSON), &metadata); err != nil {
		t.Fatalf("decode branch metadata: %v", err)
	}
	if metadata["source_key"] != "branch" {
		t.Fatalf("expected source_key branch, got %#v", metadata["source_key"])
	}
	if metadata["value"] != "feature/fix-framework" {
		t.Fatalf("expected branch value, got %#v", metadata["value"])
	}
}

func TestBuildArtifactsFromResultRejectsUnmarshalableMetadata(t *testing.T) {
	payload := results.CanonicalResult{
		TaskID: "task-artifact-invalid",
		RunID:  "run-artifact-invalid",
		Artifacts: map[string]any{
			"invalid": func() {},
		},
	}

	if _, err := BuildArtifactsFromResult(payload); err == nil {
		t.Fatal("expected unmarshalable metadata to fail")
	}
}

func artifactsByName(artifacts []model.Artifact) map[string]model.Artifact {
	byName := make(map[string]model.Artifact, len(artifacts))
	for _, artifact := range artifacts {
		byName[artifact.Name] = artifact
	}
	return byName
}
