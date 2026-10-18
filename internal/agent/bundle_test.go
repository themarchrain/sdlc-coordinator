package agent_test

import (
	"encoding/json"
	"testing"

	"sdlc-coordinator/internal/agent"
)

func TestRenderTaskBundleProducesAuditablePayload(t *testing.T) {
	t.Parallel()

	bundle := agent.TaskBundle{
		TaskPackage: agent.TaskPackage{
			TaskID: "task-1", RunID: "run-1", Type: "review_mr", Role: "ReviewerWorker",
			Objective: "review exact head", WorkspaceStrategy: agent.WorkspaceStrategyCoordinatorPrepared,
		},
		ResultSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"verdict": map[string]any{"type": "string"}},
		},
		Prompt: "review the prepared workspace",
	}
	files, err := agent.RenderTaskBundle(bundle)
	if err != nil {
		t.Fatalf("render task bundle: %v", err)
	}

	taskRaw := files[agent.TaskContextFileName]
	var task agent.TaskPackage
	if err := json.Unmarshal(taskRaw, &task); err != nil {
		t.Fatalf("decode task context: %v", err)
	}
	if task.TaskID != "task-1" || task.RunID != "run-1" || task.Role != "ReviewerWorker" {
		t.Fatalf("unexpected persisted task package: %#v", task)
	}

	schemaRaw := files[agent.ResultSchemaFileName]
	var schema map[string]any
	if err := json.Unmarshal(schemaRaw, &schema); err != nil || schema["type"] != "object" {
		t.Fatalf("unexpected persisted schema: schema=%#v err=%v", schema, err)
	}
	if got := string(files[agent.PromptFileName]); got != bundle.Prompt+"\n" {
		t.Fatalf("unexpected persisted prompt %q", got)
	}

	if len(files) != 3 {
		t.Fatalf("expected three task bundle files, got %d", len(files))
	}
}
