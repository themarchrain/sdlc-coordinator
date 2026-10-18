package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"sdlc-coordinator/internal/agent"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/workers"
	"sdlc-coordinator/internal/workspace"
)

type ReviewerWorkspacePreparer interface {
	Prepare(context.Context, workspace.ReviewerRequest) (*workspace.Manifest, error)
}

type ReviewerRunnerClient interface {
	Create(context.Context, runner.CreateExecutionRequest) (*runner.Execution, error)
	Get(context.Context, string) (*runner.Execution, error)
	Events(context.Context, string, int64) ([]runner.Event, error)
	Result(context.Context, string) (*runner.Result, error)
	Cancel(context.Context, string) (*runner.Execution, error)
}

type AgentExecutionProjectionRepository interface {
	Create(context.Context, *model.AgentExecution) error
	Update(context.Context, *model.AgentExecution) error
	TouchLastObservedAt(context.Context, string, time.Time) error
	SettleUnacceptedIfDispatching(context.Context, string, time.Time) (bool, error)
	GetByRunID(context.Context, string) (*model.AgentExecution, error)
	UpsertEvents(context.Context, []model.AgentExecutionEvent) error
	// UpdateRunnerEndpointIfUnchanged 以旧端点值为条件推进端点（fallback
	// 轮换前先落新目标，防接管方回旧端点重发同 run）。
	UpdateRunnerEndpointIfUnchanged(ctx context.Context, runID, fromEndpoint, toEndpoint string) error
}

type ReviewerResultSubmitter interface {
	SubmitResult(context.Context, SubmitResultRequest) (*SubmitResultResponse, error)
}

type ReviewerResultSubmitterFunc func(context.Context, SubmitResultRequest) (*SubmitResultResponse, error)

func (submit ReviewerResultSubmitterFunc) SubmitResult(ctx context.Context, request SubmitResultRequest) (*SubmitResultResponse, error) {
	return submit(ctx, request)
}

type ReviewerRunnerCoordinator struct {
	tasks      TaskRepository
	runs       RunRepository
	preparer   ReviewerWorkspacePreparer
	submitter  ReviewerResultSubmitter
	core       *AgentRunnerCoordinator
	channel    *ResultChannelCoordinator
	bundleSink TaskBundlePublisher
	// runnerProviderLookup 是组织段执行选择（CLI 与推理强度）的动态来源
	// （派发时现取）；nil 或字段为空串 = 沿用系统默认。
	runnerProviderLookup func(context.Context) RunnerSelection
}

type ReviewerRunnerStartRequest struct {
	Task      *model.Task
	Run       *model.Run
	Workspace workspace.ReviewerRequest
	Provider  runner.ProviderConfig
	Timeout   time.Duration
}

type ReviewerRunnerReconcileResult struct {
	Execution  *model.AgentExecution
	Events     []runner.Event
	Submission *SubmitResultResponse
	// Absorbed marks observations on a run that already reached a terminal
	// status (typically finalized via the MCP channel): the execution
	// projection still updates, but the text result is never submitted and
	// failures never escalate.
	Absorbed     bool
	AbsorbReason string
}

func NewReviewerRunnerCoordinator(
	tasks TaskRepository,
	runs RunRepository,
	executions AgentExecutionProjectionRepository,
	preparer ReviewerWorkspacePreparer,
	runnerClient ReviewerRunnerClient,
	submitter ReviewerResultSubmitter,
	opts ...RunnerCoordinatorOption,
) *ReviewerRunnerCoordinator {
	core := NewAgentRunnerCoordinator(tasks, runs, executions, runnerClient)
	if options := newRunnerCoordinatorOptions(opts); options.router != nil {
		core.setTenantRunnerRouter(options.router)
	}
	return &ReviewerRunnerCoordinator{
		tasks: tasks, runs: runs, preparer: preparer, submitter: submitter,
		core:    core,
		channel: NewResultChannelCoordinator(nil, submitter, ReviewerRunnerSettings{}, "agent_runner"),
	}
}

// SetTenantRunnerRouter 在装配期接入租户池路由（NewReady 返回前才发生派发，
// 构造后接入安全；nil 保留单端点直连）。
func (c *ReviewerRunnerCoordinator) SetTenantRunnerRouter(router TenantRunnerRouter) {
	c.core.setTenantRunnerRouter(router)
}

func (c *ReviewerRunnerCoordinator) SetTaskBundlePublisher(publisher TaskBundlePublisher) {
	c.bundleSink = publisher
}

// SetRunnerProviderLookup 在装配期接入组织段执行选择（派发时现取，
// 保存即热生效）；lookup 为 nil 或字段返回空串时保留系统默认值。
func (c *ReviewerRunnerCoordinator) SetRunnerProviderLookup(lookup func(context.Context) RunnerSelection) {
	c.runnerProviderLookup = lookup
}

func (c *ReviewerRunnerCoordinator) Start(ctx context.Context, request ReviewerRunnerStartRequest) (*model.AgentExecution, error) {
	if err := c.validateDependencies(); err != nil {
		return nil, err
	}
	if c.bundleSink == nil {
		return nil, errors.New("runner task bundle publisher is required")
	}
	if err := validateReviewerRunnerTask(request.Task, request.Run); err != nil {
		return nil, err
	}
	workspaceRequest, err := normalizeReviewerWorkspaceRequest(request)
	if err != nil {
		return nil, err
	}
	runRoot := strings.TrimSpace(request.Run.RunnerWorkspacePath)
	// 组织段执行选择覆盖请求 Provider 的 Name/Effort 与派发超时（值拷贝，
	// 其余字段保持系统值）；全部派发路径（派发/投影重建）都经本入口汇聚。
	// run 已钉住取值时（恢复/重建路径）钉住值优先，配置热切换不改变在途
	// run 的身份（请求哈希覆盖三项）。
	providerConfig, taskTimeout := resolveRunnerExecution(ctx, c.runnerProviderLookup, request.Provider, request.Timeout, request.Run)
	executionRequest := runner.CreateExecutionRequest{
		ExecutionID:    request.Run.RunID,
		TaskID:         request.Task.TaskID,
		Role:           request.Task.Role,
		WorkspacePath:  runRoot,
		PromptFile:     agent.PromptFileName,
		SchemaFile:     agent.ResultSchemaFileName,
		TimeoutSeconds: int64(taskTimeout / time.Second),
		Provider:       providerConfig,
	}
	existing, found, err := c.core.Existing(ctx, request.Run.RunID, executionRequest)
	if err != nil {
		return nil, err
	}
	if found && existing.Status != model.AgentExecutionStatusDispatching {
		return existing, nil
	}
	// 派发意图尚未收敛（dispatching）：工作区与任务包在上一次派发时已就绪，
	// 跳过重准备直接进 Start 按原端点对账，避免覆盖在途 Agent 的工作区。
	if !found {
		// 准备之前先解析并钉住 run 归属端点：准备、投递、执行、清理全链
		// 按同一端点寻址。
		if _, err := c.core.resolveAndPinEndpoint(ctx, request.Task, request.Run); err != nil {
			return nil, err
		}
		manifest, err := c.preparer.Prepare(ctx, workspaceRequest)
		if err != nil {
			return nil, fmt.Errorf("prepare reviewer workspace: %w", err)
		}
		if manifest == nil || manifest.Status != workspace.ManifestStatusReady || manifest.TaskID != request.Task.TaskID || manifest.RunID != request.Run.RunID || manifest.Role != "ReviewerWorker" {
			return nil, errors.New("reviewer workspace manifest is not ready for current task and run")
		}
		bundle, err := workers.BuildReviewerRunnerTaskBundle(request.Task, request.Run)
		if err != nil {
			return nil, fmt.Errorf("build reviewer task bundle: %w", err)
		}
		workspacePath, err := c.deliverTaskBundle(ctx, request.Task, request.Run, bundle)
		if err != nil {
			return nil, err
		}
		executionRequest.WorkspacePath = workspacePath
		if err := c.runs.PinRunnerWorkspacePathIfEmpty(ctx, request.Run.RunID, workspacePath); err != nil {
			return nil, fmt.Errorf("pin reviewer runner workspace path: %w", err)
		}
		request.Run.RunnerWorkspacePath = workspacePath
	}

	pinRunnerSelectionOnRun(ctx, c.runs, request.Run, providerConfig.Name, providerConfig.Effort, taskTimeout)
	return c.core.Start(ctx, request.Task, request.Run, executionRequest)
}

func (c *ReviewerRunnerCoordinator) Reconcile(ctx context.Context, runID string) (*ReviewerRunnerReconcileResult, error) {
	if err := c.validateDependencies(); err != nil {
		return nil, err
	}
	observation, err := c.core.Observe(ctx, runID)
	if err != nil {
		return nil, err
	}
	result := &ReviewerRunnerReconcileResult{Execution: observation.Execution, Events: observation.Events}
	runRecord, err := c.runs.GetByRunID(ctx, runID)
	if err != nil {
		return nil, err
	}
	if runRecord.Status != model.RunStatusRunning {
		// 双通道优先级：run 已终态（MCP 首提终态化或先行文本终态）时，文本
		// 结果只记日志不采纳，执行失败也不升级——BlockRunAndRequireHuman 等
		// 收敛路径不得破坏已终态的 run。
		result.Absorbed = true
		result.AbsorbReason = "run_status_" + string(runRecord.Status)
		return result, nil
	}
	if observation.Result == nil {
		return result, nil
	}
	if err := refuseDeliveryMarker(observation.Result); err != nil {
		return result, err
	}
	task, err := c.tasks.GetByTaskID(ctx, runRecord.TaskID)
	if err != nil {
		return nil, err
	}
	outcome, err := c.channel.SubmitAgentResult(ctx, task, runRecord, results.SubmitReviewerResultTool, observation.Result.StructuredOutput)
	if err != nil {
		return result, err
	}
	switch outcome.Decision {
	case ChannelRejected:
		return result, newInvalidRunnerResultError(errors.New(outcome.Problems[0].Problem))
	case ChannelFailed:
		return result, fmt.Errorf("submit reviewer runner result: %w", outcome.Err)
	}
	result.Submission = outcome.Response
	return result, nil
}

func (c *ReviewerRunnerCoordinator) validateDependencies() error {
	if c == nil || c.tasks == nil || c.runs == nil || c.preparer == nil || c.submitter == nil || c.core == nil {
		return errors.New("reviewer runner coordinator dependencies are required")
	}
	return nil
}

func validateReviewerRunnerTask(task *model.Task, run *model.Run) error {
	if task == nil || run == nil {
		return errors.New("reviewer task and run are required")
	}
	if task.Role != "ReviewerWorker" || run.WorkerRole != "ReviewerWorker" {
		return errors.New("reviewer runner requires ReviewerWorker task and run")
	}
	if strings.TrimSpace(task.TaskID) == "" || strings.TrimSpace(run.RunID) == "" || run.TaskID != task.TaskID {
		return errors.New("reviewer task and run identity is inconsistent")
	}
	if task.CurrentRunID == nil || *task.CurrentRunID != run.RunID {
		return errors.New("reviewer run is not current for task")
	}
	return nil
}

func normalizeReviewerWorkspaceRequest(request ReviewerRunnerStartRequest) (workspace.ReviewerRequest, error) {
	result := request.Workspace
	for name, values := range map[string][2]string{
		"workspace task id": {result.TaskID, request.Task.TaskID},
		"workspace run id":  {result.RunID, request.Run.RunID},
		"workspace project": {result.Project, request.Task.TargetRepo},
	} {
		actual, expected := values[0], values[1]
		if strings.TrimSpace(actual) != "" && actual != expected {
			return workspace.ReviewerRequest{}, fmt.Errorf("%s does not match current task/run", name)
		}
	}
	result.TaskID = request.Task.TaskID
	result.RunID = request.Run.RunID
	result.Project = request.Task.TargetRepo
	return result, nil
}

func validateStoredExecution(execution *model.AgentExecution, runID, requestHash string) error {
	if execution == nil || execution.RunID != runID || execution.ExecutionID != runID {
		return errors.New("stored agent execution identity does not match run")
	}
	if execution.RequestHash != requestHash {
		return errors.New("stored agent execution request conflicts with current request")
	}
	return nil
}

func validateRemoteExecution(execution *runner.Execution, executionID, requestHash string) error {
	if execution == nil || execution.ExecutionID != executionID {
		return errors.New("runner execution identity does not match run")
	}
	if execution.RequestHash != requestHash {
		return errors.New("runner execution request conflicts with current request")
	}
	return nil
}

func projectRunnerStatus(status runner.Status) (model.AgentExecutionStatus, error) {
	switch status {
	case runner.StatusAccepted:
		return model.AgentExecutionStatusAccepted, nil
	case runner.StatusStarting:
		return model.AgentExecutionStatusStarting, nil
	case runner.StatusRunning:
		return model.AgentExecutionStatusRunning, nil
	case runner.StatusSucceeded:
		return model.AgentExecutionStatusSucceeded, nil
	case runner.StatusFailed:
		return model.AgentExecutionStatusFailed, nil
	case runner.StatusCancelled:
		return model.AgentExecutionStatusCancelled, nil
	case runner.StatusInterrupted:
		return model.AgentExecutionStatusInterrupted, nil
	default:
		return "", fmt.Errorf("unsupported runner execution status %q", status)
	}
}

func (c *ReviewerRunnerCoordinator) deliverTaskBundle(ctx context.Context, task *model.Task, run *model.Run, bundle agent.TaskBundle) (string, error) {
	files, err := agent.RenderTaskBundle(bundle)
	if err != nil {
		return "", fmt.Errorf("render task bundle: %w", err)
	}
	workspacePath, err := c.bundleSink.PublishTaskBundle(ctx, task.TaskID, run.RunID, files)
	if err != nil {
		return "", fmt.Errorf("publish task bundle: %w", err)
	}
	if strings.TrimSpace(workspacePath) == "" {
		return "", errors.New("runner did not report the run workspace path for the task bundle")
	}
	return workspacePath, nil
}
