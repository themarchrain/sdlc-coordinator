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
	ErrExecutionExitUnavailable   = errors.New("execution exit confirmation unavailable")
	ErrExecutionExitStillRunning  = errors.New("execution is still running on the runner")
	ErrExecutionExitStatusUnknown = errors.New("the runner reported an unrecognised execution status")
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
	ExecutionID         string
	ExpectedFingerprint string
	RequestID           string
	Reason              string
}

func WithExecutionExitReader(resolver func(endpoint string) (ExecutionExitReader, error)) Option {
	return func(o *Orchestrator) { o.executionExitReader = resolver }
}

func WithExecutionExitConfirmationRepository(repository *repository.ExecutionExitConfirmationRepository) Option {
	return func(o *Orchestrator) { o.executionExits = repository }
}

// ConfirmExecutionExit records a human reconciliation for one execution the
// operator selected by id. The expected fingerprint pins the evidence the
// operator saw, so a row that changed in the meantime is refused rather than
// silently confirmed. The remote answer is stored as evidence; the local
// execution row is never rewritten.
func (o *Orchestrator) ConfirmExecutionExit(ctx context.Context, taskID, actorID string, request ConfirmExecutionExitRequest) (*model.ExecutionExitConfirmation, error) {
	if o.recoveries == nil || o.executionExits == nil || o.executionExitReader == nil {
		return nil, ErrExecutionExitUnavailable
	}
	taskID = strings.TrimSpace(taskID)
	actorID = strings.TrimSpace(actorID)
	executionID := strings.TrimSpace(request.ExecutionID)
	expectedFingerprint := strings.TrimSpace(request.ExpectedFingerprint)
	requestID := strings.TrimSpace(request.RequestID)
	reason := strings.TrimSpace(request.Reason)
	if taskID == "" || actorID == "" || executionID == "" || expectedFingerprint == "" || requestID == "" || reason == "" || utf8.RuneCountInString(reason) > executionExitMaxReasonLength {
		return nil, ErrRecoveryInvalid
	}
	existing, err := o.executionExits.GetByRequest(ctx, taskID, requestID)
	if err == nil {
		// 请求编号只对同一条执行、同一份证据可重放：换一条执行再提交是
		// 新意图，不能把上一份确认当成它的结果返回。
		if existing.RequestedBy == actorID && existing.Reason == reason &&
			existing.ExecutionID == executionID && existing.ExecutionFingerprint == expectedFingerprint {
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
	execution := executionByID(snapshot.Executions, executionID)
	if execution == nil {
		return nil, repository.ErrExecutionExitNotFound
	}
	if model.ExecutionIsActive(execution) {
		return nil, ErrExecutionExitStillRunning
	}
	if !model.ExecutionNeedsExitConfirmation(execution) {
		return nil, repository.ErrExecutionExitNotRequired
	}
	if model.ExecutionExitFingerprint(execution) != expectedFingerprint {
		return nil, repository.ErrExecutionFingerprintChanged
	}
	remoteState, remoteMessage, err := o.readRemoteExecutionExit(ctx, runByID(snapshot.Runs, execution.RunID), execution)
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

// executionByID returns the execution row the operator named. The row is looked
// up inside the task snapshot, so an execution id from another task cannot be
// confirmed through this task.
func executionByID(executions []model.AgentExecution, executionID string) *model.AgentExecution {
	for index := range executions {
		if executions[index].ExecutionID == executionID {
			return &executions[index]
		}
	}
	return nil
}

func runByID(runs []model.Run, runID string) *model.Run {
	for index := range runs {
		if runs[index].RunID == runID {
			return &runs[index]
		}
	}
	return nil
}

func (o *Orchestrator) readRemoteExecutionExit(ctx context.Context, run *model.Run, execution *model.AgentExecution) (string, string, error) {
	// 执行可能被池回退换过端点：观测与清理都跟 execution 记录的端点走，
	// 只有旧数据没有该字段时才退回 Run 的端点。
	endpoint := strings.TrimSpace(execution.RunnerEndpoint)
	if endpoint == "" && run != nil {
		endpoint = strings.TrimSpace(run.RunnerEndpoint)
	}
	reader, err := o.executionExitReader(endpoint)
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
	if err := validateRemoteExecution(remote, execution.ExecutionID, execution.RequestHash); err != nil {
		// 记录身份不一致的远端返回不能当退出证据，只留一条不可信说明。
		return model.ExecutionExitRemoteUnreachable, boundedRemoteMessage("runner returned a mismatched execution record: " + err.Error()), nil
	}
	switch remote.Status {
	case runner.StatusAccepted, runner.StatusStarting, runner.StatusRunning:
		return "", "", ErrExecutionExitStillRunning
	case runner.StatusSucceeded, runner.StatusFailed, runner.StatusCancelled, runner.StatusInterrupted:
		return model.ExecutionExitRemoteTerminal, boundedRemoteMessage("runner reports " + string(remote.Status)), nil
	}
	return "", "", fmt.Errorf("%w: %q", ErrExecutionExitStatusUnknown, string(remote.Status))
}

func boundedRemoteMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > executionExitMaxRemoteMessage {
		return message[:executionExitMaxRemoteMessage]
	}
	return message
}
