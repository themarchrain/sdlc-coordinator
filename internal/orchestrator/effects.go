package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
)

func BuildEffectsFromResult(payload results.CanonicalResult) ([]model.Effect, error) {
	if len(payload.Effects) == 0 {
		return nil, nil
	}
	if strings.TrimSpace(payload.TaskID) == "" {
		return nil, errors.New("result task_id is required for effects")
	}
	if strings.TrimSpace(payload.RunID) == "" {
		return nil, errors.New("result run_id is required for effects")
	}

	effects := make([]model.Effect, 0, len(payload.Effects))
	for i, intent := range payload.Effects {
		intent.Kind = strings.TrimSpace(intent.Kind)
		intent.Target = strings.TrimSpace(intent.Target)
		intent.DedupeKey = strings.TrimSpace(intent.DedupeKey)
		if intent.Kind == "" {
			return nil, fmt.Errorf("effect[%d]: kind is required", i)
		}
		if intent.Kind == "send_im_message" {
			intent.Target = "default"
		}
		if intent.Target == "" {
			return nil, fmt.Errorf("effect[%d]: target is required", i)
		}
		if intent.DedupeKey == "" {
			intent.DedupeKey = fmt.Sprintf("effect:%s:%s:%d:%s:%s", payload.TaskID, payload.RunID, i, intent.Kind, intent.Target)
		}

		request := intent.Request
		if request == nil {
			request = map[string]any{}
		}
		rawRequest, err := json.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("marshal effect[%d] request: %w", i, err)
		}

		effects = append(effects, model.Effect{
			TaskID:      payload.TaskID,
			RunID:       payload.RunID,
			DedupeKey:   intent.DedupeKey,
			Kind:        intent.Kind,
			Target:      intent.Target,
			Critical:    intent.Critical,
			Status:      model.EffectStatusPending,
			RequestJSON: string(rawRequest),
		})
	}

	return effects, nil
}

func (o *Orchestrator) materializeResultEffects(ctx context.Context, payload results.CanonicalResult) error {
	if o.effects == nil || len(payload.Effects) == 0 {
		return nil
	}

	effects, err := BuildEffectsFromResult(payload)
	if err != nil {
		return err
	}
	for i := range effects {
		if err := o.effects.UpsertByDedupeKey(ctx, &effects[i]); err != nil {
			return fmt.Errorf("upsert result effect %s: %w", effects[i].DedupeKey, err)
		}
	}
	return nil
}
