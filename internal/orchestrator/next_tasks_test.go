package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
)

func TestBuildNextTaskExpansionUsesExplicitFields(t *testing.T) {
	parentRunID := "run-parent-next"
	parent := &model.Task{
		TaskID:       "task-parent-next",
		Role:         "TesterWorker",
		SourceRepo:   "test-automates",
		TargetRepo:   "test-automates",
		CurrentRunID: &parentRunID,
	}
	parentRun := &model.Run{RunID: parentRunID, TaskID: parent.TaskID}

	child, dependency, err := BuildNextTaskExpansion(parent, parentRun, results.NextTaskIntent{
		TaskID:             "task-upstream-fix",
		DedupeKey:          "next:task-parent-next:framework-fix",
		Type:               "upstream_fix",
		Role:               "DeveloperWorker",
		SourceRepo:         "test-automates",
		TargetRepo:         "framework",
		Objective:          "fix framework validator",
		AcceptanceCriteria: "framework tests pass",
		DependencyRelation: "upstream_fix",
		AllowedRepos:       results.AllowedRepos{Writable: []string{"framework"}, Readable: []string{"test-automates"}},
		SuccessCriteria:    []string{"framework tests pass"},
		Context:            map[string]any{"failure_evidence": "downstream test failed"},
	}, 0)
	if err != nil {
		t.Fatalf("build next task expansion: %v", err)
	}

	if child.TaskID != "task-upstream-fix" {
		t.Fatalf("expected explicit task id, got %s", child.TaskID)
	}
	if child.DedupeKey != "next:task-parent-next:framework-fix" {
		t.Fatalf("expected explicit dedupe key, got %s", child.DedupeKey)
	}
	if child.Type != "upstream_fix" {
		t.Fatalf("expected upstream_fix type, got %s", child.Type)
	}
	if child.Role != "DeveloperWorker" {
		t.Fatalf("expected DeveloperWorker role, got %s", child.Role)
	}
	if child.SourceRepo != "test-automates" {
		t.Fatalf("expected source repo, got %s", child.SourceRepo)
	}
	if child.TargetRepo != "framework" {
		t.Fatalf("expected target repo, got %s", child.TargetRepo)
	}
	if child.AcceptanceCriteria != "framework tests pass" {
		t.Fatalf("expected acceptance criteria, got %s", child.AcceptanceCriteria)
	}

	var context map[string]any
	if err := json.Unmarshal([]byte(child.ContextJSON), &context); err != nil {
		t.Fatalf("decode child context: %v", err)
	}
	if context["parent_task_id"] != "task-parent-next" {
		t.Fatalf("expected parent_task_id in context, got %#v", context["parent_task_id"])
	}
	if context["parent_run_id"] != parentRunID {
		t.Fatalf("expected parent_run_id in context, got %#v", context["parent_run_id"])
	}
	if context["failure_evidence"] != "downstream test failed" {
		t.Fatalf("expected failure evidence, got %#v", context["failure_evidence"])
	}

	if dependency.ParentTaskID != "task-parent-next" {
		t.Fatalf("expected parent dependency, got %s", dependency.ParentTaskID)
	}
	if dependency.ChildTaskID != "task-upstream-fix" {
		t.Fatalf("expected child dependency, got %s", dependency.ChildTaskID)
	}
	if dependency.Relation != "upstream_fix" {
		t.Fatalf("expected upstream_fix relation, got %s", dependency.Relation)
	}
	if dependency.Status != model.DependencyStatusPending {
		t.Fatalf("expected pending dependency, got %s", dependency.Status)
	}
}

func TestBuildNextTaskExpansionDefaultsStableFields(t *testing.T) {
	parent := &model.Task{TaskID: "task-parent-defaults"}
	parentRun := &model.Run{RunID: "run-parent-defaults", TaskID: parent.TaskID}

	child, dependency, err := BuildNextTaskExpansion(parent, parentRun, results.NextTaskIntent{
		Role:            "DeveloperWorker",
		Objective:       "fix upstream package",
		SuccessCriteria: []string{"unit tests pass", "integration tests pass"},
	}, 2)
	if err != nil {
		t.Fatalf("build next task expansion: %v", err)
	}

	if child.DedupeKey != "next:task-parent-defaults:2" {
		t.Fatalf("expected stable default dedupe key, got %s", child.DedupeKey)
	}
	if child.Type != "generated_from_next_task" {
		t.Fatalf("expected default type, got %s", child.Type)
	}
	if child.AcceptanceCriteria != "unit tests pass\nintegration tests pass" {
		t.Fatalf("expected joined success criteria, got %q", child.AcceptanceCriteria)
	}
	if dependency.Relation != "upstream_fix" {
		t.Fatalf("expected default relation, got %s", dependency.Relation)
	}
	if !strings.Contains(dependency.MetadataJSON, "fix upstream package") {
		t.Fatalf("expected dependency metadata to include objective, got %s", dependency.MetadataJSON)
	}
}

func TestBuildNextTaskExpansionRejectsMissingRequiredFields(t *testing.T) {
	parent := &model.Task{TaskID: "task-parent-invalid"}
	parentRun := &model.Run{RunID: "run-parent-invalid", TaskID: parent.TaskID}

	if _, _, err := BuildNextTaskExpansion(parent, parentRun, results.NextTaskIntent{
		Objective: "missing role",
	}, 0); err == nil {
		t.Fatal("expected missing role to fail")
	}

	if _, _, err := BuildNextTaskExpansion(parent, parentRun, results.NextTaskIntent{
		Role: "DeveloperWorker",
	}, 0); err == nil {
		t.Fatal("expected missing objective to fail")
	}
}

func TestApplyCanonicalResultSkipsExpansionWithoutDependencyRepository(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, _ := newCallbackNextTaskRepositories(t)
	orch := New(taskRepo, runRepo)
	createCallbackNextTaskFixture(t, ctx, taskRepo, runRepo, "task-next-nodedeps", "run-next-nodedeps")

	if _, _, err := orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID:  "task-next-nodedeps",
		RunID:   "run-next-nodedeps",
		Status:  results.StatusSucceeded,
		Summary: "done",
		NextTasks: []results.NextTaskIntent{{
			Role:      "TesterWorker",
			Objective: "verify the merge request",
		}},
	}); err != nil {
		t.Fatalf("missing dependency port must not block result application: %v", err)
	}
}
