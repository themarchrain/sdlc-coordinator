package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"sdlc-coordinator/internal/agent"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/workers"
	"sdlc-coordinator/internal/workspace"
)

// TesterRunnerCoordinator 把验证任务送进 runner 池:工作区复用 reviewer 的
// 只读检出(固定到任务上下文冻结的头部 SHA),回传走 MCP 的 submit_tester_result
// 工具,红绿证据由 TesterContract 校验落库。
type TesterRunnerCoordinator struct {
	tasks     TaskRepository
	runs      RunRepository
	preparer  ReviewerWorkspacePreparer
	submitter ReviewerResultSubmitter
	core      *AgentRunnerCoordinator
	channel   *ResultChannelCoordinator
	// runnerProviderLookup 是组织段执行选择（CLI 与推理强度）的动态来源
	// （派发时现取）；nil 或字段为空串 = 沿用系统默认。
	runnerProviderLookup func(context.Context) RunnerSelection
}

type TesterRunnerStartRequest struct {
	Task      *model.Task
	Run       *model.Run
	Workspace workspace.ReviewerRequest
	Provider  runner.ProviderConfig
	Timeout   time.Duration
}

func NewTesterRunnerCoordinator(
	tasks TaskRepository,
	runs RunRepository,
	executions AgentExecutionProjectionRepository,
	preparer ReviewerWorkspacePreparer,
	runnerClient ReviewerRunnerClient,
	submitter ReviewerResultSubmitter,
	opts ...RunnerCoordinatorOption,
) *TesterRunnerCoordinator {
	core := NewAgentRunnerCoordinator(tasks, runs, executions, runnerClient)
	if options := newRunnerCoordinatorOptions(opts); options.router != nil {
		core.setTenantRunnerRouter(options.router)
	}
	return &TesterRunnerCoordinator{
		tasks: tasks, runs: runs, preparer: preparer, submitter: submitter,
		core:    core,
		channel: NewResultChannelCoordinator(nil, submitter, ReviewerRunnerSettings{}, "agent_runner"),
	}
}

// SetTenantRunnerRouter 在装配期接入租户池路由；nil 保留单端点直连。
func (c *TesterRunnerCoordinator) SetTenantRunnerRouter(router TenantRunnerRouter) {
	c.core.setTenantRunnerRouter(router)
}

// SetRunnerProviderLookup 在装配期接入组织段执行选择（派发时现取，
// 保存即热生效）；lookup 为 nil 或字段返回空串时保留系统默认值。
func (c *TesterRunnerCoordinator) SetRunnerProviderLookup(lookup func(context.Context) RunnerSelection) {
	c.runnerProviderLookup = lookup
}

func (c *TesterRunnerCoordinator) Start(ctx context.Context, request TesterRunnerStartRequest) (*model.AgentExecution, error) {
	if err := c.validateDependencies(); err != nil {
		return nil, err
	}
	if err := validateTesterRunnerTask(request.Task, request.Run); err != nil {
		return nil, err
	}
	workspaceRequest, err := normalizeTesterWorkspaceRequest(request)
	if err != nil {
		return nil, err
	}
	runRoot := filepath.Join(workspaceRequest.WorkspaceRoot, "runs", request.Run.RunID)
	// 组织段执行选择覆盖请求 Provider 的 Name/Effort 与派发超时；run 已钉住
	// 取值时钉住值优先，与 reviewer/developer 同一口径。
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
	if existing, found, err := c.core.Existing(ctx, request.Run.RunID, executionRequest); err != nil {
		return nil, err
	} else if found {
		return existing, nil
	}
	manifest, err := c.preparer.Prepare(ctx, workspaceRequest)
	if err != nil {
		return nil, fmt.Errorf("prepare tester workspace: %w", err)
	}
	if manifest == nil || manifest.Status != workspace.ManifestStatusReady || manifest.TaskID != request.Task.TaskID || manifest.RunID != request.Run.RunID || manifest.Role != "TesterWorker" {
		return nil, errors.New("tester workspace manifest is not ready for current task and run")
	}
	bundle, err := workers.BuildTesterRunnerTaskBundle(request.Task, request.Run)
	if err != nil {
		return nil, fmt.Errorf("build tester task bundle: %w", err)
	}
	if err := agent.WriteTaskBundle(runRoot, bundle); err != nil {
		return nil, fmt.Errorf("write tester task bundle: %w", err)
	}

	pinRunnerSelectionOnRun(ctx, c.runs, request.Run, providerConfig.Name, providerConfig.Effort, taskTimeout)
	return c.core.Start(ctx, request.Task, request.Run, executionRequest)
}

func (c *TesterRunnerCoordinator) Reconcile(ctx context.Context, runID string) (*ReviewerRunnerReconcileResult, error) {
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
		// 双通道优先级：run 已终态时文本结果不采纳、失败不升级。
		result.Absorbed = true
		result.AbsorbReason = "run_status_" + string(runRecord.Status)
		return result, nil
	}
	if observation.Result == nil {
		return result, nil
	}
	task, err := c.tasks.GetByTaskID(ctx, runRecord.TaskID)
	if err != nil {
		return nil, err
	}
	outcome, err := c.channel.SubmitAgentResult(ctx, task, runRecord, results.SubmitTesterResultTool, observation.Result.StructuredOutput)
	if err != nil {
		return result, err
	}
	switch outcome.Decision {
	case ChannelRejected:
		return result, newInvalidRunnerResultError(errors.New(outcome.Problems[0].Problem))
	case ChannelFailed:
		return result, fmt.Errorf("submit tester runner result: %w", outcome.Err)
	}
	result.Submission = outcome.Response
	return result, nil
}

func (c *TesterRunnerCoordinator) validateDependencies() error {
	if c == nil || c.tasks == nil || c.runs == nil || c.preparer == nil || c.submitter == nil || c.core == nil {
		return errors.New("tester runner coordinator dependencies are required")
	}
	return nil
}

func validateTesterRunnerTask(task *model.Task, run *model.Run) error {
	if task == nil || run == nil {
		return errors.New("tester task and run are required")
	}
	if task.Role != "TesterWorker" || run.WorkerRole != "TesterWorker" {
		return errors.New("tester runner requires TesterWorker task and run")
	}
	if strings.TrimSpace(task.TaskID) == "" || strings.TrimSpace(run.RunID) == "" || run.TaskID != task.TaskID {
		return errors.New("tester task and run identity is inconsistent")
	}
	if task.CurrentRunID == nil || *task.CurrentRunID != run.RunID {
		return errors.New("tester run is not current for task")
	}
	return nil
}

func normalizeTesterWorkspaceRequest(request TesterRunnerStartRequest) (workspace.ReviewerRequest, error) {
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
	result.WorkerRole = "TesterWorker"
	return result, nil
}
