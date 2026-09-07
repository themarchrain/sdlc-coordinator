package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
)

// materializeHumanRequiredIMEffect 把「任务进入人工介入位」物化为 IM 通知。
// 与租约恢复通知同构(best-effort:落库失败绝不阻塞状态迁移),但去重语义更严:
// 调用点(结果校验拒绝/写回停车/tick 停车/对账升级)可能对同一状态重复触达,而
// UpsertByDedupeKey 会整行覆盖已存在 effect(含把已执行 Status 重置回 pending),
// 故先查存在性——同一 (taskID, runID) 至多物化一次。
func (o *Orchestrator) materializeHumanRequiredIMEffect(ctx context.Context, task *model.Task, cause string) {
	if o.effects == nil || task == nil || task.Status != model.TaskStatusHumanRequired {
		return
	}
	runID := ""
	if task.CurrentRunID != nil {
		runID = strings.TrimSpace(*task.CurrentRunID)
	}
	dedupeKey := fmt.Sprintf("human-required-im:%s:%s", task.TaskID, runID)
	_, err := o.effects.GetByDedupeKey(ctx, dedupeKey)
	if err == nil {
		return
	}
	// 读错误(非 NotFound)时宁可漏发也不落库:此时无法确认 effect 是否已执行,
	// 继续落库会把已执行 effect 整行重置回 pending,造成重复通知。
	if !errors.Is(err, repository.ErrNotFound) {
		return
	}
	requestJSON, err := json.Marshal(map[string]string{
		"body":   humanRequiredIMBody(task, cause),
		"format": "markdown",
	})
	if err != nil {
		return
	}
	effect := &model.Effect{
		TaskID:      task.TaskID,
		RunID:       runID,
		DedupeKey:   dedupeKey,
		Kind:        leaseRecoveryIMEffectKind,
		Target:      leaseRecoveryIMEffectTarget,
		Critical:    false,
		Status:      model.EffectStatusPending,
		RequestJSON: string(requestJSON),
	}
	_ = o.effects.UpsertByDedupeKey(ctx, effect)
}

func humanRequiredIMBody(task *model.Task, cause string) string {
	runID := "none"
	if task != nil && task.CurrentRunID != nil && strings.TrimSpace(*task.CurrentRunID) != "" {
		runID = strings.TrimSpace(*task.CurrentRunID)
	}
	cause = strings.TrimSpace(cause)
	if cause == "" {
		cause = "unknown"
	}
	return fmt.Sprintf(
		"Agent Task 已进入人工介入位，等待人工核验后再决定是否恢复。\n\n- Task: `%s`\n- Worker: `%s`\n- Previous Run: `%s`\n- Attempt: %d\n- Reason: %s\n- Next status: `human_required`",
		task.TaskID,
		task.Role,
		runID,
		task.AttemptCount,
		cause,
	)
}
