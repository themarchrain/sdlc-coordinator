package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/model"
)

var ErrEffectNotRetryable = errors.New("effect is not retryable")

func (o *Orchestrator) RetryEffect(ctx context.Context, dedupeKey string) (*model.Effect, error) {
	if o.effects == nil {
		return nil, errors.New("effect repository is required")
	}
	dedupeKey = strings.TrimSpace(dedupeKey)
	if dedupeKey == "" {
		return nil, errors.New("effect dedupe key is required")
	}

	effect, err := o.effects.GetByDedupeKey(ctx, dedupeKey)
	if err != nil {
		return nil, err
	}
	if effect.Status != model.EffectStatusFailed {
		return nil, fmt.Errorf("%w: status %s", ErrEffectNotRetryable, effect.Status)
	}

	effect.Status = model.EffectStatusPending
	effect.ErrorMessage = ""
	effect.ResponseJSON = ""
	if err := o.effects.Update(ctx, effect); err != nil {
		return nil, fmt.Errorf("retry effect %s: %w", dedupeKey, err)
	}
	return effect, nil
}
