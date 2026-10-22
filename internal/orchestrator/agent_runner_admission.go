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

func dispatchSnapshot(request runner.CreateExecutionRequest) (string, error) {
	request.Provider.APIKey = ""
	headers := make(map[string]string, len(request.Provider.Headers))
	for name := range request.Provider.Headers {
		headers[name] = ""
	}
	request.Provider.Headers = headers
	request.ResultSubmission = nil
	raw, err := json.Marshal(request)
	return string(raw), err
}

func (c *AgentRunnerCoordinator) configureActionAdmissions(admissions ActionAdmissionPort, leases WorkItemLeasePort, current runner.ProviderConfig) {
	c.SetActionAdmissions(admissions, leases)
	c.dispatchCredentials = func(_ context.Context, snapshot runner.ProviderConfig) (runner.ProviderConfig, error) {
		return RefreshDispatchCredentials(current, snapshot)
	}
}

func (c *DeveloperRunnerCoordinator) SetActionAdmissions(admissions ActionAdmissionPort, leases WorkItemLeasePort, current runner.ProviderConfig) {
	c.core.configureActionAdmissions(admissions, leases, current)
	c.channel.SetActionAdmissions(admissions)
	c.channel.SetDeliveryExecutionRight(leases)
}

func (c *ReviewerRunnerCoordinator) SetActionAdmissions(admissions ActionAdmissionPort, leases WorkItemLeasePort, current runner.ProviderConfig) {
	c.core.configureActionAdmissions(admissions, leases, current)
	c.channel.SetActionAdmissions(admissions)
}

func (c *TesterRunnerCoordinator) SetActionAdmissions(admissions ActionAdmissionPort, leases WorkItemLeasePort, current runner.ProviderConfig) {
	c.core.configureActionAdmissions(admissions, leases, current)
	c.channel.SetActionAdmissions(admissions)
}

func dispatchAdmissionDenied(err error) bool {
	return errors.Is(err, repository.ErrTaskInterrupted) || errors.Is(err, repository.ErrActionLeaseLost) || errors.Is(err, repository.ErrActionAlreadyAdmitted) || errors.Is(err, repository.ErrActionOutcomeUnknown) || errors.Is(err, repository.ErrTaskNotClaimable) || errors.Is(err, repository.ErrStopNotCurrentRun) || errors.Is(err, ErrStopUnavailable)
}

func (c *AgentRunnerCoordinator) assertDispatchActive(ctx context.Context, taskID, runID string) error {
	task, err := c.tasks.GetByTaskID(ctx, taskID)
	if err != nil {
		return err
	}
	if task.Status == model.TaskStatusInterrupted {
		return repository.ErrTaskInterrupted
	}
	if task.CurrentRunID == nil || *task.CurrentRunID != runID || (task.Status != model.TaskStatusRunning && task.Status != model.TaskStatusFinalizing) {
		return repository.ErrTaskNotClaimable
	}
	return nil
}

func (c *AgentRunnerCoordinator) createAdmitted(ctx context.Context, client ReviewerRunnerClient, request runner.CreateExecutionRequest, replay bool) (*runner.Execution, error) {
	if c.admissions == nil {
		return client.Create(ctx, request)
	}
	if c.leases == nil {
		return nil, ErrStopUnavailable
	}
	allocator, ok := c.admissions.(interface {
		NextAttempt(context.Context, model.ActionKind, string) (int, error)
	})
	if !ok {
		return nil, ErrStopUnavailable
	}
	lease, won, err := c.leases.Acquire(ctx, "execution-create:"+request.ExecutionID, model.NewID("create"), request.ExecutionID, time.Minute)
	if err != nil {
		return nil, err
	}
	if !won {
		return nil, repository.ErrActionLeaseLost
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = c.leases.Release(releaseCtx, lease.Scope, lease.Owner, lease.Generation)
	}()
	attempt, err := allocator.NextAttempt(ctx, model.ActionExecutionCreate, request.ExecutionID)
	if err != nil {
		return nil, err
	}
	hash, err := runner.RequestHash(request)
	if err != nil {
		return nil, err
	}
	admission, err := c.admissions.AdmitAction(ctx, ActionAdmissionRequest{TaskID: request.TaskID, RunID: request.ExecutionID, Kind: model.ActionExecutionCreate, ActionKey: request.ExecutionID, Attempt: attempt, LeaseOwner: lease.Owner, LeaseGeneration: lease.Generation, IdempotentReplay: replay, RequestHash: hash})
	if err != nil {
		return nil, err
	}
	remote, err := client.Create(ctx, request)
	if runnerCreateDefinitelyRejected(err) {
		proof, ok := c.admissions.(interface {
			ConfirmNotSent(context.Context, *model.ActionAdmission) error
		})
		if !ok {
			return nil, ErrStopUnavailable
		}
		if proofErr := proof.ConfirmNotSent(ctx, admission); proofErr != nil {
			return nil, proofErr
		}
	}
	return remote, err
}

func (c *AgentRunnerCoordinator) replayDispatchIntent(ctx context.Context, stored *model.AgentExecution, fresh *runner.CreateExecutionRequest) (*model.AgentExecution, error) {
	if stored.CreateSpecJSON == "" {
		return nil, fmt.Errorf("%w: persisted create has no reproducible business snapshot", errDispatchOutcomeUnknown)
	}
	var request runner.CreateExecutionRequest
	if err := json.Unmarshal([]byte(stored.CreateSpecJSON), &request); err != nil {
		return nil, fmt.Errorf("%w: invalid persisted create specification", errDispatchOutcomeUnknown)
	}
	if err := c.assertDispatchActive(ctx, request.TaskID, stored.RunID); err != nil {
		return nil, err
	}
	if fresh != nil {
		request.Provider.APIKey = fresh.Provider.APIKey
		request.Provider.Headers = fresh.Provider.Headers
		request.ResultSubmission = fresh.ResultSubmission
	} else {
		if c.dispatchCredentials == nil {
			return nil, fmt.Errorf("%w: create credential resolver is unavailable", errDispatchOutcomeUnknown)
		}
		provider, err := c.dispatchCredentials(ctx, request.Provider)
		if err != nil {
			return nil, fmt.Errorf("%w: create credentials cannot be resolved", errDispatchOutcomeUnknown)
		}
		request.Provider = provider
		if c.ingressIssuer != nil && c.ingressEndpoint != "" {
			token, err := c.ingressIssuer.IssueRunToken(request.TaskID, stored.RunID, request.Role, time.Duration(request.TimeoutSeconds)*time.Second)
			if err != nil {
				return nil, err
			}
			request.ResultSubmission = &runner.ResultSubmissionChannel{EndpointURL: c.ingressEndpoint, BearerToken: token}
		}
	}
	hash, err := runner.RequestHash(request)
	if err != nil || hash != stored.RequestHash || request.ExecutionID != stored.ExecutionID {
		return nil, fmt.Errorf("%w: reconstructed create identity differs from persisted request", errDispatchOutcomeUnknown)
	}
	client, err := c.pinnedExecutionClient(stored)
	if err != nil {
		return nil, err
	}
	remote, err := c.createAdmitted(ctx, client, request, true)
	if err != nil {
		return nil, err
	}
	if err := validateRemoteExecution(remote, stored.ExecutionID, stored.RequestHash); err != nil {
		return nil, err
	}
	return c.projectRemoteIntoIntent(ctx, stored, remote, stored.LastEventSeq)
}

func (c *AgentRunnerCoordinator) pinnedExecutionClient(stored *model.AgentExecution) (ReviewerRunnerClient, error) {
	if c.admissions == nil {
		return c.executionClient(stored), nil
	}
	if c.router != nil && stored.RunnerEndpoint != "" {
		if client, ok := c.router.ClientForEndpoint(stored.RunnerEndpoint); ok {
			return client, nil
		}
	}
	return nil, fmt.Errorf("%w: original runner client is unavailable", errDispatchOutcomeUnknown)
}

func RefreshDispatchCredentials(current runner.ProviderConfig, snapshot runner.ProviderConfig) (runner.ProviderConfig, error) {
	if strings.TrimRight(current.BaseURL, "/") != strings.TrimRight(snapshot.BaseURL, "/") {
		return runner.ProviderConfig{}, fmt.Errorf("provider credential target has changed")
	}
	keys := make(map[string]string, len(snapshot.Headers))
	for name := range snapshot.Headers {
		found := false
		for key, value := range current.Headers {
			if strings.EqualFold(key, name) {
				keys[name], found = value, true
				break
			}
		}
		if !found {
			return runner.ProviderConfig{}, fmt.Errorf("provider credential header is no longer configured")
		}
	}
	snapshot.APIKey, snapshot.Headers = current.APIKey, keys
	return snapshot, nil
}
