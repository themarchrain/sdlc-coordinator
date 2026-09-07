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
	settings  ReviewerRunnerSettings
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
) *DeveloperRunnerCoordinator {
	return &DeveloperRunnerCoordinator{
		tasks: tasks, runs: runs, preparer: preparer, finalizer: finalizer, submitter: submitter,
		core: NewAgentRunnerCoordinator(tasks, runs, executions, runnerClient), settings: settings,
	}
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
	executionRequest := runner.CreateExecutionRequest{
		ExecutionID: request.Run.RunID, TaskID: request.Task.TaskID, Role: request.Task.Role,
		WorkspacePath: runRoot, PromptFile: agent.PromptFileName, SchemaFile: agent.ResultSchemaFileName,
		TimeoutSeconds: int64(request.Timeout / time.Second), Provider: request.Provider,
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
	if observation.Result == nil {
		return result, nil
	}
	runRecord, err := c.runs.GetByRunID(ctx, runID)
	if err != nil {
		return nil, err
	}
	task, err := c.tasks.GetByTaskID(ctx, runRecord.TaskID)
	if err != nil {
		return nil, err
	}
	agentContract := results.NewDeveloperAgentContract()
	agentInput, err := agentContract.DecodeStrict(observation.Result.StructuredOutput)
	if err != nil {
		return result, newInvalidRunnerResultError(fmt.Errorf("decode developer runner result: %w", err))
	}
	if err := agentContract.Validate(task, runRecord, agentInput); err != nil {
		return result, newInvalidRunnerResultError(fmt.Errorf("validate developer runner result: %w", err))
	}
	inputValue := agentInput.(*results.DeveloperAgentResultInput)
	var developerInput *results.DeveloperResultInput
	if inputValue.Outcome == "change_ready" || inputValue.Outcome == "rework_ready" {
		workspaceRequest, err := buildDeveloperWorkspaceRequest(ctx, task, runRecord, c.settings)
		if err != nil {
			return result, fmt.Errorf("rebuild trusted developer workspace metadata: %w", err)
		}
		delivery, err := c.finalizer.Finalize(ctx, workspace.DeveloperFinalizeRequest{
			WorkspaceRoot: c.settings.WorkspaceRoot, TaskID: task.TaskID, RunID: runRecord.RunID,
			Summary: inputValue.Summary, TaskType: task.Type, RepositoryURL: workspaceRequest.RepositoryURL,
			TargetBranch: workspaceRequest.TargetBranch, DeliveryBranch: workspaceRequest.DeliveryBranch,
			ExpectedInitialHeadSHA: workspaceRequest.ExpectedHeadSHA, Credential: workspaceRequest.Credential,
		})
		if err != nil {
			return result, newDeliveryRunnerResultError(fmt.Errorf("finalize developer git delivery: %w", err))
		}
		developerInput, err = agentContract.WithDelivery(agentInput, results.DeliveryEvidence{
			CommitSHA: delivery.CommitSHA, ChangedFiles: delivery.ChangedFiles, DiffSummary: delivery.DiffSummary,
		})
		if err != nil {
			return result, err
		}
	} else {
		developerInput, err = agentContract.WithoutDelivery(agentInput)
		if err != nil {
			return result, err
		}
	}
	contract := results.NewDeveloperContract()
	if err := contract.Validate(task, runRecord, developerInput); err != nil {
		return result, newInvalidRunnerResultError(fmt.Errorf("validate finalized developer result: %w", err))
	}
	canonical, err := contract.Normalize(task, runRecord, developerInput)
	if err != nil {
		return result, newInvalidRunnerResultError(fmt.Errorf("normalize finalized developer result: %w", err))
	}
	submission, err := c.submitter.SubmitResult(ctx, SubmitResultRequest{
		Source: "agent_runner", ToolName: contract.ToolName(), RawPayload: append([]byte(nil), observation.Result.StructuredOutput...), Canonical: canonical,
	})
	if err != nil {
		return result, fmt.Errorf("submit developer runner result: %w", err)
	}
	result.Submission = submission
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

// buildDeveloperWorkspaceRequest 组装 developer 工作区请求并按任务解析注册仓凭据;
// 解析失败(未绑定/查询失败)直接返回错误,由派发失败或收尾错误路径浮出。
func buildDeveloperWorkspaceRequest(ctx context.Context, task *model.Task, run *model.Run, settings ReviewerRunnerSettings) (workspace.DeveloperRequest, error) {
	credential, err := settings.CredentialFor(ctx, task)
	if err != nil {
		return workspace.DeveloperRequest{}, err
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
			WorkspaceRoot: settings.WorkspaceRoot, RepositoryURL: repo.URL, TargetBranch: repo.Branch,
			DeliveryBranch: model.DeliveryBranch(task), Credential: credential,
		},
	}
	return normalizeDeveloperWorkspaceRequest(request)
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
