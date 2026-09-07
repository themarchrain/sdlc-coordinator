package orchestrator

import (
	"context"
	"errors"
	"testing"

	"sdlc-coordinator/internal/model"
)

func TestRetryEffectMovesFailedEffectBackToPending(t *testing.T) {
	ctx := context.Background()
	effectRepo := newEffectExecutionRepository(t)
	if err := effectRepo.Create(ctx, &model.Effect{
		TaskID:       "task-1",
		RunID:        "run-1",
		DedupeKey:    "effect-retry-1",
		Kind:         "send_im_message",
		Target:       "team-review-group",
		Status:       model.EffectStatusFailed,
		AttemptCount: 2,
		ResponseJSON: `{"message_id":"old"}`,
		ErrorMessage: "webhook unavailable",
	}); err != nil {
		t.Fatalf("create effect: %v", err)
	}
	orch := New(nil, nil, WithEffectRepository(effectRepo))

	effect, err := orch.RetryEffect(ctx, "effect-retry-1")
	if err != nil {
		t.Fatalf("retry effect: %v", err)
	}
	if effect.Status != model.EffectStatusPending {
		t.Fatalf("expected pending, got %s", effect.Status)
	}
	if effect.AttemptCount != 2 {
		t.Fatalf("expected attempt count preserved, got %d", effect.AttemptCount)
	}
	if effect.ErrorMessage != "" {
		t.Fatalf("expected error cleared, got %s", effect.ErrorMessage)
	}
	if effect.ResponseJSON != "" {
		t.Fatalf("expected response cleared, got %s", effect.ResponseJSON)
	}
}

func TestRetryEffectRejectsNonFailedEffect(t *testing.T) {
	ctx := context.Background()

	for _, status := range []model.EffectStatus{
		model.EffectStatusPending,
		model.EffectStatusSucceeded,
	} {
		t.Run(string(status), func(t *testing.T) {
			effectRepo := newEffectExecutionRepository(t)
			if err := effectRepo.Create(ctx, &model.Effect{
				TaskID:    "task-1",
				RunID:     "run-1",
				DedupeKey: "effect-retry-" + string(status),
				Kind:      "send_im_message",
				Target:    "team-review-group",
				Status:    status,
			}); err != nil {
				t.Fatalf("create effect: %v", err)
			}
			orch := New(nil, nil, WithEffectRepository(effectRepo))

			_, err := orch.RetryEffect(ctx, "effect-retry-"+string(status))
			if !errors.Is(err, ErrEffectNotRetryable) {
				t.Fatalf("expected ErrEffectNotRetryable, got %v", err)
			}
		})
	}
}
