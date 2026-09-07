package orchestrator

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
)

func TestApplyCanonicalResultAdvancesCurrentTaskAndRun(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, artifactRepo := newCallbackArtifactRepositories(t)
	orch := New(taskRepo, runRepo, WithArtifactRepository(artifactRepo))
	createCallbackArtifactFixture(t, ctx, taskRepo, runRepo, "task-canonical", "run-canonical")

	task, run, err := orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID:  "task-canonical",
		RunID:   "run-canonical",
		Status:  results.StatusSucceeded,
		Summary: "canonical result applied",
	})
	if err != nil {
		t.Fatalf("apply canonical result: %v", err)
	}
	if task.Status != model.TaskStatusSucceeded {
		t.Fatalf("expected succeeded task, got %s", task.Status)
	}
	if run.Status != model.RunStatusSucceeded {
		t.Fatalf("expected succeeded run, got %s", run.Status)
	}
}
