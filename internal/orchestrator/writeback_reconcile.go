package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
)

type WritebackReconciliationResult struct {
	WritebackID string                  `json:"writeback_id"`
	DedupeKey   string                  `json:"dedupe_key"`
	State       codehost.ReconcileState `json:"state"`
	Status      model.WritebackStatus   `json:"status"`
	Error       string                  `json:"error,omitempty"`
}

func (o *Orchestrator) ReconcileUnknownWritebacks(ctx context.Context, limit int) ([]WritebackReconciliationResult, error) {
	if o.writebacks == nil {
		return nil, errors.New("writeback repository is required")
	}
	unknown, err := o.writebacks.ListUnknown(ctx, limit)
	if err != nil {
		return nil, err
	}
	if len(unknown) == 0 {
		return []WritebackReconciliationResult{}, nil
	}
	results := make([]WritebackReconciliationResult, 0, len(unknown))
	for i := range unknown {
		writeback := &unknown[i]
		// 对账与发送共享同一执行权（同一 scope）：发送方持有期间本实例不
		// 对账、不写回，杜绝迟到观察覆盖已成功状态的窗口。抢不到即跳过
		// 本轮，下一轮自然重试。
		lease, won, leaseErr := o.acquireWorkItemLease(ctx, codeHostWritebackLeaseScope+writeback.DedupeKey, "codehost-writeback", codeHostWritebackLeaseTTL)
		if leaseErr != nil {
			return nil, leaseErr
		}
		if !won {
			continue
		}
		reconciled, reconcileErr := o.reconcileOneWriteback(ctx, writeback)
		o.releaseWorkItemLease(lease)
		if reconcileErr != nil {
			return nil, reconcileErr
		}
		results = append(results, reconciled)
	}
	return results, nil
}

// reconcileOneWriteback 对账单个写回（调用方须已持有该写回的执行权）：
// 所有状态写回都带期望状态条件，并发前移（来自发送路径的成功写入或另一次
// 对账）时放弃本次写回，不覆盖库内新状态。
func (o *Orchestrator) reconcileOneWriteback(ctx context.Context, writeback *model.Writeback) (WritebackReconciliationResult, error) {
	expected := writeback.Status
	backend, backendErr := o.resolveCodeHostBackend(ctx, writeback)
	if backendErr != nil {
		// 无法解析按仓 backend（如未绑定）：按对账失败留痕，达上限升级人工。
		writeback.ReconcileAttemptCount++
		reconciledAt := o.now()
		writeback.LastReconciledAt = &reconciledAt
		writeback.ErrorMessage = backendErr.Error()
		result := WritebackReconciliationResult{
			WritebackID: writeback.WritebackID,
			DedupeKey:   writeback.DedupeKey,
			State:       codehost.ReconcileStateUnknown,
			Status:      writeback.Status,
			Error:       backendErr.Error(),
		}
		if writeback.Kind == CodeHostWritebackAddMergeRequestLabels && writeback.ReconcileAttemptCount >= o.policy.normalized().MaxAttempts {
			writeback.Status = model.WritebackStatusFailed
			result.Status = writeback.Status
		}
		applied, err := o.writebacks.UpdateGuardedIfStatus(ctx, writeback, expected)
		if err != nil {
			return result, err
		}
		if !applied {
			return result, nil
		}
		if writeback.ReconcileAttemptCount >= o.policy.normalized().MaxAttempts && writeback.Kind != CodeHostWritebackAddMergeRequestLabels {
			if err := o.escalateUnknownWriteback(ctx, writeback, backendErr.Error()); err != nil {
				return result, err
			}
		}
		return result, nil
	}
	reconciler, ok := backend.(codehost.Reconciler)
	if !ok {
		return WritebackReconciliationResult{}, errors.New("codehost backend does not support writeback reconciliation")
	}
	request, err := buildCodeHostReconcileRequest(writeback)
	if err != nil {
		return WritebackReconciliationResult{}, err
	}
	writeback.ReconcileAttemptCount++
	reconciledAt := o.now()
	writeback.LastReconciledAt = &reconciledAt
	remote, reconcileErr := reconciler.ReconcileWriteback(ctx, request)
	result := WritebackReconciliationResult{
		WritebackID: writeback.WritebackID,
		DedupeKey:   writeback.DedupeKey,
		State:       remote.State,
		Status:      writeback.Status,
	}
	if reconcileErr != nil {
		writeback.ErrorMessage = reconcileErr.Error()
		result.State = codehost.ReconcileStateUnknown
		result.Error = reconcileErr.Error()
		if writeback.Kind == CodeHostWritebackAddMergeRequestLabels && writeback.ReconcileAttemptCount >= o.policy.normalized().MaxAttempts {
			writeback.Status = model.WritebackStatusFailed
			result.Status = writeback.Status
		}
		applied, err := o.writebacks.UpdateGuardedIfStatus(ctx, writeback, expected)
		if err != nil {
			return result, err
		}
		if !applied {
			return result, nil
		}
		if writeback.ReconcileAttemptCount >= o.policy.normalized().MaxAttempts && writeback.Kind != CodeHostWritebackAddMergeRequestLabels {
			if err := o.escalateUnknownWriteback(ctx, writeback, reconcileErr.Error()); err != nil {
				return result, err
			}
		}
		return result, nil
	}
	switch remote.State {
	case codehost.ReconcileStateFound:
		writeback.Status = model.WritebackStatusSucceeded
		writeback.ExternalID = remote.ExternalID
		writeback.ExternalURL = remote.ExternalURL
		writeback.ErrorMessage = ""
		responseJSON, err := marshalReconcileResponse(remote.Response)
		if err != nil {
			return result, err
		}
		writeback.ResponseJSON = responseJSON
	case codehost.ReconcileStateAbsent:
		// 迟到的 absent 观察不得回退已完成状态：条件写命中时说明发送方
		// 已把该写回推进到 succeeded，本次写回放弃（返回 false）。
		writeback.Status = model.WritebackStatusPending
		writeback.ErrorMessage = ""
	case codehost.ReconcileStateUnknown:
		writeback.Status = model.WritebackStatusUnknown
		if writeback.Kind == CodeHostWritebackAddMergeRequestLabels && writeback.ReconcileAttemptCount >= o.policy.normalized().MaxAttempts {
			writeback.Status = model.WritebackStatusFailed
			writeback.ErrorMessage = "noncritical reviewer takeover label outcome remains unknown"
		}
	default:
		return result, fmt.Errorf("unsupported reconcile state %q", remote.State)
	}
	applied, err := o.writebacks.UpdateGuardedIfStatus(ctx, writeback, expected)
	if err != nil {
		return result, err
	}
	if !applied {
		// 状态已被并发前移（发送成功/另一次对账）：本次观察作废，不升级、
		// 不物化产物（并发方负责）。
		return result, nil
	}
	if writeback.Status == model.WritebackStatusUnknown && writeback.ReconcileAttemptCount >= o.policy.normalized().MaxAttempts && writeback.Kind != CodeHostWritebackAddMergeRequestLabels {
		if err := o.escalateUnknownWriteback(ctx, writeback, "codehost writeback outcome remains unknown after reconciliation limit"); err != nil {
			return result, err
		}
	}
	if writeback.Status == model.WritebackStatusSucceeded {
		if err := o.materializeCodeHostWritebackArtifact(ctx, writeback); err != nil {
			return result, err
		}
	}
	result.Status = writeback.Status
	return result, nil
}

func (o *Orchestrator) escalateUnknownWriteback(ctx context.Context, writeback *model.Writeback, reason string) error {
	message := fmt.Sprintf("codehost writeback %s requires human reconciliation: %s", writeback.DedupeKey, reason)
	task, err := o.tasks.GetByTaskID(ctx, writeback.TaskID)
	if err != nil {
		return err
	}
	if task.Status == model.TaskStatusRunning || task.Status == model.TaskStatusFinalizing {
		previous := task.Status
		if err := o.stateMachine.Transition(task, model.TaskStatusHumanRequired); err != nil {
			return err
		}
		o.policy.Apply(task, previous, task.Status, o.now())
		task.LastError = message
		// 与写回停车路径同规:LastError 与 IM cause 一律 token 掩码,凭据不落
		// 库也不进通知(当前 message 来源无凭据,属纵深防御)。
		if repo := o.lookupRegisteredRepo(ctx, task); repo != nil {
			token := strings.TrimSpace(repo.Token)
			if token != "" {
				task.LastError = strings.ReplaceAll(task.LastError, token, "***")
			}
		}
		if err := o.tasks.Update(ctx, task); err != nil {
			return err
		}
		o.materializeHumanRequiredIMEffect(ctx, task, task.LastError)
	}
	run, err := o.runs.GetByRunID(ctx, writeback.RunID)
	if err != nil {
		return err
	}
	if run.Status == model.RunStatusRunning || run.Status == model.RunStatusFinalizing {
		run.Status = model.RunStatusBlocked
		run.ErrorMessage = message
		completedAt := o.now()
		run.CompletedAt = &completedAt
		if err := o.runs.Update(ctx, run); err != nil {
			return err
		}
	}
	if o.resultSubmissions == nil {
		return nil
	}
	submission, err := o.resultSubmissions.GetByRunID(ctx, writeback.RunID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	submission.Status = model.ResultSubmissionStatusUnknown
	submission.ErrorMessage = message
	submission.LeaseUntil = nil
	return o.resultSubmissions.Update(ctx, submission)
}

func buildCodeHostReconcileRequest(writeback *model.Writeback) (codehost.ReconcileRequest, error) {
	intent, err := decodeCodeHostWritebackIntent(writeback)
	if err != nil {
		return codehost.ReconcileRequest{}, err
	}
	marker := writeback.OperationMarker
	if marker == "" {
		marker = CodeHostOperationMarker(writeback.DedupeKey)
	}
	return codehost.ReconcileRequest{
		DedupeKey:       writeback.DedupeKey,
		OperationMarker: marker,
		Kind:            writeback.Kind,
		ProjectID:       writeback.ProjectID,
		TargetType:      writeback.TargetType,
		TargetIID:       writeback.TargetIID,
		SourceBranch:    intent.SourceBranch,
		TargetBranch:    intent.TargetBranch,
		Labels:          intent.Labels,
	}, nil
}

func marshalReconcileResponse(response any) (string, error) {
	if response == nil {
		return "{}", nil
	}
	raw, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("marshal reconcile response: %w", err)
	}
	return string(raw), nil
}
