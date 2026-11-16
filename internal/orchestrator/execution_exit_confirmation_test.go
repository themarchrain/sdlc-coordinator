package orchestrator

import (
	"context"
	"errors"
	"net/http"
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
	reader := &fakeExecutionExitReader{remote: &runner.Execution{ExecutionID: runID, Status: runner.StatusInterrupted}}
	orchestrator := New(repository.NewTaskRepository(db), repository.NewRunRepository(db),
		WithTaskRecoveryRepository(repository.NewTaskRecoveryRepository(db)),
		WithExecutionExitConfirmationRepository(repository.NewExecutionExitConfirmationRepository(db)),
		WithExecutionExitReader(func(string) (ExecutionExitReader, error) { return reader, nil }),
	)
	return &exitConfirmationFixture{db: db, task: task, run: run, exec: exec, reader: reader, orchestrator: orchestrator}
}

func (f *exitConfirmationFixture) confirm(t *testing.T, request ConfirmExecutionExitRequest) *model.ExecutionExitConfirmation {
	t.Helper()
	confirmation, err := f.orchestrator.ConfirmExecutionExit(context.Background(), f.task.TaskID, "tenant-admin-1", request)
	if err != nil {
		t.Fatalf("confirm execution exit: %v", err)
	}
	return confirmation
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
	fixture.reader.remote = &runner.Execution{ExecutionID: fixture.run.RunID, Status: runner.StatusRunning}
	if _, err := fixture.orchestrator.ConfirmExecutionExit(context.Background(), fixture.task.TaskID, "tenant-admin-1", ConfirmExecutionExitRequest{RequestID: "request-exit-running", Reason: "should be rejected"}); !errors.Is(err, ErrExecutionExitStillRunning) {
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

func TestConfirmExecutionExitRequiresInterruptedLocalExecution(t *testing.T) {
	fixture := openExitConfirmationOrchestrator(t)
	if err := fixture.db.Model(&model.AgentExecution{}).Where("run_id = ?", fixture.run.RunID).
		Update("status", model.AgentExecutionStatusFailed).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.orchestrator.ConfirmExecutionExit(context.Background(), fixture.task.TaskID, "tenant-admin-1", ConfirmExecutionExitRequest{RequestID: "request-exit-failed", Reason: "not needed"}); !errors.Is(err, repository.ErrExecutionNotInterrupted) {
		t.Fatalf("non-interrupted execution error = %v", err)
	}
	if fixture.reader.calls != 0 {
		t.Fatalf("remote runner was read for an execution that does not need confirmation: %d", fixture.reader.calls)
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
	request := ConfirmExecutionExitRequest{RequestID: "request-exit-replay", Reason: "checked the host"}
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
	}
}
