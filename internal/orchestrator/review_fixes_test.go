package orchestrator

import (
	"context"
	"errors"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
	"testing"
)

type stopBeforeStartRunner struct {
	fakeDeveloperRunnerCoordinator
	before func()
}

func (r *stopBeforeStartRunner) Start(context.Context, DeveloperRunnerStartRequest) (*model.AgentExecution, error) {
	r.before()
	return nil, repository.ErrTaskInterrupted
}
func TestStopBeforeRunnerPinHasNoInventedRemoteTarget(t *testing.T) {
	db, o, task, _ := externalAdmissionFixture(t)
	if err := db.Model(task).Updates(map[string]any{"status": model.TaskStatusPending, "current_run_id": nil, "role": "DeveloperWorker", "type": "develop_from_issue", "target_repo": "group/project", "delivery_branch": "agent/fix", "context_json": `{"project":{"path":"group/project"},"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"main","access":"writable"}]}`}).Error; err != nil {
		t.Fatal(err)
	}
	r := &stopBeforeStartRunner{before: func() {
		stopped, err := o.StopTask(context.Background(), task.TaskID, StopActor{UserID: "admin"}, "")
		if err != nil {
			t.Fatal(err)
		}
		if stopped.ConfirmationKind != model.StopConfirmationNoRemoteTarget {
			t.Fatalf("invented remote target before Runner start: %+v", stopped)
		}
	}}
	WithDeveloperRunner(r, ReviewerRunnerSettings{ResolveRepository: stubRunnerRepository()})(o)
	if _, err := o.DispatchDeveloperTask(context.Background(), task.TaskID); !errors.Is(err, repository.ErrTaskInterrupted) {
		t.Fatalf("dispatch=%v", err)
	}
}

func TestStoppedPendingWritebacksSettleWithoutSending(t *testing.T) {
	db, o, task, run := externalAdmissionFixture(t)
	for _, key := range []string{"unsent", "admitted"} {
		if err := db.Create(&model.Writeback{TaskID: task.TaskID, RunID: run.RunID, DedupeKey: key, Kind: "create_issue", Status: model.WritebackStatusPending}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&model.ActionAdmission{AdmissionID: "permit", TaskID: task.TaskID, RunID: run.RunID, Kind: model.ActionWriteback, ActionKey: "admitted", Attempt: 1, LeaseOwner: "sender", LeaseGeneration: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := o.StopTask(context.Background(), task.TaskID, StopActor{UserID: "admin"}, ""); err != nil {
		t.Fatal(err)
	}
	if err := o.ReconcileStops(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]model.WritebackStatus{"unsent": model.WritebackStatusSkipped, "admitted": model.WritebackStatusUnknown} {
		got, err := o.writebacks.GetByDedupeKey(context.Background(), key)
		if err != nil || got.Status != want {
			t.Fatalf("%s: %+v want %s err=%v", key, got, want, err)
		}
	}
}

func TestEffectAdmissionSystemErrorRemainsPending(t *testing.T) {
	db, o, task, run := externalAdmissionFixture(t)
	effect := admissionEffect(t, db, task, run, "db-fault", model.EffectStatusPending)
	fault := errors.New("admission database temporarily unavailable")
	o.actionAdmissions = admissionBoundaryFunc(func(context.Context, ActionAdmissionRequest) (*model.ActionAdmission, error) { return nil, fault })
	executor := &stopBoundaryEffectExecutor{}
	o.effectExecutor = executor
	_, err := o.ExecutePendingEffects(context.Background(), 20)
	got, readErr := o.effects.GetByDedupeKey(context.Background(), effect.DedupeKey)
	if !errors.Is(err, fault) || readErr != nil || got.Status != model.EffectStatusPending || executor.calls != 0 {
		t.Fatalf("lost unsent system failure: %+v calls=%d err=%v read=%v", got, executor.calls, err, readErr)
	}
}
func TestEffectFromHistoricalRunSkipsWithoutSending(t *testing.T) {
	db, o, task, run := externalAdmissionFixture(t)
	effect := admissionEffect(t, db, task, run, "historical", model.EffectStatusPending)
	if err := db.Model(task).Update("current_run_id", nil).Error; err != nil {
		t.Fatal(err)
	}
	executor := &stopBoundaryEffectExecutor{}
	o.effectExecutor = executor
	if _, err := o.ExecutePendingEffects(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	got, err := o.effects.GetByDedupeKey(context.Background(), effect.DedupeKey)
	if err != nil || got.Status != model.EffectStatusSkipped || got.ErrorMessage != "run_not_current" || executor.calls != 0 {
		t.Fatalf("historical effect=%+v calls=%d err=%v", got, executor.calls, err)
	}
}
