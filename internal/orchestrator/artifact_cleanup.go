package orchestrator

import (
	"context"
	"errors"
	"strings"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
)

const (
	artifactRetention        = 7 * 24 * time.Hour
	artifactCleanupBatchSize = 10
	// artifactPurgeLeaseWindow 是清理归属哨兵的有效期；失败或持有者退出后，到期才允许重试。
	artifactPurgeLeaseWindow = 30 * time.Minute
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
	if o.runWorkspaceCleaner == nil {
		return nil
	}
	now := o.now().UTC()
	runs, err := o.runs.ListSucceededForArtifactCleanup(ctx, now.Add(-artifactRetention), now.Add(-artifactPurgeLeaseWindow), artifactCleanupBatchSize)
	if err != nil {
		return []ArtifactCleanupResult{{Error: err.Error()}}
	}
	results := make([]ArtifactCleanupResult, 0, len(runs))
	for _, run := range runs {
		result := o.purgeSucceededRunArtifacts(ctx, run)
		results = append(results, result)
	}
	if stops, ok := o.stops.(stoppedArtifactCleanupPort); ok {
		stopped, err := stops.ListCleanupReady(ctx, now.Add(-artifactRetention), now.Add(-artifactPurgeLeaseWindow), artifactCleanupBatchSize)
		if err != nil {
			return append(results, ArtifactCleanupResult{Error: err.Error()})
		}
		for _, run := range stopped {
			results = append(results, o.purgeStoppedRunArtifacts(ctx, stops, run))
		}
	}
	return results
}

func (o *Orchestrator) purgeSucceededRunArtifacts(ctx context.Context, run model.Run) ArtifactCleanupResult {
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
	claimedAt := o.now().UTC()
	claimID := model.NewID("purge")
	applied, err := o.runs.MarkArtifactsPurgingIfUnclean(ctx, run.RunID, claimID, claimedAt, claimedAt.Add(-artifactPurgeLeaseWindow))
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if !applied {
		return result
	}
	return o.deleteRunArtifacts(ctx, run, result, claimID)
}

type stoppedArtifactCleanupPort interface {
	ListCleanupReady(context.Context, time.Time, time.Time, int) ([]model.Run, error)
	ClaimCleanup(context.Context, string, string, time.Time, time.Time) (bool, error)
}

func (o *Orchestrator) purgeStoppedRunArtifacts(ctx context.Context, stops stoppedArtifactCleanupPort, run model.Run) ArtifactCleanupResult {
	result := ArtifactCleanupResult{RunID: run.RunID, ExecutionID: run.ExternalID}
	execution, err := o.agentExecutions.GetByRunID(ctx, run.RunID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		result.Error = err.Error()
		return result
	}
	if execution != nil {
		result.ExecutionID = execution.ExecutionID
	}
	now := o.now().UTC()
	claimID := model.NewID("purge")
	claimed, err := stops.ClaimCleanup(ctx, run.RunID, claimID, now, now.Add(-artifactPurgeLeaseWindow))
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if !claimed {
		return result
	}
	return o.deleteRunArtifacts(ctx, run, result, claimID)
}

func (o *Orchestrator) deleteRunArtifacts(ctx context.Context, run model.Run, result ArtifactCleanupResult, claimID string) ArtifactCleanupResult {
	if err := o.runWorkspaceCleaner.RemoveRunWorkspace(ctx, run.TaskID, run.RunID); err != nil {
		result.Error = err.Error()
		return result
	}
	var deleteErr error
	if result.ExecutionID != "" {
		if routed, ok := o.runnerArtifacts.(interface {
			DeleteForRun(context.Context, string, string) error
		}); ok {
			deleteErr = routed.DeleteForRun(ctx, run.RunID, result.ExecutionID)
		} else {
			deleteErr = o.runnerArtifacts.Delete(ctx, result.ExecutionID)
		}
	}
	if deleteErr != nil {
		result.Error = deleteErr.Error()
		return result
	}
	completed, err := o.runs.CompleteArtifactsPurging(ctx, run.RunID, claimID, o.now().UTC())
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Purged = completed
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
