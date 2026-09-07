package orchestrator

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

func TestApplyCanonicalResultMaterializesEffects(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, effectRepo := newCallbackEffectRepositories(t)
	orch := New(taskRepo, runRepo, WithEffectRepository(effectRepo))
	createCallbackArtifactFixture(t, ctx, taskRepo, runRepo, "task-callback-effects", "run-callback-effects")

	_, run, err := orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID:  "task-callback-effects",
		RunID:   "run-callback-effects",
		Status:  results.StatusSucceeded,
		Summary: "review completed",
		Effects: []results.EffectIntent{
			{
				DedupeKey: "im:mr-review:24207:12:abc123",
				Kind:      "send_im_message",
				Target:    "team-review-group",
				Critical:  false,
				Request: map[string]any{
					"body": "MR !12 审查完成",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("apply callback: %v", err)
	}
	if run.Status != model.RunStatusSucceeded {
		t.Fatalf("expected run succeeded, got %s", run.Status)
	}

	effects, err := effectRepo.ListByRunID(ctx, "run-callback-effects")
	if err != nil {
		t.Fatalf("list effects: %v", err)
	}
	if len(effects) != 1 {
		t.Fatalf("expected one effect, got %d", len(effects))
	}
	if effects[0].Kind != "send_im_message" {
		t.Fatalf("unexpected effect kind: %s", effects[0].Kind)
	}
	if effects[0].Target != "default" {
		t.Fatalf("unexpected effect target: %s", effects[0].Target)
	}
	if effects[0].Status != model.EffectStatusPending {
		t.Fatalf("expected pending effect, got %s", effects[0].Status)
	}
}

func TestApplyCanonicalResultMaterializesEffectsIdempotentlyForTerminalRun(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, effectRepo := newCallbackEffectRepositories(t)
	orch := New(taskRepo, runRepo, WithEffectRepository(effectRepo))
	createCallbackArtifactFixture(t, ctx, taskRepo, runRepo, "task-callback-effects-idempotent", "run-callback-effects-idempotent")

	payload := results.CanonicalResult{
		TaskID: "task-callback-effects-idempotent",
		RunID:  "run-callback-effects-idempotent",
		Status: results.StatusSucceeded,
		Effects: []results.EffectIntent{
			{
				DedupeKey: "im:mr-review:24207:12:abc123",
				Kind:      "send_im_message",
				Target:    "team-review-group",
				Request: map[string]any{
					"body": "first",
				},
			},
		},
	}
	if _, _, err := orch.ApplyCanonicalResult(ctx, payload); err != nil {
		t.Fatalf("first apply callback: %v", err)
	}
	payload.Effects[0].Request = map[string]any{
		"body": "second",
	}
	if _, _, err := orch.ApplyCanonicalResult(ctx, payload); err != nil {
		t.Fatalf("second apply callback: %v", err)
	}

	effects, err := effectRepo.ListByRunID(ctx, "run-callback-effects-idempotent")
	if err != nil {
		t.Fatalf("list effects: %v", err)
	}
	if len(effects) != 1 {
		t.Fatalf("expected one effect after repeated callback, got %d", len(effects))
	}
	if effects[0].RequestJSON != `{"body":"second"}` {
		t.Fatalf("expected upserted request json, got %s", effects[0].RequestJSON)
	}
}

func newCallbackEffectRepositories(t *testing.T) (*repository.TaskRepository, *repository.RunRepository, *repository.EffectRepository) {
	t.Helper()

	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Effect{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return repository.NewTaskRepository(db), repository.NewRunRepository(db), repository.NewEffectRepository(db)
}
