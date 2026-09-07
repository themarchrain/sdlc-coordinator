package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/model"
)

const (
	leaseRecoveryIMEffectKind   = "send_im_message"
	leaseRecoveryIMEffectTarget = "default"
)

func (o *Orchestrator) materializeLeaseRecoveryIMEffect(ctx context.Context, task *model.Task) {
	if o.effects == nil || task == nil || task.Status != model.TaskStatusRetryPending {
		return
	}

	requestJSON, err := json.Marshal(map[string]string{
		"body":   leaseRecoveryIMBody(task),
		"format": "markdown",
	})
	if err != nil {
		return
	}

	runID := ""
	if task.CurrentRunID != nil {
		runID = strings.TrimSpace(*task.CurrentRunID)
	}
	effect := &model.Effect{
		TaskID:      task.TaskID,
		RunID:       runID,
		DedupeKey:   fmt.Sprintf("lease-recovery-im:%s:%d", task.TaskID, task.AttemptCount),
		Kind:        leaseRecoveryIMEffectKind,
		Target:      leaseRecoveryIMEffectTarget,
		Critical:    false,
		Status:      model.EffectStatusPending,
		RequestJSON: string(requestJSON),
	}

	// Recovery is the source of truth. Notification persistence is intentionally
	// best-effort: an IM outage or a local Effect storage error must never keep
	// a safely recoverable Task from retry_pending.
	_ = o.effects.UpsertByDedupeKey(ctx, effect)
}

func (o *Orchestrator) materializeLeaseHumanRequiredIMEffect(ctx context.Context, task *model.Task) {
	if o.effects == nil || task == nil || task.Status != model.TaskStatusHumanRequired {
		return
	}

	requestJSON, err := json.Marshal(map[string]string{
		"body":   leaseHumanRequiredIMBody(task),
		"format": "markdown",
	})
	if err != nil {
		return
	}

	runID := ""
	if task.CurrentRunID != nil {
		runID = strings.TrimSpace(*task.CurrentRunID)
	}
	effect := &model.Effect{
		TaskID:      task.TaskID,
		RunID:       runID,
		DedupeKey:   fmt.Sprintf("lease-human-required-im:%s:%d", task.TaskID, task.AttemptCount),
		Kind:        leaseRecoveryIMEffectKind,
		Target:      leaseRecoveryIMEffectTarget,
		Critical:    false,
		Status:      model.EffectStatusPending,
		RequestJSON: string(requestJSON),
	}

	_ = o.effects.UpsertByDedupeKey(ctx, effect)
}

func leaseRecoveryIMBody(task *model.Task) string {
	runID := "none"
	if task != nil && task.CurrentRunID != nil && strings.TrimSpace(*task.CurrentRunID) != "" {
		runID = strings.TrimSpace(*task.CurrentRunID)
	}
	return fmt.Sprintf(
		"Agent Task lease 已超时，协调层已将任务放回自动重试队列。\n\n- Task: `%s`\n- Worker: `%s`\n- Previous Run: `%s`\n- Attempt: %d\n- Reason: lease expired\n- Next status: `retry_pending`",
		task.TaskID,
		task.Role,
		runID,
		task.AttemptCount,
	)
}

func leaseHumanRequiredIMBody(task *model.Task) string {
	runID := "none"
	if task != nil && task.CurrentRunID != nil && strings.TrimSpace(*task.CurrentRunID) != "" {
		runID = strings.TrimSpace(*task.CurrentRunID)
	}
	return fmt.Sprintf(
		"Agent Task lease 已超时，协调层无法安全自动重试，需要人工核验后再决定是否恢复。\n\n- Task: `%s`\n- Worker: `%s`\n- Previous Run: `%s`\n- Attempt: %d\n- Reason: %s\n- Next status: `human_required`",
		task.TaskID,
		task.Role,
		runID,
		task.AttemptCount,
		task.LastError,
	)
}
