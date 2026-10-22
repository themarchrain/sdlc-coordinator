package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/workspace"
	"sdlc-coordinator/internal/workspaceproto"
)

func admittedGitFixture(t *testing.T) (resultChannelFixture, *repository.StopRepository) {
	t.Helper()
	db, _, _, _ := externalAdmissionFixture(t)
	f := newResultChannelFixture("DeveloperWorker")
	f.task.DedupeKey = "channel"
	if err := db.Create(f.task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(f.run).Error; err != nil {
		t.Fatal(err)
	}
	f.core.SetDeliveryExecutionRight(repository.NewWorkItemLeaseRepository(db))
	f.core.SetActionAdmissions(repository.NewActionAdmissionRepository(db))
	return f, repository.NewStopRepository(db)
}

func TestAdmittedGitChangedParametersCannotReplay(t *testing.T) {
	f, _ := admittedGitFixture(t)
	f.core.finalizer = &observedGitFinalizer{fakeDeveloperFinalizer: f.finalizer}
	lease, won, err := f.core.deliveryRight.Acquire(context.Background(), "developer-delivery:"+f.run.RunID, "test", "test", time.Minute)
	if err != nil || !won {
		t.Fatalf("acquire: %v", err)
	}
	request := workspace.DeveloperCommitRequest{TaskID: f.task.TaskID, RunID: f.run.RunID, CommitMessage: "original", DeliveryOwner: lease.Owner, DeliveryGeneration: lease.Generation}
	f.finalizer.commitErr = &runner.HTTPError{StatusCode: 503}
	if _, err := f.core.commitAdmitted(context.Background(), request); err == nil {
		t.Fatal("expected ambiguous commit")
	}
	f.finalizer.commitErr = nil
	request.CommitMessage = "changed"
	if _, err := f.core.commitAdmitted(context.Background(), request); !errors.Is(err, repository.ErrActionOutcomeUnknown) || f.finalizer.commits != 1 {
		t.Fatalf("changed payload replayed: calls=%d err=%v", f.finalizer.commits, err)
	}
}

func TestStopBeforeCommit(t *testing.T) {
	f, stops := admittedGitFixture(t)
	ctx := context.Background()
	if _, err := stops.Stop(ctx, repository.StopTarget{TaskID: f.task.TaskID}, "admin", "", 3); err != nil {
		t.Fatal(err)
	}
	outcome, err := f.core.SubmitAgentResult(ctx, f.task, f.run, results.SubmitDeveloperResultTool, developerChangeReadyPayload())
	if err != nil || !errors.Is(outcome.Err, repository.ErrTaskInterrupted) || f.finalizer.commits != 0 || f.finalizer.pushes != 0 || f.submitter.calls != 0 {
		t.Fatalf("stopped delivery continued: commits=%d pushes=%d submits=%d outcome=%+v err=%v", f.finalizer.commits, f.finalizer.pushes, f.submitter.calls, outcome, err)
	}
}

func TestStopBetweenCommitAndPush(t *testing.T) {
	f, stops := admittedGitFixture(t)
	ctx := context.Background()
	f.finalizer.onCreate = func() {
		if _, err := stops.Stop(ctx, repository.StopTarget{TaskID: f.task.TaskID}, "admin", "after commit", 3); err != nil {
			t.Fatal(err)
		}
	}
	outcome, err := f.core.SubmitAgentResult(ctx, f.task, f.run, results.SubmitDeveloperResultTool, developerChangeReadyPayload())
	if err != nil || !errors.Is(outcome.Err, repository.ErrTaskInterrupted) || f.finalizer.commits != 1 || f.finalizer.pushes != 0 || f.submitter.calls != 0 {
		t.Fatalf("stop did not fence push: commits=%d pushes=%d submits=%d outcome=%+v err=%v", f.finalizer.commits, f.finalizer.pushes, f.submitter.calls, outcome, err)
	}
	if f.checkpoints.record.CommitSHA == "" {
		t.Fatal("stop erased the already-created commit checkpoint")
	}
	if f.task.Status != model.TaskStatusRunning {
		t.Fatal("test must retain the stale caller snapshot")
	}
}

type observedGitFinalizer struct {
	*fakeDeveloperFinalizer
	operations []string
}

func (f *observedGitFinalizer) ReadDeliveryOperation(_ context.Context, _, operationID string) (*workspaceproto.OperationRecord, error) {
	f.operations = append(f.operations, operationID)
	return nil, &runner.HTTPError{StatusCode: 404}
}

func TestAdmittedGitReplayKeepsOperationID(t *testing.T) {
	f, _ := admittedGitFixture(t)
	observed := &observedGitFinalizer{fakeDeveloperFinalizer: f.finalizer}
	f.core.finalizer = observed
	f.finalizer.pushErr = &runner.HTTPError{StatusCode: 503}
	outcome, err := f.core.SubmitAgentResult(context.Background(), f.task, f.run, results.SubmitDeveloperResultTool, developerChangeReadyPayload())
	if err != nil || outcome.Decision != ChannelFailed {
		t.Fatalf("expected ambiguous push, got %+v err=%v", outcome, err)
	}
	f.finalizer.pushErr = nil
	outcome, err = f.core.SubmitAgentResult(context.Background(), f.task, f.run, results.SubmitDeveloperResultTool, developerChangeReadyPayload())
	if err != nil || outcome.Decision != ChannelAccepted || f.finalizer.commits != 1 || f.finalizer.pushes != 2 || len(observed.operations) != 1 || observed.operations[0] != "push-"+f.run.RunID {
		t.Fatalf("Git replay failed its stable identity contract: outcome=%+v commits=%d pushes=%d reads=%v err=%v", outcome, f.finalizer.commits, f.finalizer.pushes, observed.operations, err)
	}
}
