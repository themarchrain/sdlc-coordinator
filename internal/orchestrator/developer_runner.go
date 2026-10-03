package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
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

type DeveloperWorkspacePreparer interface {
	Prepare(context.Context, workspace.DeveloperRequest) (*workspace.Manifest, error)
}

type DeveloperGitFinalizer interface {
	Finalize(context.Context, workspace.DeveloperFinalizeRequest) (*workspace.DeveloperDelivery, error)
}

type DeveloperRunnerCoordinator struct {
	tasks     TaskRepository
	runs      RunRepository
	preparer  DeveloperWorkspacePreparer
	finalizer DeveloperGitFinalizer
	submitter ReviewerResultSubmitter
	core      *AgentRunnerCoordinator
	channel   *ResultChannelCoordinator
	settings  ReviewerRunnerSettings
	// runnerProviderLookup 是组织段执行选择（CLI 与推理强度）的动态来源
	// （派发时现取）；nil 或字段为空串 = 沿用系统默认。
	runnerProviderLookup func(context.Context) RunnerSelection
}

type DeveloperRunnerStartRequest struct {
	Task      *model.Task
	Run       *model.Run
	Workspace workspace.DeveloperRequest
	Provider  runner.ProviderConfig
	Timeout   time.Duration
}

type DeveloperRunnerReconcileResult = ReviewerRunnerReconcileResult

func NewDeveloperRunnerCoordinator(
	tasks TaskRepository,
	runs RunRepository,
	executions AgentExecutionProjectionRepository,
	preparer DeveloperWorkspacePreparer,
	finalizer DeveloperGitFinalizer,
	runnerClient ReviewerRunnerClient,
	submitter ReviewerResultSubmitter,
	settings ReviewerRunnerSettings,
	opts ...RunnerCoordinatorOption,
) *DeveloperRunnerCoordinator {
	core := NewAgentRunnerCoordinator(tasks, runs, executions, runnerClient)
	if options := newRunnerCoordinatorOptions(opts); options.router != nil {
		core.setTenantRunnerRouter(options.router)
	}
	return &DeveloperRunnerCoordinator{
		tasks: tasks, runs: runs, preparer: preparer, finalizer: finalizer, submitter: submitter,
		core: core, settings: settings,
		channel: NewResultChannelCoordinator(finalizer, submitter, settings, "agent_runner"),
	}
}

// SetTenantRunnerRouter 在装配期接入租户池路由（NewReady 返回前才发生派发，
// 构造后接入安全；nil 保留单端点直连）。
func (c *DeveloperRunnerCoordinator) SetTenantRunnerRouter(router TenantRunnerRouter) {
	c.core.setTenantRunnerRouter(router)
}

// SetRunnerProviderLookup 在装配期接入组织段执行选择（派发时现取，
// 保存即热生效）；lookup 为 nil 或字段返回空串时保留系统默认值。
func (c *DeveloperRunnerCoordinator) SetRunnerProviderLookup(lookup func(context.Context) RunnerSelection) {
	c.runnerProviderLookup = lookup
}

func (c *DeveloperRunnerCoordinator) Start(ctx context.Context, request DeveloperRunnerStartRequest) (*model.AgentExecution, error) {
	if err := c.validateDependencies(); err != nil {
		return nil, err
	}
	if err := validateDeveloperRunnerTask(request.Task, request.Run); err != nil {
		return nil, err
	}
	workspaceRequest, err := normalizeDeveloperWorkspaceRequest(request)
	if err != nil {
		return nil, err
	}
	runRoot := filepath.Join(workspaceRequest.WorkspaceRoot, "runs", request.Run.RunID)
	// 组织段执行选择覆盖请求 Provider 的 Name/Effort 与派发超时（值拷贝，
	// 其余字段保持系统值）；全部派发路径（派发/投影重建）都经本入口汇聚。
	// run 已钉住取值时（恢复/重建路径）钉住值优先，配置热切换不改变在途
	// run 的身份（请求哈希覆盖三项）。
	providerConfig, taskTimeout := resolveRunnerExecution(ctx, c.runnerProviderLookup, request.Provider, request.Timeout, request.Run)
	executionRequest := runner.CreateExecutionRequest{
		ExecutionID: request.Run.RunID, TaskID: request.Task.TaskID, Role: request.Task.Role,
		WorkspacePath: runRoot, PromptFile: agent.PromptFileName, SchemaFile: agent.ResultSchemaFileName,
		TimeoutSeconds: int64(taskTimeout / time.Second), Provider: providerConfig,
	}
	if existing, found, err := c.core.Existing(ctx, request.Run.RunID, executionRequest); err != nil {
		return nil, err
	} else if found {
		return existing, nil
	}
	manifest, err := c.preparer.Prepare(ctx, workspaceRequest)
	if err != nil {
		return nil, fmt.Errorf("prepare developer workspace: %w", err)
	}
	if manifest == nil || manifest.Status != workspace.ManifestStatusReady || manifest.TaskID != request.Task.TaskID || manifest.RunID != request.Run.RunID || manifest.Role != "DeveloperWorker" {
		return nil, errors.New("developer workspace manifest is not ready for current task and run")
	}
	bundle, err := workers.BuildDeveloperRunnerTaskBundle(request.Task, request.Run)
	if err != nil {
		return nil, fmt.Errorf("build developer task bundle: %w", err)
	}
	if err := agent.WriteTaskBundle(runRoot, bundle); err != nil {
		return nil, fmt.Errorf("write developer task bundle: %w", err)
	}
	pinRunnerSelectionOnRun(ctx, c.runs, request.Run, providerConfig.Name, providerConfig.Effort, taskTimeout)
	return c.core.Start(ctx, request.Task, request.Run, executionRequest)
}

func (c *DeveloperRunnerCoordinator) Reconcile(ctx context.Context, runID string) (*DeveloperRunnerReconcileResult, error) {
	if err := c.validateDependencies(); err != nil {
		return nil, err
	}
	observation, err := c.core.Observe(ctx, runID)
	if err != nil {
		return nil, err
	}
	result := &DeveloperRunnerReconcileResult{Execution: observation.Execution, Events: observation.Events}
	runRecord, err := c.runs.GetByRunID(ctx, runID)
	if err != nil {
		return nil, err
	}
	if runRecord.Status != model.RunStatusRunning {
		// 双通道优先级：run 已终态（MCP 首提终态化或先行文本终态）时，文本
		// 结果只记日志不采纳，执行失败也不升级。
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
	outcome, err := c.channel.SubmitAgentResult(ctx, task, runRecord, results.SubmitDeveloperResultTool, observation.Result.StructuredOutput)
	if err != nil {
		return result, err
	}
	switch outcome.Decision {
	case ChannelRejected:
		switch outcome.Stage {
		case ChannelStageFinalize:
			return result, newDeliveryRunnerResultError(errors.New(outcome.Problems[0].Problem))
		default:
			return result, newInvalidRunnerResultError(errors.New(outcome.Problems[0].Problem))
		}
	case ChannelFailed:
		return result, fmt.Errorf("submit developer runner result: %w", outcome.Err)
	}
	result.Submission = outcome.Response
	return result, nil
}

func (c *DeveloperRunnerCoordinator) validateDependencies() error {
	if c == nil || c.tasks == nil || c.runs == nil || c.preparer == nil || c.finalizer == nil || c.submitter == nil || c.core == nil {
		return errors.New("developer runner coordinator dependencies are required")
	}
	return nil
}

func validateDeveloperRunnerTask(task *model.Task, run *model.Run) error {
	if task == nil || run == nil {
		return errors.New("developer task and run are required")
	}
	if task.Role != "DeveloperWorker" || run.WorkerRole != "DeveloperWorker" {
		return errors.New("developer runner requires DeveloperWorker task and run")
	}
	if run.TaskID != task.TaskID || task.CurrentRunID == nil || *task.CurrentRunID != run.RunID {
		return errors.New("developer task and run identity is inconsistent")
	}
	return nil
}

func normalizeDeveloperWorkspaceRequest(request DeveloperRunnerStartRequest) (workspace.DeveloperRequest, error) {
	result := request.Workspace
	result.TaskID = request.Task.TaskID
	result.RunID = request.Run.RunID
	result.TaskType = request.Task.Type
	result.Project = request.Task.TargetRepo
	if strings.TrimSpace(result.DeliveryBranch) == "" {
		result.DeliveryBranch = model.DeliveryBranch(request.Task)
	}
	if request.Task.Type == model.TaskTypeReworkMergeRequest {
		var contextValue struct {
			MergeRequest struct {
				SourceBranch    string `json:"source_branch"`
				TargetBranch    string `json:"target_branch"`
				ExpectedHeadSHA string `json:"expected_head_sha"`
			} `json:"merge_request"`
		}
		if err := json.Unmarshal([]byte(request.Task.ContextJSON), &contextValue); err != nil {
			return workspace.DeveloperRequest{}, fmt.Errorf("decode developer rework context: %w", err)
		}
		if result.SourceBranch == "" {
			result.SourceBranch = contextValue.MergeRequest.SourceBranch
		}
		if result.TargetBranch == "" {
			result.TargetBranch = contextValue.MergeRequest.TargetBranch
		}
		if result.ExpectedHeadSHA == "" {
			result.ExpectedHeadSHA = contextValue.MergeRequest.ExpectedHeadSHA
		}
	}
	return result, nil
}

// buildDeveloperWorkspaceRequest 组装 developer 工作区请求并按任务解析注册仓行;
// clone/push URL 由仓行 CloneURL 派生(事件载荷 URL 仅作展示),凭据取同一行的每仓 Token。
// 解析失败(未绑定/查询失败/仓行缺身份字段)直接返回错误,由派发失败或收尾错误路径浮出。
func buildDeveloperWorkspaceRequest(ctx context.Context, task *model.Task, run *model.Run, settings ReviewerRunnerSettings) (workspace.DeveloperRequest, error) {
	repoRow, err := settings.RegisteredRepositoryFor(ctx, task)
	if err != nil {
		return workspace.DeveloperRequest{}, err
	}
	cloneURL := repoRow.CloneURL()
	if cloneURL == "" {
		return workspace.DeveloperRequest{}, fmt.Errorf("registered repository %s cannot derive clone url: base_url or path is empty", boundRepositoryID(task))
	}
	pkg, err := workers.BuildRunnerTaskPackage(task, run)
	if err != nil {
		return workspace.DeveloperRequest{}, err
	}
	repo, err := developerRepository(pkg.Repos, task.TargetRepo)
	if err != nil {
		return workspace.DeveloperRequest{}, err
	}
	request := DeveloperRunnerStartRequest{
		Task: task, Run: run,
		Workspace: workspace.DeveloperRequest{
			WorkspaceRoot: settings.WorkspaceRoot, RepositoryURL: cloneURL, TargetBranch: repo.Branch,
			DeliveryBranch: model.DeliveryBranch(task), Credential: gitCredentialFrom(repoRow),
			// 提交身份取注册仓行原值(可为空,prepare 侧降级内置身份),不
			// 信任 Agent 输出或事件载荷里的任何身份字段。
			GitAuthorName: repoRow.GitAuthorName, GitAuthorEmail: repoRow.GitAuthorEmail,
		},
	}
	return normalizeDeveloperWorkspaceRequest(request)
}

func boundRepositoryID(task *model.Task) string {
	if task == nil || task.RepositoryID == nil {
		return ""
	}
	return strings.TrimSpace(*task.RepositoryID)
}

func developerRepository(repos []agent.RepoSpec, targetRepo string) (agent.RepoSpec, error) {
	targetName := path.Base(strings.TrimSpace(targetRepo))
	var writable []agent.RepoSpec
	for _, repo := range repos {
		if repo.Access == "writable" {
			writable = append(writable, repo)
		}
	}
	if len(writable) != 1 {
		return agent.RepoSpec{}, fmt.Errorf("developer task requires exactly one writable repository, got %d", len(writable))
	}
	if targetName != "" && writable[0].Name != targetName {
		return agent.RepoSpec{}, errors.New("developer writable repository does not match target repository")
	}
	if strings.TrimSpace(writable[0].Branch) == "" {
		writable[0].Branch = "main"
	}
	return writable[0], nil
}
