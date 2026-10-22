package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	effectsexec "sdlc-coordinator/internal/effects"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
)

// staleEffectRunningTimeout 是效果停留 running 态的回收阈值:超过即视为执行方
// 中断遗留,可被任意实例原子接管重做;与任务租约恢复、写回 unknown 对账的
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
		listed := effects[i]

		// 超龄通知先抢占再作废:代次栅栏保证双实例竞争时只有一个落终态,
		// 败者本轮静默跳过。
		if time.Since(listed.CreatedAt) > effectMaxPendingAge {
			result, err := o.expireAgedPendingEffect(ctx, listed)
			if err != nil {
				return nil, err
			}
			if result != nil {
				results = append(results, *result)
			}
			continue
		}

		claimed, won, err := o.effects.Claim(ctx, listed.DedupeKey, staleEffectRunningTimeout)
		if err != nil {
			return nil, fmt.Errorf("claim effect %s: %w", listed.DedupeKey, err)
		}
		if !won {
			// 执行权被另一实例取得(或终态已被并发推进):本轮不产生结果。
			continue
		}
		effect := *claimed
		result := EffectExecutionResult{
			EffectID:     effect.EffectID,
			DedupeKey:    effect.DedupeKey,
			Kind:         effect.Kind,
			Target:       effect.Target,
			Critical:     effect.Critical,
			AttemptCount: effect.AttemptCount,
		}

		responseJSON, err := o.executeAdmittedEffect(ctx, &effect)
		if err != nil {
			if errors.Is(err, repository.ErrTaskInterrupted) || actionOutcomeUncertain(err) {
				status, reason := model.EffectStatusSkipped, "task_interrupted"
				if actionOutcomeUncertain(err) {
					status, reason = model.EffectStatusDeliveryUnverified, err.Error()
				}
				applied, updateErr := o.effects.Complete(ctx, effect.DedupeKey, effect.AttemptCount, status, "", reason)
				if updateErr != nil {
					return nil, updateErr
				}
				if applied {
					result.Status, result.ErrorMessage = status, reason
					results = append(results, result)
				}
				continue
			}
			if errors.Is(err, ErrEffectExecutorDisabled) {
				// 批次中途渠道被清除(与批首探测间存在窗口):本条退回
				// 队列保持 pending,后续效果原样留待配置后重放。
				if _, updateErr := o.effects.RequeueIfRunning(ctx, effect.DedupeKey, effect.AttemptCount); updateErr != nil {
					return nil, fmt.Errorf("requeue effect %s: %w", effect.DedupeKey, updateErr)
				}
				break
			}
			if errors.Is(err, effectsexec.ErrEffectDeliveryUnverified) {
				// 投递未核验:消息可能已送达,记独立终态避免误导性重发;
				// 错误文本只含有界元数据(状态码/原因/上限/读取量),
				// 不携带远端响应正文。
				status := model.EffectStatusDeliveryUnverified
				applied, updateErr := o.effects.Complete(ctx, effect.DedupeKey, effect.AttemptCount, status, "", err.Error())
				if updateErr != nil {
					return nil, fmt.Errorf("mark effect %s delivery unverified: %w", effect.DedupeKey, updateErr)
				}
				if applied {
					result.Status = status
					result.ErrorMessage = err.Error()
					results = append(results, result)
				}
				continue
			}
			applied, updateErr := o.effects.Complete(ctx, effect.DedupeKey, effect.AttemptCount, model.EffectStatusFailed, "", err.Error())
			if updateErr != nil {
				return nil, fmt.Errorf("mark effect %s failed: %w", effect.DedupeKey, updateErr)
			}
			if applied {
				result.Status = model.EffectStatusFailed
				result.ErrorMessage = err.Error()
				results = append(results, result)
			}
			continue
		}

		applied, updateErr := o.effects.Complete(ctx, effect.DedupeKey, effect.AttemptCount, model.EffectStatusSucceeded, responseJSON, "")
		if updateErr != nil {
			return nil, fmt.Errorf("mark effect %s succeeded: %w", effect.DedupeKey, updateErr)
		}
		if applied {
			result.Status = model.EffectStatusSucceeded
			results = append(results, result)
		}
	}

	return results, nil
}

// expireAgedPendingEffect 把超龄待发效果作废为 skipped 终态;作废权被另一
// 实例取得(或已被抢占执行)时返回 nil(对方已负责处理)。
func (o *Orchestrator) expireAgedPendingEffect(ctx context.Context, listed model.Effect) (*EffectExecutionResult, error) {
	errorMessage := fmt.Sprintf("通知已过期跳过（创建超过 %s）", effectMaxPendingAge)
	applied, err := o.effects.SkipIfPending(ctx, listed.DedupeKey, errorMessage)
	if err != nil {
		return nil, fmt.Errorf("mark effect %s skipped: %w", listed.DedupeKey, err)
	}
	if !applied {
		return nil, nil
	}
	return &EffectExecutionResult{
		EffectID:     listed.EffectID,
		DedupeKey:    listed.DedupeKey,
		Kind:         listed.Kind,
		Target:       listed.Target,
		Critical:     listed.Critical,
		Status:       model.EffectStatusSkipped,
		AttemptCount: listed.AttemptCount,
		ErrorMessage: errorMessage,
	}, nil
}

// RecoverStaleEffects 回收停留 running 超过阈值的效果:条件终态回写恰好让
// 一个实例置 failed 供人工 RetryEffect,败者 0 行退出;attempt 计数不变
// (回收是终止,不是新的执行尝试)。返回的 EffectExecutionResult 供 tick
// 结果观测;无回收时返回空切片。
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
		listed := effects[i]
		errorMessage := "effect execution stalled; recovered by stale-running sweep"
		status := model.EffectStatusFailed
		var applied bool
		if o.actionAdmissions != nil {
			guarded, ok := o.effects.(interface {
				RecoverStaleDelivery(context.Context, string, int, time.Duration, string) (model.EffectStatus, bool, error)
			})
			if !ok {
				return nil, ErrStopUnavailable
			}
			status, applied, err = guarded.RecoverStaleDelivery(ctx, listed.DedupeKey, listed.AttemptCount, staleEffectRunningTimeout, errorMessage)
		} else {
			applied, err = o.effects.FailStaleRunning(ctx, listed.DedupeKey, staleEffectRunningTimeout, errorMessage)
		}
		if err != nil {
			return nil, fmt.Errorf("recover stale effect %s: %w", listed.DedupeKey, err)
		}
		if !applied {
			continue
		}
		results = append(results, EffectExecutionResult{
			EffectID:     listed.EffectID,
			DedupeKey:    listed.DedupeKey,
			Kind:         listed.Kind,
			Target:       listed.Target,
			Critical:     listed.Critical,
			Status:       status,
			AttemptCount: listed.AttemptCount,
			ErrorMessage: errorMessage,
		})
	}
	return results, nil
}

// conditionalEffectExecutor 允许执行器在运行期申报"未启用":效果保留在
// 队列,等待渠道配置后重放,与未装配执行器语义一致。
type conditionalEffectExecutor interface {
	EffectExecutionEnabled(ctx context.Context) bool
}

// ErrEffectExecutorDisabled 是执行器在运行期申报"未启用"的哨兵错误:
// ExecuteEffect 返回被它包装的错误时,效果退回队列保持 pending,而不是
// 置 failed 等人工重试。
var ErrEffectExecutorDisabled = errors.New("effect executor disabled")
