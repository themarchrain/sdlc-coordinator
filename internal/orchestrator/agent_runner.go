package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store/repository"
)

type AgentRunnerObservation struct {
	Execution *model.AgentExecution
	Events    []runner.Event
	Result    *runner.Result
}

// RunTokenIssuer signs the per-run bearer tokens for the MCP result channel.
type RunTokenIssuer interface {
	IssueRunToken(taskID, runID, role string) (string, error)
}

// RunTokenIssuerFunc adapts a function into a RunTokenIssuer.
type RunTokenIssuerFunc func(taskID, runID, role string) (string, error)

func (f RunTokenIssuerFunc) IssueRunToken(taskID, runID, role string) (string, error) {
	return f(taskID, runID, role)
}

type AgentRunnerCoordinator struct {
	tasks      TaskRepository
	runs       RunRepository
	executions AgentExecutionProjectionRepository
	runner     ReviewerRunnerClient
	now        func() time.Time
	// ingress enables the MCP result channel on dispatch; nil keeps the
	// channel closed (text-only mode).
	ingressIssuer   RunTokenIssuer
	ingressEndpoint string
}

func NewAgentRunnerCoordinator(tasks TaskRepository, runs RunRepository, executions AgentExecutionProjectionRepository, runnerClient ReviewerRunnerClient) *AgentRunnerCoordinator {
	return &AgentRunnerCoordinator{tasks: tasks, runs: runs, executions: executions, runner: runnerClient, now: time.Now}
}

func (c *AgentRunnerCoordinator) Existing(ctx context.Context, runID string, request runner.CreateExecutionRequest) (*model.AgentExecution, bool, error) {
	if err := c.validateDependencies(); err != nil {
		return nil, false, err
	}
	expectedHash, err := runner.RequestHash(request)
	if err != nil {
		return nil, false, fmt.Errorf("build runner request identity: %w", err)
	}
	existing, err := c.executions.GetByRunID(ctx, runID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if err := validateStoredExecution(existing, runID, expectedHash); err != nil {
		return nil, false, err
	}
	return existing, true, nil
}

// setResultIngress wires the MCP result channel onto dispatch. Only real
// creates sign tokens: idempotent replays (Existing hit) keep the token the
// original dispatch carried.
func (c *AgentRunnerCoordinator) setResultIngress(issuer RunTokenIssuer, endpointURL string) {
	c.ingressIssuer = issuer
	c.ingressEndpoint = strings.TrimSpace(endpointURL)
}

func (c *AgentRunnerCoordinator) Start(ctx context.Context, task *model.Task, run *model.Run, request runner.CreateExecutionRequest) (*model.AgentExecution, error) {
	if err := c.validateDependencies(); err != nil {
		return nil, err
	}
	if err := validateAgentRunnerTask(task, run, request); err != nil {
		return nil, err
	}
	expectedHash, err := runner.RequestHash(request)
	if err != nil {
		return nil, fmt.Errorf("build runner request identity: %w", err)
	}
	existing, found, err := c.Existing(ctx, run.RunID, request)
	if err != nil {
		return nil, err
	}
	if found {
		return existing, nil
	}
	if c.ingressIssuer != nil && c.ingressEndpoint != "" {
		token, err := c.ingressIssuer.IssueRunToken(task.TaskID, run.RunID, task.Role)
		if err != nil {
			return nil, fmt.Errorf("issue run result token: %w", err)
		}
		request.ResultSubmission = &runner.ResultSubmissionChannel{EndpointURL: c.ingressEndpoint, BearerToken: token}
	}
	remote, createErr := c.runner.Create(ctx, request)
	if createErr != nil {
		remote, err = c.runner.Get(ctx, run.RunID)
		if err != nil {
			return nil, fmt.Errorf("create runner execution: %w", createErr)
		}
	}
	if err := validateRemoteExecution(remote, run.RunID, expectedHash); err != nil {
		return nil, err
	}
	projection, err := c.project(remote, request.Provider.Revision, 0, request.Provider.Model, request.Provider.Effort)
	if err != nil {
		return nil, err
	}
	if err := c.executions.Create(ctx, projection); err != nil {
		stored, lookupErr := c.executions.GetByRunID(ctx, run.RunID)
		if lookupErr == nil && validateStoredExecution(stored, run.RunID, expectedHash) == nil {
			return stored, nil
		}
		return nil, err
	}
	return projection, nil
}

func (c *AgentRunnerCoordinator) Observe(ctx context.Context, runID string) (*AgentRunnerObservation, error) {
	if err := c.validateDependencies(); err != nil {
		return nil, err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, errors.New("run id is required")
	}
	stored, err := c.executions.GetByRunID(ctx, runID)
	if err != nil {
		return nil, err
	}
	events, err := c.runner.Events(ctx, stored.ExecutionID, stored.LastEventSeq)
	if err != nil {
		return nil, fmt.Errorf("list runner execution events: %w", err)
	}
	lastEventSeq := stored.LastEventSeq
	persistedEvents := make([]model.AgentExecutionEvent, 0, len(events))
	for _, event := range events {
		if event.ExecutionID != stored.ExecutionID {
			return nil, fmt.Errorf("runner event %d belongs to execution %q", event.Seq, event.ExecutionID)
		}
		if event.Seq > lastEventSeq {
			lastEventSeq = event.Seq
		}
		payload, err := json.Marshal(event.Payload)
		if err != nil {
			return nil, fmt.Errorf("encode runner event %d payload: %w", event.Seq, err)
		}
		persistedEvents = append(persistedEvents, model.AgentExecutionEvent{
			ExecutionID: stored.ExecutionID, RunID: stored.RunID, Provider: stored.Provider,
			Seq: event.Seq, Type: event.Type, OccurredAt: event.At.UTC(), PayloadJSON: string(payload),
		})
	}
	if err := c.executions.UpsertEvents(ctx, persistedEvents); err != nil {
		return nil, err
	}
	remote, err := c.runner.Get(ctx, stored.ExecutionID)
	if err != nil {
		return nil, fmt.Errorf("get runner execution: %w", err)
	}
	if err := validateRemoteExecution(remote, stored.ExecutionID, stored.RequestHash); err != nil {
		return nil, err
	}
	projection, err := c.project(remote, stored.ProviderConfigRevision, lastEventSeq, stored.ProviderModel, stored.ProviderEffort)
	if err != nil {
		return nil, err
	}
	projection.ID = stored.ID
	projection.CreatedAt = stored.CreatedAt
	if err := c.executions.Update(ctx, projection); err != nil {
		return nil, err
	}
	observation := &AgentRunnerObservation{Execution: projection, Events: events}
	if remote.Status != runner.StatusSucceeded {
		return observation, nil
	}
	result, err := c.runner.Result(ctx, stored.ExecutionID)
	if err != nil {
		return nil, fmt.Errorf("get runner structured result: %w", err)
	}
	if result == nil || result.ExecutionID != stored.ExecutionID {
		return nil, errors.New("runner structured result has unexpected execution identity")
	}
	if remote.ResultHash != "" && result.ResultHash != remote.ResultHash {
		return nil, errors.New("runner structured result hash does not match execution")
	}
	observation.Result = result
	return observation, nil
}

func (c *AgentRunnerCoordinator) validateDependencies() error {
	if c == nil || c.tasks == nil || c.runs == nil || c.executions == nil || c.runner == nil {
		return errors.New("agent runner coordinator dependencies are required")
	}
	return nil
}

func (c *AgentRunnerCoordinator) project(remote *runner.Execution, revision int64, lastEventSeq int64, providerModel, providerEffort string) (*model.AgentExecution, error) {
	status, err := projectRunnerStatus(remote.Status)
	if err != nil {
		return nil, err
	}
	now := c.now().UTC()
	return &model.AgentExecution{
		RunID: remote.ExecutionID, ExecutionID: remote.ExecutionID, Provider: remote.Provider,
		ProviderConfigRevision: revision, ProviderModel: providerModel, ProviderEffort: providerEffort,
		Status: status, RequestHash: remote.RequestHash,
		LastEventSeq: lastEventSeq, LastObservedAt: &now, ResultHash: remote.ResultHash,
		ErrorCode: remote.ErrorCode, ErrorMessage: remote.ErrorMessage, StartedAt: remote.StartedAt, FinishedAt: remote.FinishedAt,
	}, nil
}

func validateAgentRunnerTask(task *model.Task, run *model.Run, request runner.CreateExecutionRequest) error {
	if task == nil || run == nil {
		return errors.New("agent runner task and run are required")
	}
	if strings.TrimSpace(task.TaskID) == "" || strings.TrimSpace(run.RunID) == "" || run.TaskID != task.TaskID {
		return errors.New("agent runner task and run identity is inconsistent")
	}
	if task.CurrentRunID == nil || *task.CurrentRunID != run.RunID {
		return errors.New("agent runner run is not current for task")
	}
	if task.Role == "" || run.WorkerRole != task.Role {
		return errors.New("agent runner task and run role is inconsistent")
	}
	if request.ExecutionID != run.RunID || request.TaskID != task.TaskID || request.Role != task.Role {
		return errors.New("agent runner request identity does not match task and run")
	}
	return nil
}
