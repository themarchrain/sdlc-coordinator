package orchestrator

import (
	"context"
	"errors"
	"strings"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/workspace"
)

const (
	artifactRetention        = 7 * 24 * time.Hour
	artifactCleanupBatchSize = 10
)

type ArtifactCleanupResult struct {
	RunID       string `json:"run_id"`
	ExecutionID string `json:"execution_id,omitempty"`
	Purged      bool   `json:"purged"`
	Error       string `json:"error,omitempty"`
}

func (o *Orchestrator) cleanupExpiredArtifacts(ctx context.Context) []ArtifactCleanupResult {
	if o.agentExecutions == nil || o.runnerArtifacts == nil || o.resultSubmissions == nil {
		return nil
	}
	workspaceRoot := strings.TrimSpace(o.reviewerRunnerSettings.WorkspaceRoot)
	if workspaceRoot == "" {
		workspaceRoot = strings.TrimSpace(o.developerRunnerSettings.WorkspaceRoot)
	}
	if workspaceRoot == "" && o.runWorkspaceCleaner == nil {
		return nil
	}
	cutoff := o.now().UTC().Add(-artifactRetention)
	runs, err := o.runs.ListSucceededForArtifactCleanup(ctx, cutoff, artifactCleanupBatchSize)
	if err != nil {
		return []ArtifactCleanupResult{{Error: err.Error()}}
	}
	results := make([]ArtifactCleanupResult, 0, len(runs))
	for _, run := range runs {
		result := o.purgeSucceededRunArtifacts(ctx, workspaceRoot, run)
		results = append(results, result)
	}
	return results
}

func (o *Orchestrator) purgeSucceededRunArtifacts(ctx context.Context, workspaceRoot string, run model.Run) ArtifactCleanupResult {
	result := ArtifactCleanupResult{RunID: run.RunID}
	execution, err := o.agentExecutions.GetByRunID(ctx, run.RunID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return result
		}
		result.Error = err.Error()
		return result
	}
	result.ExecutionID = execution.ExecutionID
	if !fullySucceededExecution(execution) {
		return result
	}
	submission, err := o.resultSubmissions.GetByRunID(ctx, run.RunID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return result
		}
		result.Error = err.Error()
		return result
	}
	if submission.Status != model.ResultSubmissionStatusSucceeded || strings.TrimSpace(submission.ErrorMessage) != "" {
		return result
	}
	if o.writebacks != nil {
		writebacks, listErr := o.writebacks.ListByRunID(ctx, run.RunID)
		if listErr != nil {
			result.Error = listErr.Error()
			return result
		}
		for _, writeback := range writebacks {
			if writeback.Status != model.WritebackStatusSucceeded {
				return result
			}
		}
	}
	if o.effects != nil {
		effects, listErr := o.effects.ListByRunID(ctx, run.RunID)
		if listErr != nil {
			result.Error = listErr.Error()
			return result
		}
		for _, effect := range effects {
			// 投递未核验与 succeeded/skipped 一样放行:它是无人工出路的终态,
			// 挡着会让 run 产物永久滞留清理候选并占满批次;送达凭据与原因
			// 元数据都在效果记录中,不依赖产物。
			if effect.Status != model.EffectStatusSucceeded && effect.Status != model.EffectStatusSkipped && effect.Status != model.EffectStatusDeliveryUnverified {
				return result
			}
		}
	}
	if o.runWorkspaceCleaner != nil {
		if err := o.runWorkspaceCleaner.RemoveRunWorkspace(ctx, run.RunID); err != nil {
			result.Error = err.Error()
			return result
		}
	} else if err := workspace.RemoveRunDirectory(workspaceRoot, run.RunID); err != nil {
		result.Error = err.Error()
		return result
	}
	if err := o.runnerArtifacts.Delete(ctx, execution.ExecutionID); err != nil {
		result.Error = err.Error()
		return result
	}
	now := o.now().UTC()
	run.ArtifactsPurgedAt = &now
	if err := o.runs.Update(ctx, &run); err != nil {
		result.Error = err.Error()
		return result
	}
	result.Purged = true
	return result
}

func fullySucceededExecution(execution *model.AgentExecution) bool {
	if execution == nil {
		return false
	}
	return execution.Status == model.AgentExecutionStatusSucceeded &&
		strings.TrimSpace(execution.ResultHash) != "" &&
		strings.TrimSpace(execution.ErrorCode) == "" &&
		strings.TrimSpace(execution.ErrorMessage) == ""
}
