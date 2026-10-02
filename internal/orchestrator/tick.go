package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
)

type TickRequest struct {
	CallbackBaseURL     string `json:"callback_base_url,omitempty"`
	WorkspaceRoot       string `json:"workspace_root,omitempty"`
	DispatchLimit       int    `json:"dispatch_limit,omitempty"`
	PollLimit           int    `json:"poll_limit,omitempty"`
	ReconciliationLimit int    `json:"reconciliation_limit,omitempty"`
	FinalizationLimit   int    `json:"finalization_limit,omitempty"`
	RecoveryLimit       int    `json:"recovery_limit,omitempty"`
	EffectLimit         int    `json:"effect_limit,omitempty"`
	GateLimit           int    `json:"gate_limit,omitempty"`
}

type TickResult struct {
	PolledRuns           []PollRunTickResult             `json:"polled_runs"`
	ReconciledWritebacks []WritebackReconciliationResult `json:"reconciled_writebacks"`
	FinalizedResults     []ResultFinalizationResult      `json:"finalized_results"`
	Recoveries           []RecoveryResult                `json:"recoveries"`
	DispatchedTasks      []DispatchTickResult            `json:"dispatched_tasks"`
	DispatchFailures     []DispatchTickResult            `json:"dispatch_failures"`
	GateDecisions        []MergeGateTickResult           `json:"gate_decisions,omitempty"`
	ExecutedEffects      []EffectExecutionResult         `json:"executed_effects"`
	CleanedArtifacts     []ArtifactCleanupResult         `json:"cleaned_artifacts,omitempty"`
}

type MergeGateTickResult struct {
	TaskID   string `json:"task_id"`
	HeadSHA  string `json:"head_sha,omitempty"`
	Decision string `json:"decision"`
	Reasons  string `json:"reasons,omitempty"`
	Error    string `json:"error,omitempty"`
}

type PollRunTickResult struct {
	RunID              string          `json:"run_id"`
	Status             model.RunStatus `json:"status"`
	ConversationStatus string          `json:"conversation_status"`
	ExecutionStatus    string          `json:"execution_status,omitempty"`
	Terminal           bool            `json:"terminal"`
	Applied            bool            `json:"applied"`
	ParseError         string          `json:"parse_error,omitempty"`
	Error              string          `json:"error,omitempty"`
}

type DispatchTickResult struct {
	TaskID string           `json:"task_id"`
	RunID  string           `json:"run_id,omitempty"`
	Status model.TaskStatus `json:"status,omitempty"`
	Error  string           `json:"error,omitempty"`
}

// unsupportedWorkerRoleReason 是派发循环对未知 worker 角色的统一停靠文案:
// developer/reviewer/tester 都有专属派发路径,走到这里的角色没有执行面。
const unsupportedWorkerRoleReason = "该 worker 角色暂未迁移到 Agent Runner，已转人工处理"

// testerRunnerNotConfiguredReason 是验证执行面未装配时的停靠文案:装配缺失
// 不是任务失败,回退到既有转人工行为,等执行面就绪后由人恢复或重开。
const testerRunnerNotConfiguredReason = "TesterWorker 验证执行面未装配，已转人工处理"

// parkWorkerRoleWithoutRunner 把没有执行面的任务停靠出派发队列并留痕:
// 停靠会清空 current_run_id,重取快照后由 helper 的状态门禁决定是否物化通知。
func (o *Orchestrator) parkWorkerRoleWithoutRunner(ctx context.Context, task model.Task, reason string, result *TickResult) error {
	if err := o.tasks.MarkHumanRequiredIfDispatchable(ctx, task.TaskID, reason); err != nil {
		return err
	}
	result.DispatchFailures = append(result.DispatchFailures, DispatchTickResult{
		TaskID: task.TaskID, Status: model.TaskStatusHumanRequired, Error: reason,
	})
	if parked, getErr := o.tasks.GetByTaskID(ctx, task.TaskID); getErr == nil {
		o.materializeHumanRequiredIMEffect(ctx, parked, reason)
	}
	return nil
}

// ErrTickInProgress 表示已有一个 Tick 在执行。Tick 内部包含非幂等的外部副作用
// （developer 交付的 git commit/push），并发执行会在同一 workspace 上竞争并把
// 瞬时失败误判为交付冲突，因此 Tick 必须串行。
var ErrTickInProgress = errors.New("scheduler tick is already running")

func (o *Orchestrator) Tick(ctx context.Context, req TickRequest) (*TickResult, error) {
	if !o.tickMu.TryLock() {
		return nil, ErrTickInProgress
	}
	defer o.tickMu.Unlock()
	result := &TickResult{}

	pollRun := func(run model.Run) {
		var runnerResult *ReviewerRunnerReconcileResult
		var reconcileErr error
		if run.WorkerRole == "DeveloperWorker" {
			runnerResult, reconcileErr = o.ReconcileDeveloperRunner(ctx, run.RunID)
		} else if run.WorkerRole == "TesterWorker" {
			runnerResult, reconcileErr = o.ReconcileTesterRunner(ctx, run.RunID)
		} else {
			runnerResult, reconcileErr = o.ReconcileReviewerRunner(ctx, run.RunID)
		}
		if reconcileErr != nil {
			result.PolledRuns = append(result.PolledRuns, PollRunTickResult{RunID: run.RunID, Status: run.Status, Error: reconcileErr.Error()})
			return
		}
		storedRun, getErr := o.runs.GetByRunID(ctx, run.RunID)
		if getErr != nil {
			// 单个 run 的重读失败不应放大为整个 Tick 的失败：写入该 run 自己的
			// poll 错误后继续处理剩余 run 与后续阶段。
			result.PolledRuns = append(result.PolledRuns, PollRunTickResult{RunID: run.RunID, Status: run.Status, Error: getErr.Error()})
			return
		}
		terminal := storedRun.Status == model.RunStatusSucceeded || storedRun.Status == model.RunStatusFailed || storedRun.Status == model.RunStatusBlocked
		pollResult := PollRunTickResult{
			RunID: run.RunID, Status: storedRun.Status, ExecutionStatus: string(runnerResult.Execution.Status),
			Terminal: terminal, Applied: runnerResult.Submission != nil,
		}
		if storedRun.Status == model.RunStatusFailed || storedRun.Status == model.RunStatusBlocked {
			pollResult.Error = storedRun.ErrorMessage
		}
		result.PolledRuns = append(result.PolledRuns, pollResult)
	}

	runningRuns, err := o.runs.ListRunning(ctx, req.PollLimit)
	if err != nil {
		return nil, fmt.Errorf("list running runs: %w", err)
	}
	for _, run := range runningRuns {
		if run.ExternalSystem != agentRunnerExternalSystem {
			continue
		}
		pollRun(run)
	}

	// MCP 主通道首提即终态化 run 行，此类 run 不会再进 ListRunning，但执行
	// 投影要等 CLI 进程退出后的最终观察才能闭合；补一次观察（reconcile 的
	// 状态门会把迟到的文本结果静默吸收），否则投影滞留 running 会让 artifact
	// 清理永久跳过并逐渐饿死整个清理批次。
	// 收尾观察是尽力而为的观测性步骤：查询失败不阻断 Tick 主链路（下一轮
	// Tick 自然重试），因此这里有意不像 ListRunning 那样上抛错误。
	terminalRuns, sweepErr := o.runs.ListTerminalWithActiveExecution(ctx, req.PollLimit)
	if sweepErr == nil {
		for _, run := range terminalRuns {
			pollRun(run)
		}
	}

	if o.writebacks != nil {
		reconciled, err := o.ReconcileUnknownWritebacks(ctx, req.ReconciliationLimit)
		if err != nil {
			return nil, fmt.Errorf("reconcile unknown writebacks: %w", err)
		}
		result.ReconciledWritebacks = reconciled
	}

	if o.resultSubmissions != nil {
		finalized, err := o.FinalizeRecoverableSubmissions(ctx, req.FinalizationLimit)
		if err != nil {
			return nil, fmt.Errorf("finalize recoverable result submissions: %w", err)
		}
		result.FinalizedResults = finalized
	}

	// 自动合并门禁:消费已完成的验证任务(证据合取全绿才动合并,缺一环
	// 回落人工并留审计)。开关关闭时内部零开销短路;扫描失败有意降级为
	// 日志而非上抛——门禁是可选阶段,不得拖住同轮的恢复/派发/效果执行。
	gateDecisions, gateErr := o.RunAutoMergeGate(ctx, req.GateLimit)
	if gateErr != nil {
		logGateScanFailure(gateErr)
	} else {
		result.GateDecisions = gateDecisions
	}

	recoveries, err := o.RecoverExpiredTasks(ctx, req.RecoveryLimit)
	if err != nil {
		return nil, fmt.Errorf("recover expired tasks: %w", err)
	}
	result.Recoveries = recoveries

	dispatchableTasks, err := o.tasks.ListDispatchable(ctx, req.DispatchLimit)
	if err != nil {
		return nil, fmt.Errorf("list dispatchable tasks: %w", err)
	}
	for _, task := range dispatchableTasks {
		var dispatchResult *DispatchResult
		// 仓库注册门禁（registeredrepo）：未绑定/未启用的任务不得派发。
		if err := o.assertRepositoryDispatchable(ctx, &task); err != nil {
			if errors.Is(err, ErrRepositoryNotBound) {
				// 未绑定任务 park 出 dispatchable 队列并留痕，否则会永久占满
				// 派发窗口饿死其余任务。经 RebindTask 补记 repository_id 恢复。
				if parkErr := o.parkUnboundTask(ctx, task.TaskID); parkErr != nil {
					return nil, parkErr
				}
				continue
			}
			if errors.Is(err, ErrRepositoryNotEnabled) {
				continue
			}
			return nil, err
		}
		if task.Role == "ReviewerWorker" {
			dispatchResult, err = o.DispatchReviewerTask(ctx, task.TaskID)
		} else if task.Role == "DeveloperWorker" {
			dispatchResult, err = o.DispatchDeveloperTask(ctx, task.TaskID)
		} else if task.Role == "TesterWorker" && o.testerRunner != nil {
			dispatchResult, err = o.DispatchTesterTask(ctx, task.TaskID)
		} else {
			// 未知角色,或验证任务但执行面未装配:统一停靠人工(fail-closed 到人)。
			reason := unsupportedWorkerRoleReason
			if task.Role == "TesterWorker" {
				reason = testerRunnerNotConfiguredReason
			}
			if parkErr := o.parkWorkerRoleWithoutRunner(ctx, task, reason, result); parkErr != nil {
				err = parkErr
			} else {
				continue
			}
		}
		if err != nil {
			// A concurrent Tick can list the same pending Task, but Claim is the
			// atomic ownership boundary. Losing that race is expected, not a Tick
			// failure and must not create another Run.
			if errors.Is(err, repository.ErrTaskNotClaimable) {
				continue
			}
			// 同 MR 有更早的活跃审查时派发被有意延后，等前序 settle 后下个
			// Tick 自然重试；这是预期等待，不是失败。
			if errors.Is(err, ErrReviewerDispatchDeferred) {
				continue
			}
			failed := DispatchTickResult{TaskID: task.TaskID, Error: err.Error()}
			if current, getErr := o.tasks.GetByTaskID(ctx, task.TaskID); getErr == nil {
				failed.Status = current.Status
				if current.CurrentRunID != nil {
					failed.RunID = *current.CurrentRunID
				}
			}
			result.DispatchFailures = append(result.DispatchFailures, failed)
			continue
		}
		result.DispatchedTasks = append(result.DispatchedTasks, DispatchTickResult{
			TaskID: dispatchResult.Task.TaskID,
			RunID:  dispatchResult.Run.RunID,
			Status: dispatchResult.Task.Status,
		})
	}

	if req.EffectLimit > 0 {
		effectResults, err := o.ExecutePendingEffects(ctx, req.EffectLimit)
		if err != nil {
			return nil, fmt.Errorf("execute pending effects: %w", err)
		}
		result.ExecutedEffects = effectResults

		// running 挂起态回收:执行中崩溃遗留的效果由 sweep 置 failed,不无限滞留。
		staleResults, err := o.RecoverStaleEffects(ctx, req.EffectLimit)
		if err != nil {
			return nil, fmt.Errorf("recover stale effects: %w", err)
		}
		result.ExecutedEffects = append(result.ExecutedEffects, staleResults...)
	}

	result.CleanedArtifacts = o.cleanupExpiredArtifacts(ctx)
	return result, nil
}
