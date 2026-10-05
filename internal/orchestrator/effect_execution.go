package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	effectsexec "sdlc-coordinator/internal/effects"
	"sdlc-coordinator/internal/model"
)

// staleEffectRunningTimeout 是效果停留 running 态的回收阈值:超过即视为执行方
// 中断遗留,置 failed 供人工 RetryEffect,与任务租约恢复、写回 unknown 对账的
// 恢复模式对齐。
const staleEffectRunningTimeout = 15 * time.Minute

// effectMaxPendingAge 是待发效果的时效阈值:渠道长期未配置期间积压的
// 通知,补发只剩噪音价值,超龄直接作废(skipped 终态记因),配置恢复后
// 只补发新鲜通知。
const effectMaxPendingAge = 24 * time.Hour

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
	if probe, ok := o.effectExecutor.(conditionalEffectExecutor); ok && !probe.EffectExecutionEnabled(ctx) {
		// 执行器运行期申报未启用(组织段未配置 IM 渠道):与无执行器同语义。
		return []EffectExecutionResult{}, nil
	}

	effects, err := o.effects.ListPending(ctx, limit)
	if err != nil {
		return nil, err
	}

	results := make([]EffectExecutionResult, 0, len(effects))
	for i := range effects {
		effect := effects[i]
		if time.Since(effect.CreatedAt) > effectMaxPendingAge {
			effect.Status = model.EffectStatusSkipped
			effect.ErrorMessage = fmt.Sprintf("通知已过期跳过（创建超过 %s）", effectMaxPendingAge)
			if err := o.effects.Update(ctx, &effect); err != nil {
				return nil, fmt.Errorf("mark effect %s skipped: %w", effect.DedupeKey, err)
			}
			results = append(results, EffectExecutionResult{
				EffectID: effect.EffectID, DedupeKey: effect.DedupeKey, Kind: effect.Kind,
				Target: effect.Target, Critical: effect.Critical, Status: effect.Status,
				AttemptCount: effect.AttemptCount, ErrorMessage: effect.ErrorMessage,
			})
			continue
		}
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
			if errors.Is(err, ErrEffectExecutorDisabled) {
				// 批次中途渠道被清除(与批首探测间存在窗口):本条退回
				// 队列保持 pending,后续效果原样留待配置后重放。
				effect.Status = model.EffectStatusPending
				if updateErr := o.effects.Update(ctx, &effect); updateErr != nil {
					return nil, fmt.Errorf("requeue effect %s: %w", effect.DedupeKey, updateErr)
				}
				break
			}
			if errors.Is(err, effectsexec.ErrEffectDeliveryUnverified) {
				// 投递未核验:消息可能已送达,记独立终态避免误导性重发;
				// 错误文本只含有界元数据(状态码/原因/上限/读取量),
				// 不携带远端响应正文。
				effect.Status = model.EffectStatusDeliveryUnverified
				effect.ErrorMessage = err.Error()
				if updateErr := o.effects.Update(ctx, &effect); updateErr != nil {
					return nil, fmt.Errorf("mark effect %s delivery unverified: %w", effect.DedupeKey, updateErr)
				}
				result.Status = effect.Status
				result.AttemptCount = effect.AttemptCount
				result.ErrorMessage = effect.ErrorMessage
				results = append(results, result)
				continue
			}
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

// conditionalEffectExecutor 允许执行器在运行期申报「未启用」:效果保留在
// 队列,等待渠道配置后重放,与未装配执行器语义一致。
type conditionalEffectExecutor interface {
	EffectExecutionEnabled(ctx context.Context) bool
}

// ErrEffectExecutorDisabled 是执行器在运行期申报「未启用」的哨兵错误:
// ExecuteEffect 返回被它包装的错误时,效果退回队列保持 pending,而不是
// 置 failed 等人工重试。
var ErrEffectExecutorDisabled = errors.New("effect executor disabled")
