package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store/repository"
)

var (
	ErrExecutionExitUnavailable  = errors.New("execution exit confirmation unavailable")
	ErrExecutionExitStillRunning = errors.New("execution is still running on the runner")
)

const (
	executionExitRemoteReadTimeout = 5 * time.Second
	executionExitMaxReasonLength   = 1000
	executionExitMaxRemoteMessage  = 512
)

// ExecutionExitReader reads one remote execution record without observing it:
// ExitConfirmation must not create events or touch last_observed_at.
type ExecutionExitReader interface {
	Get(context.Context, string) (*runner.Execution, error)
}

type ConfirmExecutionExitRequest struct {
	RequestID string
	Reason    string
}

func WithExecutionExitReader(resolver func(endpoint string) (ExecutionExitReader, error)) Option {
	return func(o *Orchestrator) { o.executionExitReader = resolver }
}

func WithExecutionExitConfirmationRepository(repository *repository.ExecutionExitConfirmationRepository) Option {
	return func(o *Orchestrator) { o.executionExits = repository }
}

// ConfirmExecutionExit records a human reconciliation for an interrupted source
// execution. The remote answer is stored as evidence; the local execution row is
// never rewritten, so a later change invalidates the confirmation fingerprint.
func (o *Orchestrator) ConfirmExecutionExit(ctx context.Context, taskID, actorID string, request ConfirmExecutionExitRequest) (*model.ExecutionExitConfirmation, error) {
	if o.recoveries == nil || o.executionExits == nil || o.executionExitReader == nil {
		return nil, ErrExecutionExitUnavailable
	}
	taskID = strings.TrimSpace(taskID)
	actorID = strings.TrimSpace(actorID)
	requestID := strings.TrimSpace(request.RequestID)
	reason := strings.TrimSpace(request.Reason)
	if taskID == "" || actorID == "" || requestID == "" || reason == "" || utf8.RuneCountInString(reason) > executionExitMaxReasonLength {
		return nil, ErrRecoveryInvalid
	}
	existing, err := o.executionExits.GetByRequest(ctx, taskID, requestID)
	if err == nil {
		if existing.RequestedBy == actorID && existing.Reason == reason {
			return existing, nil
		}
		return nil, repository.ErrRecoveryRequestConflict
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}

	snapshot, err := o.recoveries.LoadRecoverySnapshot(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if snapshot.Task == nil {
		return nil, repository.ErrNotFound
	}
	if snapshot.Task.Status != model.TaskStatusBlocked {
		return nil, repository.ErrRecoveryStateChanged
	}
	source := sourceRun(RecoveryFacts{Task: snapshot.Task, Runs: snapshot.Runs})
	if source == nil {
		return nil, ErrExecutionExitUnavailable
	}
	execution := latestExecutionForRun(snapshot.Executions, source.RunID)
	if execution == nil {
		return nil, ErrExecutionExitUnavailable
	}
	if execution.Status != model.AgentExecutionStatusInterrupted {
		return nil, repository.ErrExecutionNotInterrupted
	}
	remoteState, remoteMessage, err := o.readRemoteExecutionExit(ctx, source, execution)
	if err != nil {
		return nil, err
	}
	confirmedAt := time.Now().UTC()
	if o.now != nil {
		confirmedAt = o.now().UTC()
	}
	confirmation := &model.ExecutionExitConfirmation{
		ConfirmationID:       model.NewID("exitconfirmation"),
		TaskID:               snapshot.Task.TaskID,
		RunID:                execution.RunID,
		ExecutionID:          execution.ExecutionID,
		SourceGeneration:     snapshot.Task.RecoveryGeneration,
		ExecutionFingerprint: model.ExecutionExitFingerprint(execution),
		ExecutionStatus:      string(execution.Status),
		ExecutionErrorCode:   execution.ErrorCode,
		ExecutionFinishedAt:  execution.FinishedAt,
		ExecutionRequestHash: execution.RequestHash,
		RemoteState:          remoteState,
		RemoteMessage:        remoteMessage,
		RequestID:            requestID,
		RequestedBy:          actorID,
		Reason:               reason,
		ConfirmedAt:          confirmedAt,
	}
	stored, _, err := o.executionExits.Create(ctx, confirmation)
	if err != nil {
		return nil, err
	}
	return stored, nil
}

func (o *Orchestrator) readRemoteExecutionExit(ctx context.Context, run *model.Run, execution *model.AgentExecution) (string, string, error) {
	reader, err := o.executionExitReader(run.RunnerEndpoint)
	if err != nil || reader == nil {
		return model.ExecutionExitRemoteUnreachable, boundedRemoteMessage(fmt.Sprintf("runner client unavailable: %v", err)), nil
	}
	readCtx, cancel := context.WithTimeout(ctx, executionExitRemoteReadTimeout)
	defer cancel()
	remote, err := reader.Get(readCtx, execution.ExecutionID)
	if err != nil {
		if runnerNotFound(err) {
			return model.ExecutionExitRemoteNotFound, "runner has no record of this execution", nil
		}
		return model.ExecutionExitRemoteUnreachable, boundedRemoteMessage(err.Error()), nil
	}
	if remote == nil {
		return model.ExecutionExitRemoteUnreachable, "runner returned an empty execution record", nil
	}
	switch remote.Status {
	case runner.StatusAccepted, runner.StatusStarting, runner.StatusRunning:
		return "", "", ErrExecutionExitStillRunning
	}
	return model.ExecutionExitRemoteTerminal, boundedRemoteMessage("runner reports " + string(remote.Status)), nil
}

func boundedRemoteMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > executionExitMaxRemoteMessage {
		return message[:executionExitMaxRemoteMessage]
	}
	return message
}
