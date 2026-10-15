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
	CommitDelivery(context.Context, workspace.DeveloperCommitRequest) (*workspace.DeveloperCommitResult, error)
	PushDelivery(context.Context, workspace.DeveloperPushRequest) (*workspace.DeveloperDelivery, error)
}

type DeveloperRunnerCoordinator struct {
	tasks       TaskRepository
	runs        RunRepository
	preparer    DeveloperWorkspacePreparer
	finalizer   DeveloperGitFinalizer
	bundleSink  TaskBundlePublisher
	checkpoints DeveloperDeliveryCheckpoints
	submitter   ReviewerResultSubmitter
	core        *AgentRunnerCoordinator
	channel     *ResultChannelCoordinator
	settings    ReviewerRunnerSettings
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

// SetDeveloperDeliveryCheckpoints 在装配期接入交付检查点端口(租户库实现),
// Start 与结果通道共用同一实现;必须在首次派发前完成。
func (c *DeveloperRunnerCoordinator) SetDeveloperDeliveryCheckpoints(checkpoints DeveloperDeliveryCheckpoints) {
	c.checkpoints = checkpoints
	c.channel.SetDeveloperDeliveryCheckpoints(checkpoints)
}

// SetTaskBundlePublisher 在装配期接入任务包投递端口（远端工作区形态注入；
// 本机形态不注入 = 协调器直接写本地路径）。独立文件系统下任务包必须经受控
// 通道到达 run 所在 Runner，本地写入在那边不可见。
func (c *DeveloperRunnerCoordinator) SetTaskBundlePublisher(publisher TaskBundlePublisher) {
	c.bundleSink = publisher
}

// SetDeliveryExecutionRight 在装配期接入交付执行权端口：文本通道的交付
// （reconcile → 通道内 commit/push）与 MCP 通道共用同一 run 级执行权。
func (c *DeveloperRunnerCoordinator) SetDeliveryExecutionRight(leases WorkItemLeasePort) {
	c.channel.SetDeliveryExecutionRight(leases)
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
	// 工作区路径以 run 行已钉住的 Runner 侧路径为准（独立文件系统下协调器
	// 本地路径在 Runner 那边不存在）；未钉住（首次派发）时用协调器本地
	// 计算值占位，首次准备成功后由 prepare 返回值钉回。
	runRoot := strings.TrimSpace(request.Run.RunnerWorkspacePath)
	if runRoot == "" {
		runRoot = filepath.Join(workspaceRequest.WorkspaceRoot, "runs", request.Run.RunID)
	}
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
		// 准备之前先解析并钉住 run 归属端点：准备、投递、执行、commit/push、
		// 清理全链按同一端点寻址。
		if _, err := c.core.resolveAndPinEndpoint(ctx, request.Task, request.Run); err != nil {
			return nil, err
		}
		manifest, err := c.preparer.Prepare(ctx, workspaceRequest)
		if err != nil {
			return nil, fmt.Errorf("prepare developer workspace: %w", err)
		}
		if manifest == nil || manifest.Status != workspace.ManifestStatusReady || manifest.TaskID != request.Task.TaskID || manifest.RunID != request.Run.RunID || manifest.Role != "DeveloperWorker" {
			return nil, errors.New("developer workspace manifest is not ready for current task and run")
		}
		if err := c.checkpoints.RecordBaseline(ctx, &model.DeveloperDeliveryCheckpoint{
			TaskID: request.Task.TaskID, RunID: request.Run.RunID, TaskType: request.Task.Type,
			RepositoryURL: workspaceRequest.RepositoryURL, DeliveryBranch: workspaceRequest.DeliveryBranch,
			BaselineSHA: strings.ToLower(strings.TrimSpace(manifest.Repository.InitialHeadSHA)),
		}); err != nil {
			return nil, fmt.Errorf("record developer delivery baseline: %w", err)
		}
		bundle, err := workers.BuildDeveloperRunnerTaskBundle(request.Task, request.Run)
		if err != nil {
			return nil, fmt.Errorf("build developer task bundle: %w", err)
		}
		workspacePath, err := c.deliverTaskBundle(ctx, request.Task, request.Run, bundle, runRoot)
		if err != nil {
			return nil, err
		}
		// Runner 返回的工作区路径是执行请求的唯一权威来源；钉回 run 行供
		// 崩溃恢复路径按同一路径重建请求（请求哈希覆盖该字段，值不稳会
		// 让重放对不上远端既有执行）。
		executionRequest.WorkspacePath = workspacePath
		if err := c.runs.PinRunnerWorkspacePathIfEmpty(ctx, request.Run.RunID, workspacePath); err != nil {
			return nil, fmt.Errorf("pin developer runner workspace path: %w", err)
		}
		request.Run.RunnerWorkspacePath = workspacePath
	}
	pinRunnerSelectionOnRun(ctx, c.runs, request.Run, providerConfig.Name, providerConfig.Effort, taskTimeout)
	return c.core.Start(ctx, request.Task, request.Run, executionRequest)
}

// deliverTaskBundle 把任务包送到 Agent 读取的位置：远端形态（装配了投递
// 端口）经受控通道投递并返回 Runner 侧路径；本机形态直接写本地路径并沿用
// 协调器计算值。两条路径共享同一渲染器，落盘内容逐字节一致。
func (c *DeveloperRunnerCoordinator) deliverTaskBundle(ctx context.Context, task *model.Task, run *model.Run, bundle agent.TaskBundle, localRoot string) (string, error) {
	if c.bundleSink == nil {
		if err := agent.WriteTaskBundle(localRoot, bundle); err != nil {
			return "", fmt.Errorf("write developer task bundle: %w", err)
		}
		return localRoot, nil
	}
	files, err := agent.RenderTaskBundle(bundle)
	if err != nil {
		return "", fmt.Errorf("render developer task bundle: %w", err)
	}
	workspacePath, err := c.bundleSink.PublishTaskBundle(ctx, task.TaskID, run.RunID, files)
	if err != nil {
		return "", fmt.Errorf("publish developer task bundle: %w", err)
	}
	if strings.TrimSpace(workspacePath) == "" {
		return "", errors.New("runner did not report the run workspace path for the task bundle")
	}
	return workspacePath, nil
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
	if err := refuseDeliveryMarker(observation.Result); err != nil {
		return result, err
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
	if c == nil || c.tasks == nil || c.runs == nil || c.preparer == nil || c.finalizer == nil || c.checkpoints == nil || c.submitter == nil || c.core == nil {
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
