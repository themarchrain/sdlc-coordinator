package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"sdlc-coordinator/internal/agent"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/workers"
	"sdlc-coordinator/internal/workspace"
)

// DispatchTesterTask 把验证任务送进 runner 池:工作区按任务上下文冻结的
// 头部 SHA 只读检出,验证处方(下限命令)从注册仓行读取并写进任务包。
func (o *Orchestrator) DispatchTesterTask(ctx context.Context, taskID string) (*DispatchResult, error) {
	if o.testerRunner == nil {
		return nil, errors.New("tester runner is required")
	}
	task, err := o.tasks.GetByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.Role != "TesterWorker" {
		return nil, errors.New("runner tester dispatch requires TesterWorker task")
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
	// 仓行单次查询双消费:任务包处方与工作区 URL/凭据同源。
	repoRow, err := o.testerRunnerSettings.RegisteredRepositoryFor(ctx, task)
	if err != nil {
		o.recordDispatchFailure(ctx, task, run, err)
		return nil, err
	}
	pkg, err := o.buildTesterTaskPackage(ctx, task, run, repoRow)
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
		// InputJSON 已组好但落库失败:任务回退重试路径,不留孤儿 run。
		o.recordDispatchFailure(ctx, task, run, err)
		return nil, err
	}
	workspaceRequest, err := buildTesterWorkspaceRequest(task, run, repoRow, o.testerRunnerSettings)
	if err != nil {
		o.recordDispatchFailure(ctx, task, run, err)
		return nil, err
	}
	if _, err := o.testerRunner.Start(ctx, TesterRunnerStartRequest{
		Task: task, Run: run, Workspace: workspaceRequest, Provider: o.testerRunnerSettings.Provider, Timeout: o.testerRunnerSettings.TaskTimeout,
	}); err != nil {
		o.recordDispatchFailure(ctx, task, run, err)
		return nil, err
	}
	return &DispatchResult{Task: task, Run: run}, nil
}

func (o *Orchestrator) ReconcileTesterRunner(ctx context.Context, runID string) (*ReviewerRunnerReconcileResult, error) {
	if o.testerRunner == nil {
		return nil, errors.New("tester runner is required")
	}
	result, err := o.testerRunner.Reconcile(ctx, runID)
	if errors.Is(err, repository.ErrNotFound) {
		// 投影缺失时重建；已有派发意图的 Runner 404 属于结果未知，不能据此重发。
		if _, repairErr := o.rebuildTesterRunnerProjection(ctx, runID); repairErr != nil {
			return nil, fmt.Errorf("rebuild tester runner projection: %w", repairErr)
		}
		result, err = o.testerRunner.Reconcile(ctx, runID)
	}
	if err != nil {
		var invalidResult invalidRunnerResultError
		if errors.As(err, &invalidResult) && result != nil && result.Execution != nil && result.Execution.Status == model.AgentExecutionStatusSucceeded {
			if blockErr := o.recordInvalidRunnerResult(ctx, runID, err); blockErr != nil {
				return nil, blockErr
			}
			return result, nil
		}
		return nil, err
	}
	if result == nil || result.Execution == nil {
		return nil, errors.New("tester runner returned no execution observation")
	}
	if result.Absorbed {
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
			return nil, errors.New("succeeded tester runner execution has no accepted submission")
		}
	default:
		return nil, fmt.Errorf("unsupported tester runner projection status %q", result.Execution.Status)
	}
	return result, nil
}

func (o *Orchestrator) rebuildTesterRunnerProjection(ctx context.Context, runID string) (*model.AgentExecution, error) {
	run, err := o.runs.GetByRunID(ctx, runID)
	if err != nil {
		return nil, err
	}
	task, err := o.tasks.GetByTaskID(ctx, run.TaskID)
	if err != nil {
		return nil, err
	}
	workspaceRequest, err := buildTesterWorkspaceRequestForRebuild(ctx, task, run, o.testerRunnerSettings)
	if err != nil {
		return nil, err
	}
	return o.testerRunner.Start(ctx, TesterRunnerStartRequest{
		Task: task, Run: run, Workspace: workspaceRequest, Provider: o.testerRunnerSettings.Provider, Timeout: o.testerRunnerSettings.TaskTimeout,
	})
}

// buildTesterTaskPackage 构造验证任务包:处方=下限∪复放(见
// buildVerificationPlan),任务包经 run.InputJSON 持久化,投影重建时由
// bundle 侧恢复同一处方。
func (o *Orchestrator) buildTesterTaskPackage(ctx context.Context, task *model.Task, run *model.Run, repoRow *registeredrepo.RegisteredRepository) (agent.TaskPackage, error) {
	pkg, err := workers.BuildRunnerTaskPackage(task, run)
	if err != nil {
		return agent.TaskPackage{}, err
	}
	if plan := o.buildVerificationPlan(ctx, task, repoRow); plan != nil {
		pkg.Verification = plan
	}
	return pkg, nil
}

// buildTesterWorkspaceRequest 组装验证工作区请求:与 reviewer 同一只读检出
// 语义(URL 与凭据按注册仓行派生),固定到上下文冻结的头部 SHA。
func buildTesterWorkspaceRequest(task *model.Task, run *model.Run, repoRow *registeredrepo.RegisteredRepository, settings ReviewerRunnerSettings) (workspace.ReviewerRequest, error) {
	pkg, err := buildTesterPackageForWorkspace(task, run)
	if err != nil {
		return workspace.ReviewerRequest{}, err
	}
	return assembleTesterWorkspaceRequest(task, run, repoRow, pkg, settings)
}

// buildTesterWorkspaceRequestForRebuild 服务于投影重建:此时 run.InputJSON
// 已持久化派发时的任务包(含处方),直接复用,不重新解析仓行处方。
func buildTesterWorkspaceRequestForRebuild(ctx context.Context, task *model.Task, run *model.Run, settings ReviewerRunnerSettings) (workspace.ReviewerRequest, error) {
	repoRow, err := settings.RegisteredRepositoryFor(ctx, task)
	if err != nil {
		return workspace.ReviewerRequest{}, err
	}
	pkg, err := buildTesterPackageForWorkspace(task, run)
	if err != nil {
		return workspace.ReviewerRequest{}, err
	}
	return assembleTesterWorkspaceRequest(task, run, repoRow, pkg, settings)
}

func buildTesterPackageForWorkspace(task *model.Task, run *model.Run) (agent.TaskPackage, error) {
	return workers.BuildRunnerTaskPackage(task, run)
}

func assembleTesterWorkspaceRequest(task *model.Task, run *model.Run, repoRow *registeredrepo.RegisteredRepository, pkg agent.TaskPackage, settings ReviewerRunnerSettings) (workspace.ReviewerRequest, error) {
	cloneURL := repoRow.CloneURL()
	if cloneURL == "" {
		return workspace.ReviewerRequest{}, fmt.Errorf("registered repository %s cannot derive clone url: base_url or path is empty", boundRepositoryID(task))
	}
	if _, err := reviewerRepository(pkg.Repos, task.TargetRepo); err != nil {
		return workspace.ReviewerRequest{}, err
	}
	mergeRequest, _ := pkg.Context["merge_request"].(map[string]any)
	return workspace.ReviewerRequest{
		TaskID: task.TaskID, RunID: run.RunID,
		Project: task.TargetRepo, RepositoryURL: cloneURL,
		SourceBranch: stringContextField(mergeRequest, "source_branch"),
		TargetBranch: stringContextField(mergeRequest, "target_branch"),
		HeadSHA:      stringContextField(mergeRequest, "head_sha"), BaseSHA: stringContextField(mergeRequest, "base_sha"),
		RequestedReviewMode: workspace.ReviewModeFull, Credential: gitCredentialFrom(repoRow),
		WorkerRole: "TesterWorker",
	}, nil
}
