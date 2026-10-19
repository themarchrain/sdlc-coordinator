package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"

	"sdlc-coordinator/internal/agent"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/workers"
	"sdlc-coordinator/internal/workspace"
)

const agentRunnerExternalSystem = "agent-runner"

// ErrReviewerDispatchDeferred 表示同一 MR 存在更早创建且仍活跃的审查任务，本轮
// 派发有意延后：多轮审查并行会导致结论乱序写回（增量结论先于完整结论落 MR）。
// Tick 层把它视为预期等待而非失败。
var ErrReviewerDispatchDeferred = errors.New("reviewer dispatch deferred: an earlier review for this merge request is still active")

func reviewerDedupePrefix(task *model.Task) (string, error) {
	parts := strings.Split(task.DedupeKey, ":")
	if len(parts) != 5 || parts[1] != "mr-review" || parts[2] == "" || parts[4] == "" {
		return "", errors.New("reviewer dispatch requires a review dedupe key")
	}
	switch parts[0] {
	case "github", "gitlab", "gitea":
	default:
		return "", errors.New("unsupported reviewer dedupe key platform")
	}
	iid, err := strconv.Atoi(parts[3])
	if err != nil || iid <= 0 {
		return "", errors.New("reviewer dispatch requires merge request iid")
	}
	return strings.Join(parts[:4], ":") + ":", nil
}

func (o *Orchestrator) DispatchReviewerTask(ctx context.Context, taskID string) (*DispatchResult, error) {
	if o.reviewerRunner == nil {
		return nil, errors.New("reviewer runner is required")
	}
	task, err := o.tasks.GetByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.Role != "ReviewerWorker" {
		return nil, errors.New("runner reviewer dispatch requires ReviewerWorker task")
	}
	if task.Status != model.TaskStatusPending && task.Status != model.TaskStatusRetryPending {
		return nil, repository.ErrTaskNotClaimable
	}
	if prefix, prefixErr := reviewerDedupePrefix(task); prefixErr == nil {
		active, err := o.tasks.HasActiveReviewerTaskForDedupePrefix(ctx, prefix, task.TaskID)
		if err != nil {
			return nil, err
		}
		if active {
			return nil, ErrReviewerDispatchDeferred
		}
	}
	run := &model.Run{RunID: model.NewID("run"), TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusPending}
	run, task, err = o.StartRun(ctx, task.TaskID, run)
	if err != nil {
		return nil, err
	}
	_ = o.markReviewerTakenOver(ctx, task, run)
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
	workspaceRequest, err := buildReviewerWorkspaceRequest(ctx, task, run, o.reviewerRunnerSettings)
	if err != nil {
		o.recordDispatchFailure(ctx, task, run, err)
		return nil, err
	}
	if _, err := o.reviewerRunner.Start(ctx, ReviewerRunnerStartRequest{
		Task: task, Run: run, Workspace: workspaceRequest, Provider: o.reviewerRunnerSettings.Provider, Timeout: o.reviewerRunnerSettings.TaskTimeout,
	}); err != nil {
		o.recordDispatchFailure(ctx, task, run, err)
		return nil, err
	}
	return &DispatchResult{Task: task, Run: run}, nil
}

func (o *Orchestrator) ReconcileReviewerRunner(ctx context.Context, runID string) (*ReviewerRunnerReconcileResult, error) {
	if o.reviewerRunner == nil {
		return nil, errors.New("reviewer runner is required")
	}
	result, err := o.reviewerRunner.Reconcile(ctx, runID)
	if errors.Is(err, repository.ErrNotFound) {
		// 投影缺失时重建；已有派发意图的 Runner 404 属于结果未知，不能据此重发。
		if _, repairErr := o.rebuildReviewerRunnerProjection(ctx, runID); repairErr != nil {
			return nil, fmt.Errorf("rebuild reviewer runner projection: %w", repairErr)
		}
		result, err = o.reviewerRunner.Reconcile(ctx, runID)
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
		return nil, errors.New("reviewer runner returned no execution observation")
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
			return nil, errors.New("succeeded reviewer runner execution has no accepted submission")
		}
	default:
		return nil, fmt.Errorf("unsupported reviewer runner projection status %q", result.Execution.Status)
	}
	return result, nil
}

func (o *Orchestrator) rebuildReviewerRunnerProjection(ctx context.Context, runID string) (*model.AgentExecution, error) {
	run, err := o.runs.GetByRunID(ctx, runID)
	if err != nil {
		return nil, err
	}
	task, err := o.tasks.GetByTaskID(ctx, run.TaskID)
	if err != nil {
		return nil, err
	}
	workspaceRequest, err := buildReviewerWorkspaceRequest(ctx, task, run, o.reviewerRunnerSettings)
	if err != nil {
		return nil, err
	}
	return o.reviewerRunner.Start(ctx, ReviewerRunnerStartRequest{
		Task: task, Run: run, Workspace: workspaceRequest, Provider: o.reviewerRunnerSettings.Provider, Timeout: o.reviewerRunnerSettings.TaskTimeout,
	})
}

func (o *Orchestrator) recordReviewerRunnerFailure(ctx context.Context, runID, errorCode string) error {
	run, err := o.runs.GetByRunID(ctx, runID)
	if err != nil {
		return err
	}
	if isTerminalRunStatus(run.Status) {
		return nil
	}
	task, err := o.tasks.GetByTaskID(ctx, run.TaskID)
	if err != nil {
		return err
	}
	message := strings.TrimSpace(errorCode)
	if message == "" {
		message = "agent runner execution failed"
	}
	completedAt := o.now()
	applied, err := o.runs.UpdateFailedIfInFlight(ctx, run.RunID, message, completedAt)
	if err != nil {
		return err
	}
	if !applied {
		// 并发方已把 run 终态化（结果回传通道先到）：失败回写到此为止，
		// 不推进任务，防止把已推进的任务回拉。
		return nil
	}
	// 当前 run 栅栏：run 已不是任务的当前 run（run2 已接管/重派）时，run1
	// 的迟到失败只终态化 run1 自身，绝不推进 task——否则会把 run2 正在跑
	// 的任务推入 retry_pending 并清掉当前执行权。
	if task.CurrentRunID == nil || *task.CurrentRunID != run.RunID {
		return nil
	}
	// 推进本身也带 current_run_id 条件：读 task 与本条 UPDATE 之间仍可能
	// 发生接管，条件写把该窗口一同关闭；0 行即接管已发生，静默收敛。
	previous := task.Status
	if err := o.stateMachine.Transition(task, o.policy.NextFailureStatus(task)); err != nil {
		return err
	}
	o.policy.Apply(task, previous, task.Status, o.now())
	advanced, err := o.tasks.AdvanceGuardedForCurrentRun(ctx, task, previous, run.RunID)
	if err != nil {
		return err
	}
	if !advanced {
		return nil
	}
	// 通知物化先于错误信息回写:推进已把停靠状态落库,LastError 回写遇瞬态
	// 故障提前返回时 run 已终态、后续对账被吸收,错过即永失(与
	// recordDispatchFailure 的吞错照发对齐)。
	o.materializeTesterStopIMEffect(ctx, task, message)
	task.LastError = message
	if _, err := o.tasks.UpdateLastErrorIfStatus(ctx, task.TaskID, task.Status, message); err != nil {
		return err
	}
	return nil
}

// buildReviewerWorkspaceRequest 组装 reviewer 工作区请求并按任务解析注册仓行
// (reviewer 只读克隆,URL 同样由仓行派生、凭据取同一行的每仓 Token)。
func buildReviewerWorkspaceRequest(ctx context.Context, task *model.Task, run *model.Run, settings ReviewerRunnerSettings) (workspace.ReviewerRequest, error) {
	repoRow, err := settings.RegisteredRepositoryFor(ctx, task)
	if err != nil {
		return workspace.ReviewerRequest{}, err
	}
	cloneURL := repoRow.CloneURL()
	if cloneURL == "" {
		return workspace.ReviewerRequest{}, fmt.Errorf("registered repository %s cannot derive clone url: base_url or path is empty", boundRepositoryID(task))
	}
	pkg, err := workers.BuildRunnerTaskPackage(task, run)
	if err != nil {
		return workspace.ReviewerRequest{}, err
	}
	// 仓行供给 URL 与凭据;repo spec 仅作「目标仓必须是唯一可读仓」的结构校验。
	if _, err := reviewerRepository(pkg.Repos, task.TargetRepo); err != nil {
		return workspace.ReviewerRequest{}, err
	}
	mergeRequest, _ := pkg.Context["merge_request"].(map[string]any)
	request := workspace.ReviewerRequest{
		TaskID: task.TaskID, RunID: run.RunID,
		Project: task.TargetRepo, RepositoryURL: cloneURL,
		SourceBranch: stringContextField(mergeRequest, "source_branch"),
		TargetBranch: stringContextField(mergeRequest, "target_branch"),
		HeadSHA:      stringContextField(mergeRequest, "head_sha"), BaseSHA: stringContextField(mergeRequest, "base_sha"),
		RequestedReviewMode: stringContextField(mergeRequest, "review_mode"), Credential: gitCredentialFrom(repoRow),
	}
	if request.RequestedReviewMode == "" {
		request.RequestedReviewMode = workspace.ReviewModeFull
	}
	return request, nil
}

func reviewerRepository(repos []agent.RepoSpec, targetRepo string) (agent.RepoSpec, error) {
	targetName := path.Base(strings.TrimSpace(targetRepo))
	for index := range repos {
		repo := &repos[index]
		if repo.Access != "readable" {
			continue
		}
		if repo.Name == targetName {
			return *repo, nil
		}
	}
	return agent.RepoSpec{}, errors.New("reviewer task has no readable repository matching target repository")
}

func stringContextField(object map[string]any, name string) string {
	value, _ := object[name].(string)
	return strings.TrimSpace(value)
}
