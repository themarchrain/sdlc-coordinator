package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
)

func TestBuildEffectsFromResultBuildsPendingEffects(t *testing.T) {
	effects, err := BuildEffectsFromResult(results.CanonicalResult{
		TaskID: "task-review-1",
		RunID:  "run-review-1",
		Effects: []results.EffectIntent{
			{
				DedupeKey: "im:mr-review:24207:12:abc123",
				Kind:      "send_im_message",
				Target:    "team-review-group",
				Critical:  false,
				Request: map[string]any{
					"body":     "MR !12 审查完成",
					"mentions": []any{"author@example.com"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("build effects: %v", err)
	}
	if len(effects) != 1 {
		t.Fatalf("expected one effect, got %d", len(effects))
	}

	effect := effects[0]
	if effect.TaskID != "task-review-1" {
		t.Fatalf("unexpected task id: %s", effect.TaskID)
	}
	if effect.RunID != "run-review-1" {
		t.Fatalf("unexpected run id: %s", effect.RunID)
	}
	if effect.DedupeKey != "im:mr-review:24207:12:abc123" {
		t.Fatalf("unexpected dedupe key: %s", effect.DedupeKey)
	}
	if effect.Kind != "send_im_message" {
		t.Fatalf("unexpected kind: %s", effect.Kind)
	}
	if effect.Target != "default" {
		t.Fatalf("unexpected target: %s", effect.Target)
	}
	if effect.Critical {
		t.Fatal("expected non-critical effect")
	}
	if effect.Status != model.EffectStatusPending {
		t.Fatalf("expected pending status, got %s", effect.Status)
	}

	var request map[string]any
	if err := json.Unmarshal([]byte(effect.RequestJSON), &request); err != nil {
		t.Fatalf("decode request json: %v", err)
	}
	if request["body"] != "MR !12 审查完成" {
		t.Fatalf("unexpected request body: %#v", request["body"])
	}
}

func TestBuildEffectsFromResultNormalizesIMTargetToDefault(t *testing.T) {
	effects, err := BuildEffectsFromResult(results.CanonicalResult{
		TaskID: "task-review-1",
		RunID:  "run-review-1",
		Effects: []results.EffectIntent{{
			DedupeKey: "im:review:1",
			Kind:      "send_im_message",
			Target:    "team-review-group",
			Request:   map[string]any{"body": "review completed"},
		}},
	})
	if err != nil {
		t.Fatalf("build effects: %v", err)
	}
	if len(effects) != 1 || effects[0].Target != "default" {
		t.Fatalf("send_im_message target was not normalized: %#v", effects)
	}
}

func TestBuildEffectsFromResultRejectsMissingKind(t *testing.T) {
	_, err := BuildEffectsFromResult(results.CanonicalResult{
		TaskID: "task-review-1",
		RunID:  "run-review-1",
		Effects: []results.EffectIntent{
			{
				Target: "team-review-group",
			},
		},
	})
	if err == nil {
		t.Fatal("expected missing kind error")
	}
	if !strings.Contains(err.Error(), "kind is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildEffectsFromResultDefaultsMissingIMTarget(t *testing.T) {
	effects, err := BuildEffectsFromResult(results.CanonicalResult{
		TaskID: "task-review-1",
		RunID:  "run-review-1",
		Effects: []results.EffectIntent{
			{
				Kind: "send_im_message",
			},
		},
	})
	if err != nil {
		t.Fatalf("build effects: %v", err)
	}
	if len(effects) != 1 || effects[0].Target != "default" {
		t.Fatalf("missing IM target was not defaulted: %#v", effects)
	}
}

func TestBuildEffectsFromResultGeneratesFallbackDedupeKey(t *testing.T) {
	effects, err := BuildEffectsFromResult(results.CanonicalResult{
		TaskID: "task-review-1",
		RunID:  "run-review-1",
		Effects: []results.EffectIntent{
			{
				Kind:   "send_im_message",
				Target: "team-review-group",
			},
		},
	})
	if err != nil {
		t.Fatalf("build effects: %v", err)
	}
	if len(effects) != 1 {
		t.Fatalf("expected one effect, got %d", len(effects))
	}
	if effects[0].DedupeKey != "effect:task-review-1:run-review-1:0:send_im_message:default" {
		t.Fatalf("unexpected fallback dedupe key: %s", effects[0].DedupeKey)
	}
}
