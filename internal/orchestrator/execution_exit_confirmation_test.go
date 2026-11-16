package orchestrator

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"

	"gorm.io/gorm"
)

type fakeExecutionExitReader struct {
	remote *runner.Execution
	err    error
	calls  int
}

func (f *fakeExecutionExitReader) Get(context.Context, string) (*runner.Execution, error) {
	f.calls++
	return f.remote, f.err
}

type exitConfirmationFixture struct {
	db           *gorm.DB
	task         *model.Task
	run          *model.Run
	exec         *model.AgentExecution
	reader       *fakeExecutionExitReader
	endpoint     string
	orchestrator *Orchestrator
}

func openExitConfirmationOrchestrator(t *testing.T) *exitConfirmationFixture {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.ExecutionExitConfirmation{}, &model.TaskRecovery{}, &model.StopRequest{}, &model.WorkItemLease{}, &model.ActionAdmission{}, &model.ResultSubmission{}, &model.Writeback{}, &model.Effect{}, &model.DeveloperDeliveryCheckpoint{}, &model.MergeGateAudit{}); err != nil {
		t.Fatal(err)
	}
	runID := "run-exit-confirmation"
	task := &model.Task{
		TaskID: "task-exit-confirmation", DedupeKey: "dedupe-exit-confirmation", Type: "bugfix", Role: "DeveloperWorker",
		Status: model.TaskStatusBlocked, AttemptCount: 5, CurrentRunID: &runID, ContextJSON: `{}`,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusBlocked}
	if err := db.Create(run).Error; err != nil {
		t.Fatal(err)
	}
	finished := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	exec := &model.AgentExecution{
		RunID: runID, ExecutionID: runID, Provider: "codex", Status: model.AgentExecutionStatusInterrupted,
		ErrorCode: "runner_restarted", RequestHash: "request-hash", FinishedAt: &finished,
	}
	if err := db.Create(exec).Error; err != nil {
		t.Fatal(err)
	}
	reader := &fakeExecutionExitReader{remote: &runner.Execution{ExecutionID: runID, Status: runner.StatusInterrupted, RequestHash: exec.RequestHash}}
	fixture := &exitConfirmationFixture{db: db, task: task, run: run, exec: exec, reader: reader}
	orchestrator := New(repository.NewTaskRepository(db), repository.NewRunRepository(db),
		WithTaskRecoveryRepository(repository.NewTaskRecoveryRepository(db)),
		WithExecutionExitConfirmationRepository(repository.NewExecutionExitConfirmationRepository(db)),
		WithExecutionExitReader(func(endpoint string) (ExecutionExitReader, error) {
			fixture.endpoint = endpoint
			return reader, nil
		}),
	)
	fixture.orchestrator = orchestrator
	return fixture
}

func (f *exitConfirmationFixture) confirm(t *testing.T, request ConfirmExecutionExitRequest) *model.ExecutionExitConfirmation {
	t.Helper()
	confirmation, err := f.orchestrator.ConfirmExecutionExit(context.Background(), f.task.TaskID, "tenant-admin-1", f.bound(request, f.exec))
	if err != nil {
		t.Fatalf("confirm execution exit: %v", err)
	}
	return confirmation
}

// bound fills the execution binding the operator's page supplied, so each test
// states only what it actually exercises.
func (f *exitConfirmationFixture) bound(request ConfirmExecutionExitRequest, execution *model.AgentExecution) ConfirmExecutionExitRequest {
	if request.ExecutionID == "" {
		request.ExecutionID = execution.ExecutionID
	}
	if request.ExpectedFingerprint == "" {
		request.ExpectedFingerprint = model.ExecutionExitFingerprint(execution)
	}
	return request
}

func TestConfirmExecutionExitRecordsTerminalEvidence(t *testing.T) {
	fixture := openExitConfirmationOrchestrator(t)
	confirmation := fixture.confirm(t, ConfirmExecutionExitRequest{RequestID: "request-exit-terminal", Reason: "checked the runner host, no process left"})
	if confirmation.RemoteState != model.ExecutionExitRemoteTerminal {
		t.Fatalf("remote state = %q", confirmation.RemoteState)
	}
	if confirmation.ExecutionFingerprint != model.ExecutionExitFingerprint(fixture.exec) || confirmation.SourceGeneration != 0 {
		t.Fatalf("confirmation evidence = %+v", confirmation)
	}
	var stored model.AgentExecution
	if err := fixture.db.Where("run_id = ?", fixture.run.RunID).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.AgentExecutionStatusInterrupted {
		t.Fatalf("confirmation changed the execution row: %s", stored.Status)
	}
	assessment := EvaluateTaskRecovery(ExitConfirmationFacts(fixture.db, t, fixture.task.TaskID))
	if !assessment.Eligible {
		t.Fatalf("confirmed recovery is not eligible: %+v", assessment.SafetyBlockers)
	}
}

func TestConfirmExecutionExitRejectsRunningRemoteExecution(t *testing.T) {
	fixture := openExitConfirmationOrchestrator(t)
	fixture.reader.remote = &runner.Execution{ExecutionID: fixture.exec.ExecutionID, Status: runner.StatusRunning, RequestHash: fixture.exec.RequestHash}
	if _, err := fixture.orchestrator.ConfirmExecutionExit(context.Background(), fixture.task.TaskID, "tenant-admin-1", fixture.bound(ConfirmExecutionExitRequest{RequestID: "request-exit-running", Reason: "should be rejected"}, fixture.exec)); !errors.Is(err, ErrExecutionExitStillRunning) {
		t.Fatalf("running remote error = %v", err)
	}
	var count int64
	if err := fixture.db.Model(&model.ExecutionExitConfirmation{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("rejected confirmation was stored: count=%d err=%v", count, err)
	}
}

func TestConfirmExecutionExitRecordsMissingAndUnreachableRunners(t *testing.T) {
	tests := []struct {
		name      string
		readerErr error
		want      string
	}{
		{name: "runner has no record", readerErr: &runner.HTTPError{StatusCode: http.StatusNotFound}, want: model.ExecutionExitRemoteNotFound},
		{name: "runner unreachable", readerErr: errors.New("dial tcp 10.0.0.1:8081: connect: connection refused"), want: model.ExecutionExitRemoteUnreachable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := openExitConfirmationOrchestrator(t)
			fixture.reader.remote = nil
			fixture.reader.err = test.readerErr
			confirmation := fixture.confirm(t, ConfirmExecutionExitRequest{RequestID: "request-exit-" + test.want, Reason: "recorded the runner answer as evidence"})
			if confirmation.RemoteState != test.want {
				t.Fatalf("remote state = %q, want %q", confirmation.RemoteState, test.want)
			}
		})
	}
}

func TestConfirmExecutionExitRequiresAnExecutionWithoutExitProof(t *testing.T) {
	fixture := openExitConfirmationOrchestrator(t)
	// The row still needs confirmation, but the evidence the operator saw changed.
	if err := fixture.db.Model(&model.AgentExecution{}).Where("run_id = ?", fixture.run.RunID).
		Update("error_code", "provider_failed").Error; err != nil {
		t.Fatal(err)
	}
	stale := fixture.bound(ConfirmExecutionExitRequest{RequestID: "request-exit-stale", Reason: "checked the host"}, fixture.exec)
	if _, err := fixture.orchestrator.ConfirmExecutionExit(context.Background(), fixture.task.TaskID, "tenant-admin-1", stale); !errors.Is(err, repository.ErrExecutionFingerprintChanged) {
		t.Fatalf("stale fingerprint error = %v", err)
	}
	// A terminal row with a finish time proves its own exit.
	if err := fixture.db.Model(&model.AgentExecution{}).Where("run_id = ?", fixture.run.RunID).
		Update("status", model.AgentExecutionStatusFailed).Error; err != nil {
		t.Fatal(err)
	}
	var current model.AgentExecution
	if err := fixture.db.Where("run_id = ?", fixture.run.RunID).First(&current).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.orchestrator.ConfirmExecutionExit(context.Background(), fixture.task.TaskID, "tenant-admin-1", fixture.bound(ConfirmExecutionExitRequest{RequestID: "request-exit-finished", Reason: "not needed"}, &current)); !errors.Is(err, repository.ErrExecutionExitNotRequired) {
		t.Fatalf("execution with a finish time error = %v", err)
	}
	if fixture.reader.calls != 0 {
		t.Fatalf("remote runner was read for an execution that does not need confirmation: %d", fixture.reader.calls)
	}
}

func TestConfirmExecutionExitRequiresTheExecutionAndFingerprint(t *testing.T) {
	fixture := openExitConfirmationOrchestrator(t)
	for name, request := range map[string]ConfirmExecutionExitRequest{
		"missing execution":   {RequestID: "request-exit-no-execution", Reason: "reason", ExpectedFingerprint: "fingerprint"},
		"missing fingerprint": {RequestID: "request-exit-no-fingerprint", Reason: "reason", ExecutionID: fixture.exec.ExecutionID},
	} {
		if _, err := fixture.orchestrator.ConfirmExecutionExit(context.Background(), fixture.task.TaskID, "tenant-admin-1", request); !errors.Is(err, ErrRecoveryInvalid) {
			t.Fatalf("%s error = %v", name, err)
		}
	}
	if fixture.reader.calls != 0 {
		t.Fatalf("remote runner was read for an invalid request: %d", fixture.reader.calls)
	}
}

func TestConfirmExecutionExitReadsTheExecutionRunnerEndpoint(t *testing.T) {
	fixture := openExitConfirmationOrchestrator(t)
	if err := fixture.db.Model(&model.AgentExecution{}).Where("run_id = ?", fixture.run.RunID).
		Update("runner_endpoint", "http://runner-b:8081").Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&model.Run{}).Where("run_id = ?", fixture.run.RunID).
		Update("runner_endpoint", "http://runner-a:8081").Error; err != nil {
		t.Fatal(err)
	}
	fixture.confirm(t, ConfirmExecutionExitRequest{RequestID: "request-exit-endpoint", Reason: "checked the host"})
	if fixture.endpoint != "http://runner-b:8081" {
		t.Fatalf("remote read endpoint = %q, want the endpoint recorded on the execution", fixture.endpoint)
	}
}

func TestConfirmExecutionExitRejectsUnknownRemoteStatus(t *testing.T) {
	fixture := openExitConfirmationOrchestrator(t)
	fixture.reader.remote = &runner.Execution{ExecutionID: fixture.exec.ExecutionID, Status: "", RequestHash: fixture.exec.RequestHash}
	if _, err := fixture.orchestrator.ConfirmExecutionExit(context.Background(), fixture.task.TaskID, "tenant-admin-1",
		fixture.bound(ConfirmExecutionExitRequest{RequestID: "request-exit-unknown", Reason: "checked the host"}, fixture.exec)); !errors.Is(err, ErrExecutionExitStatusUnknown) {
		t.Fatalf("unknown remote status error = %v", err)
	}
	var count int64
	if err := fixture.db.Model(&model.ExecutionExitConfirmation{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("unknown status confirmation was stored: count=%d err=%v", count, err)
	}
}

func TestConfirmExecutionExitRecordsMismatchedRemoteIdentityAsUnreachable(t *testing.T) {
	fixture := openExitConfirmationOrchestrator(t)
	fixture.reader.remote = &runner.Execution{ExecutionID: "another-execution", Status: runner.StatusSucceeded, RequestHash: "another-hash"}
	confirmation := fixture.confirm(t, ConfirmExecutionExitRequest{RequestID: "request-exit-mismatch", Reason: "checked the host"})
	if confirmation.RemoteState != model.ExecutionExitRemoteUnreachable {
		t.Fatalf("mismatched remote record was used as exit evidence: %q", confirmation.RemoteState)
	}
	if !strings.Contains(confirmation.RemoteMessage, "mismatched execution record") {
		t.Fatalf("mismatched remote record was not explained: %q", confirmation.RemoteMessage)
	}
}

func TestConfirmExecutionExitAcceptsAnOlderInterruptedExecution(t *testing.T) {
	fixture := openExitConfirmationOrchestrator(t)
	olderRun := &model.Run{RunID: "run-older", TaskID: fixture.task.TaskID, WorkerRole: fixture.task.Role, Status: model.RunStatusFailed}
	if err := fixture.db.Create(olderRun).Error; err != nil {
		t.Fatal(err)
	}
	olderExecution := &model.AgentExecution{
		RunID: olderRun.RunID, ExecutionID: "execution-older", Provider: "codex", Status: model.AgentExecutionStatusInterrupted,
		ErrorCode: "runner_restarted", RequestHash: "request-hash-older",
	}
	if err := fixture.db.Create(olderExecution).Error; err != nil {
		t.Fatal(err)
	}
	assessment := EvaluateTaskRecovery(ExitConfirmationFacts(fixture.db, t, fixture.task.TaskID))
	if assessment.Eligible || !hasSafetyBlocker(assessment, RecoverySafetyExecutionUnconfirmed) {
		t.Fatalf("older interrupted execution did not block recovery: %+v", assessment.SafetyBlockers)
	}
	if len(assessment.UnconfirmedExecutions) != 2 {
		t.Fatalf("unconfirmed executions = %+v, want both rows", assessment.UnconfirmedExecutions)
	}
	fixture.confirm(t, ConfirmExecutionExitRequest{ExecutionID: olderExecution.ExecutionID, ExpectedFingerprint: model.ExecutionExitFingerprint(olderExecution), RequestID: "request-exit-older", Reason: "confirmed the older process is gone"})
	if err := fixture.db.Model(&model.AgentExecution{}).Where("run_id = ?", fixture.run.RunID).
		Update("status", model.AgentExecutionStatusFailed).Error; err != nil {
		t.Fatal(err)
	}
	assessment = EvaluateTaskRecovery(ExitConfirmationFacts(fixture.db, t, fixture.task.TaskID))
	if !assessment.Eligible {
		t.Fatalf("confirming the older execution did not clear the gate: %+v", assessment.SafetyBlockers)
	}
	if len(assessment.UnconfirmedExecutions) != 0 {
		t.Fatalf("confirmed execution still listed: %+v", assessment.UnconfirmedExecutions)
	}
}

func TestExitConfirmationEvidenceClearsOnlyTheExecutionItNames(t *testing.T) {
	fixture := openExitConfirmationOrchestrator(t)
	olderRun := &model.Run{RunID: "run-other", TaskID: fixture.task.TaskID, WorkerRole: fixture.task.Role, Status: model.RunStatusFailed}
	if err := fixture.db.Create(olderRun).Error; err != nil {
		t.Fatal(err)
	}
	otherExecution := &model.AgentExecution{
		RunID: olderRun.RunID, ExecutionID: "execution-other", Provider: "codex", Status: model.AgentExecutionStatusInterrupted,
		ErrorCode: "runner_restarted", RequestHash: "request-hash-other",
	}
	if err := fixture.db.Create(otherExecution).Error; err != nil {
		t.Fatal(err)
	}
	confirmation := fixture.confirm(t, ConfirmExecutionExitRequest{ExecutionID: otherExecution.ExecutionID, ExpectedFingerprint: model.ExecutionExitFingerprint(otherExecution), RequestID: "request-exit-other", Reason: "confirmed the other process is gone"})
	if confirmation.ExecutionID != otherExecution.ExecutionID {
		t.Fatalf("confirmation bound execution = %q", confirmation.ExecutionID)
	}
	assessment := EvaluateTaskRecovery(ExitConfirmationFacts(fixture.db, t, fixture.task.TaskID))
	if assessment.Eligible {
		t.Fatalf("one confirmation cleared every execution: %+v", assessment.SafetyBlockers)
	}
	for _, unconfirmed := range assessment.UnconfirmedExecutions {
		if unconfirmed.ExecutionID == otherExecution.ExecutionID {
			t.Fatalf("confirmed execution is still listed: %+v", unconfirmed)
		}
	}
	if len(assessment.UnconfirmedExecutions) != 1 || assessment.UnconfirmedExecutions[0].ExecutionID != fixture.exec.ExecutionID {
		t.Fatalf("unconfirmed executions = %+v", assessment.UnconfirmedExecutions)
	}
}

func TestConfirmExecutionExitRequiresReason(t *testing.T) {
	fixture := openExitConfirmationOrchestrator(t)
	if _, err := fixture.orchestrator.ConfirmExecutionExit(context.Background(), fixture.task.TaskID, "tenant-admin-1", ConfirmExecutionExitRequest{RequestID: "request-exit-blank", Reason: "   "}); !errors.Is(err, ErrRecoveryInvalid) {
		t.Fatalf("blank reason error = %v", err)
	}
	if _, err := fixture.orchestrator.ConfirmExecutionExit(context.Background(), fixture.task.TaskID, "tenant-admin-1", ConfirmExecutionExitRequest{RequestID: "", Reason: "reason"}); !errors.Is(err, ErrRecoveryInvalid) {
		t.Fatalf("missing request id error = %v", err)
	}
}

func TestConfirmExecutionExitReplaysSameRequest(t *testing.T) {
	fixture := openExitConfirmationOrchestrator(t)
	request := fixture.bound(ConfirmExecutionExitRequest{RequestID: "request-exit-replay", Reason: "checked the host"}, fixture.exec)
	first := fixture.confirm(t, request)
	second := fixture.confirm(t, request)
	if second.ConfirmationID != first.ConfirmationID {
		t.Fatalf("replay created another confirmation: %s != %s", second.ConfirmationID, first.ConfirmationID)
	}
	if fixture.reader.calls != 1 {
		t.Fatalf("replay re-read the remote runner: %d calls", fixture.reader.calls)
	}
	changed := request
	changed.Reason = "another reason"
	if _, err := fixture.orchestrator.ConfirmExecutionExit(context.Background(), fixture.task.TaskID, "tenant-admin-1", changed); !errors.Is(err, repository.ErrRecoveryRequestConflict) {
		t.Fatalf("changed replay error = %v", err)
	}
}

func TestConfirmExecutionExitDoesNotClearOtherSafetyGates(t *testing.T) {
	fixture := openExitConfirmationOrchestrator(t)
	fixture.confirm(t, ConfirmExecutionExitRequest{RequestID: "request-exit-gates", Reason: "checked the host"})
	submission := &model.ResultSubmission{
		SubmissionID: "submission-exit-gates", TaskID: fixture.task.TaskID, RunID: fixture.run.RunID,
		Source: "mcp", Role: "DeveloperWorker", PayloadHash: "payload-hash",
		RawPayloadJSON: `{}`, CanonicalJSON: `{}`, Status: model.ResultSubmissionStatusProcessing,
	}
	if err := fixture.db.Create(submission).Error; err != nil {
		t.Fatal(err)
	}
	assessment := EvaluateTaskRecovery(ExitConfirmationFacts(fixture.db, t, fixture.task.TaskID))
	if assessment.Eligible {
		t.Fatalf("exit confirmation cleared unrelated safety gates: %+v", assessment)
	}
	if !hasSafetyBlocker(assessment, RecoverySafetySubmissionProcessing) {
		t.Fatalf("processing submission gate is missing: %+v", assessment.SafetyBlockers)
	}
}

// ExitConfirmationFacts loads recovery facts the way the safety reader does.
func ExitConfirmationFacts(db *gorm.DB, t *testing.T, taskID string) RecoveryFacts {
	t.Helper()
	snapshot, err := repository.NewTaskRecoveryRepository(db).LoadRecoverySnapshot(context.Background(), taskID)
	if err != nil {
		t.Fatal(err)
	}
	valid := true
	return RecoveryFacts{
		Task:              snapshot.Task,
		Runs:              snapshot.Runs,
		Executions:        snapshot.Executions,
		ResultSubmissions: snapshot.ResultSubmissions,
		Writebacks:        snapshot.Writebacks,
		Effects:           snapshot.Effects,
		ActionAdmissions:  snapshot.ActionAdmissions,
		WorkItemLeases:    snapshot.WorkItemLeases,
		ExitConfirmations: snapshot.ExitConfirmations,
		MergeGateAudit:    snapshot.MergeGateAudit,
		Stop:              snapshot.Stop,
		TaskObjectValid:   &valid,
		ContextValid:      &valid,
		Repository:        &registeredrepo.RegisteredRepository{RepositoryID: "repo-1", Status: registeredrepo.StatusEnabled},
		CheckedAt:         time.Date(2026, 11, 16, 10, 10, 0, 0, time.UTC),

		DispatchProvenNotSentRuns: repository.RecoveryDispatchProvenNotSentRuns(snapshot),
	}
}
