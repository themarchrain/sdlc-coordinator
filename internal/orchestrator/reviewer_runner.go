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
	GetByRunID(context.Context, string) (*model.AgentExecution, error)
	UpsertEvents(context.Context, []model.AgentExecutionEvent) error
}

type ReviewerResultSubmitter interface {
	SubmitResult(context.Context, SubmitResultRequest) (*SubmitResultResponse, error)
}

type ReviewerResultSubmitterFunc func(context.Context, SubmitResultRequest) (*SubmitResultResponse, error)

func (submit ReviewerResultSubmitterFunc) SubmitResult(ctx context.Context, request SubmitResultRequest) (*SubmitResultResponse, error) {
	return submit(ctx, request)
}

type ReviewerRunnerCoordinator struct {
	tasks     TaskRepository
	runs      RunRepository
	preparer  ReviewerWorkspacePreparer
	submitter ReviewerResultSubmitter
	core      *AgentRunnerCoordinator
	channel   *ResultChannelCoordinator
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
}

func NewReviewerRunnerCoordinator(
	tasks TaskRepository,
	runs RunRepository,
	executions AgentExecutionProjectionRepository,
	preparer ReviewerWorkspacePreparer,
	runnerClient ReviewerRunnerClient,
	submitter ReviewerResultSubmitter,
) *ReviewerRunnerCoordinator {
	return &ReviewerRunnerCoordinator{
		tasks: tasks, runs: runs, preparer: preparer, submitter: submitter,
		core:    NewAgentRunnerCoordinator(tasks, runs, executions, runnerClient),
		channel: NewResultChannelCoordinator(nil, submitter, ReviewerRunnerSettings{}, "agent_runner"),
	}
}

func (c *ReviewerRunnerCoordinator) Start(ctx context.Context, request ReviewerRunnerStartRequest) (*model.AgentExecution, error) {
	if err := c.validateDependencies(); err != nil {
		return nil, err
	}
	if err := validateReviewerRunnerTask(request.Task, request.Run); err != nil {
		return nil, err
	}
	workspaceRequest, err := normalizeReviewerWorkspaceRequest(request)
	if err != nil {
		return nil, err
	}
	runRoot := filepath.Join(workspaceRequest.WorkspaceRoot, "runs", request.Run.RunID)
	executionRequest := runner.CreateExecutionRequest{
		ExecutionID:    request.Run.RunID,
		TaskID:         request.Task.TaskID,
		Role:           request.Task.Role,
		WorkspacePath:  runRoot,
		PromptFile:     agent.PromptFileName,
		SchemaFile:     agent.ResultSchemaFileName,
		TimeoutSeconds: int64(request.Timeout / time.Second),
		Provider:       request.Provider,
	}
	if existing, found, err := c.core.Existing(ctx, request.Run.RunID, executionRequest); err != nil {
		return nil, err
	} else if found {
		return existing, nil
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
	if err := agent.WriteTaskBundle(runRoot, bundle); err != nil {
		return nil, fmt.Errorf("write reviewer task bundle: %w", err)
	}

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
