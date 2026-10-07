package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/workers"
)

func (o *Orchestrator) DispatchDeveloperTask(ctx context.Context, taskID string) (*DispatchResult, error) {
	if o.developerRunner == nil {
		return nil, errors.New("developer runner is required")
	}
	task, err := o.tasks.GetByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.Role != "DeveloperWorker" {
		return nil, errors.New("runner developer dispatch requires DeveloperWorker task")
	}
	if task.Status != model.TaskStatusPending && task.Status != model.TaskStatusRetryPending {
		return nil, repository.ErrTaskNotClaimable
	}
	run := &model.Run{RunID: model.NewID("run"), TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusPending}
	run, task, err = o.StartRun(ctx, task.TaskID, run)
	if err != nil {
		return nil, err
	}
	run.ExternalSystem = agentRunnerExternalSystem
	run.ExternalID = run.RunID
	pkg, err := workers.BuildRunnerTaskPackage(task, run)
	if err != nil {
		o.recordDispatchFailure(ctx, task, run, err)
		return nil, err
	}
	rawInput, err := json.Marshal(pkg)
	if err != nil {
		o.recordDispatchFailure(ctx, task, run, err)
		return nil, err
	}
	run.InputJSON = string(rawInput)
	if err := o.runs.UpdateDispatchProjection(ctx, run.RunID, run.ExternalSystem, run.ExternalID, run.InputJSON); err != nil {
		return nil, err
	}
	workspaceRequest, err := buildDeveloperWorkspaceRequest(ctx, task, run, o.developerRunnerSettings)
	if err != nil {
		o.recordDispatchFailure(ctx, task, run, err)
		return nil, err
	}
	if _, err := o.developerRunner.Start(ctx, DeveloperRunnerStartRequest{
		Task: task, Run: run, Workspace: workspaceRequest,
		Provider: o.developerRunnerSettings.Provider, Timeout: o.developerRunnerSettings.TaskTimeout,
	}); err != nil {
		o.recordDispatchFailure(ctx, task, run, err)
		return nil, err
	}
	return &DispatchResult{Task: task, Run: run}, nil
}

func (o *Orchestrator) ReconcileDeveloperRunner(ctx context.Context, runID string) (*DeveloperRunnerReconcileResult, error) {
	if o.developerRunner == nil {
		return nil, errors.New("developer runner is required")
	}
	result, err := o.developerRunner.Reconcile(ctx, runID)
	if errors.Is(err, repository.ErrNotFound) {
		if _, repairErr := o.rebuildDeveloperRunnerProjection(ctx, runID); repairErr != nil {
			return nil, fmt.Errorf("rebuild developer runner projection: %w", repairErr)
		}
		result, err = o.developerRunner.Reconcile(ctx, runID)
	}
	if err != nil {
		if runnerResultNeedsConvergence(err) && result != nil && result.Execution != nil && result.Execution.Status == model.AgentExecutionStatusSucceeded {
			if blockErr := o.recordInvalidRunnerResult(ctx, runID, err); blockErr != nil {
				return nil, blockErr
			}
			return result, nil
		}
		return nil, err
	}
	if result == nil || result.Execution == nil {
		return nil, errors.New("developer runner returned no execution observation")
	}
	if result.Absorbed {
		// run 已终态：不续租、不升级、不要求提交——避免任务推进后
		// RenewActiveLease 的 ErrTaskLeaseNotRenewable 噪声。
		return result, nil
	}
	switch result.Execution.Status {
	case model.AgentExecutionStatusAccepted, model.AgentExecutionStatusStarting, model.AgentExecutionStatusRunning:
		run, err := o.runs.GetByRunID(ctx, runID)
		if err != nil {
			return nil, err
		}
		if err := o.tasks.RenewActiveLease(ctx, run.TaskID, run.RunID, o.policy.LeaseUntil(o.now())); err != nil {
			return nil, err
		}
	case model.AgentExecutionStatusFailed, model.AgentExecutionStatusCancelled, model.AgentExecutionStatusInterrupted:
		if err := o.recordReviewerRunnerFailure(ctx, runID, result.Execution.ErrorCode); err != nil {
			return nil, err
		}
	case model.AgentExecutionStatusSucceeded:
		if result.Submission == nil {
			return nil, errors.New("succeeded developer runner execution has no accepted submission")
		}
	default:
		return nil, fmt.Errorf("unsupported developer runner projection status %q", result.Execution.Status)
	}
	return result, nil
}

func (o *Orchestrator) recordInvalidRunnerResult(ctx context.Context, runID string, cause error) error {
	message := strings.TrimSpace(cause.Error())
	if message == "" {
		message = "agent runner returned an invalid business result"
	}
	if err := o.tasks.BlockRunAndRequireHuman(ctx, runID, message, o.now()); err != nil {
		return err
	}
	// BlockRunAndRequireHuman 幂等:run 已终态时静默返回,task 未必因本次调用迁移;
	// 重取 task 交给 helper 的 human_required 状态门禁裁决。通知 best-effort,
	// 取数失败不改变本函数的返回语义。
	if run, err := o.runs.GetByRunID(ctx, runID); err == nil {
		if task, err := o.tasks.GetByTaskID(ctx, run.TaskID); err == nil {
			o.materializeHumanRequiredIMEffect(ctx, task, message)
		}
	}
	return nil
}

func (o *Orchestrator) rebuildDeveloperRunnerProjection(ctx context.Context, runID string) (*model.AgentExecution, error) {
	run, err := o.runs.GetByRunID(ctx, runID)
	if err != nil {
		return nil, err
	}
	task, err := o.tasks.GetByTaskID(ctx, run.TaskID)
	if err != nil {
		return nil, err
	}
	workspaceRequest, err := buildDeveloperWorkspaceRequest(ctx, task, run, o.developerRunnerSettings)
	if err != nil {
		return nil, err
	}
	return o.developerRunner.Start(ctx, DeveloperRunnerStartRequest{
		Task: task, Run: run, Workspace: workspaceRequest,
		Provider: o.developerRunnerSettings.Provider, Timeout: o.developerRunnerSettings.TaskTimeout,
	})
}
