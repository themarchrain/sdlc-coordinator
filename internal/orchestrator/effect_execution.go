package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"sdlc-coordinator/internal/model"
)

// staleEffectRunningTimeout 是效果停留 running 态的回收阈值:超过即视为执行方
// 中断遗留,置 failed 供人工 RetryEffect,与任务租约恢复、写回 unknown 对账的
// 恢复模式对齐。
const staleEffectRunningTimeout = 15 * time.Minute

type EffectExecutionResult struct {
	EffectID     string             `json:"effect_id"`
	DedupeKey    string             `json:"dedupe_key"`
	Kind         string             `json:"kind"`
	Target       string             `json:"target"`
	Critical     bool               `json:"critical"`
	Status       model.EffectStatus `json:"status"`
	AttemptCount int                `json:"attempt_count"`
	ErrorMessage string             `json:"error_message,omitempty"`
}

func (o *Orchestrator) ExecutePendingEffects(ctx context.Context, limit int) ([]EffectExecutionResult, error) {
	if o.effects == nil {
		return nil, errors.New("effect repository is required")
	}
	if o.effectExecutor == nil {
		// IM 未配置时没有执行器：暂存效果保留在队列里，等渠道配置后自然重放。
		// 这不是 tick 失败，避免调度循环对合法的"未启用 IM"状态空转报错。
		return []EffectExecutionResult{}, nil
	}

	effects, err := o.effects.ListPending(ctx, limit)
	if err != nil {
		return nil, err
	}

	results := make([]EffectExecutionResult, 0, len(effects))
	for i := range effects {
		effect := effects[i]
		result := EffectExecutionResult{
			EffectID:     effect.EffectID,
			DedupeKey:    effect.DedupeKey,
			Kind:         effect.Kind,
			Target:       effect.Target,
			Critical:     effect.Critical,
			AttemptCount: effect.AttemptCount,
		}

		effect.Status = model.EffectStatusRunning
		effect.AttemptCount++
		effect.ErrorMessage = ""
		if err := o.effects.Update(ctx, &effect); err != nil {
			return nil, fmt.Errorf("mark effect %s running: %w", effect.DedupeKey, err)
		}

		responseJSON, err := o.effectExecutor.ExecuteEffect(ctx, &effect)
		if err != nil {
			effect.Status = model.EffectStatusFailed
			effect.ErrorMessage = err.Error()
			if updateErr := o.effects.Update(ctx, &effect); updateErr != nil {
				return nil, fmt.Errorf("mark effect %s failed: %w", effect.DedupeKey, updateErr)
			}
			result.Status = effect.Status
			result.AttemptCount = effect.AttemptCount
			result.ErrorMessage = effect.ErrorMessage
			results = append(results, result)
			continue
		}

		effect.Status = model.EffectStatusSucceeded
		effect.ResponseJSON = responseJSON
		effect.ErrorMessage = ""
		if err := o.effects.Update(ctx, &effect); err != nil {
			return nil, fmt.Errorf("mark effect %s succeeded: %w", effect.DedupeKey, err)
		}
		result.Status = effect.Status
		result.AttemptCount = effect.AttemptCount
		results = append(results, result)
	}

	return results, nil
}


// RecoverStaleEffects 把停留 running 超过阈值的效果回收为 failed。
// 返回的 EffectExecutionResult 供 tick 结果观测;无回收时返回空切片。
func (o *Orchestrator) RecoverStaleEffects(ctx context.Context, limit int) ([]EffectExecutionResult, error) {
	if o.effects == nil {
		return nil, errors.New("effect repository is required")
	}
	effects, err := o.effects.ListStaleRunning(ctx, o.now().Add(-staleEffectRunningTimeout), limit)
	if err != nil {
		return nil, err
	}

	results := make([]EffectExecutionResult, 0, len(effects))
	for i := range effects {
		effect := effects[i]
		effect.Status = model.EffectStatusFailed
		effect.ErrorMessage = "effect execution stalled; recovered by stale-running sweep"
		if err := o.effects.Update(ctx, &effect); err != nil {
			return nil, fmt.Errorf("recover stale effect %s: %w", effect.DedupeKey, err)
		}
		results = append(results, EffectExecutionResult{
			EffectID:     effect.EffectID,
			DedupeKey:    effect.DedupeKey,
			Kind:         effect.Kind,
			Target:       effect.Target,
			Critical:     effect.Critical,
			Status:       effect.Status,
			AttemptCount: effect.AttemptCount,
			ErrorMessage: effect.ErrorMessage,
		})
	}
	return results, nil
}
