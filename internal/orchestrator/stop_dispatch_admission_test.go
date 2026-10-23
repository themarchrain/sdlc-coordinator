package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

func admissionIntentWorld(t *testing.T) *intentWorld {
	t.Helper()
	w := newIntentWorld(t)
	sqlDB, _ := w.db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := store.Migrate(w.db, &model.StopRequest{}, &model.ResultSubmission{}, &model.WorkItemLease{}, &model.ActionAdmission{}, &model.Writeback{}); err != nil {
		t.Fatal(err)
	}
	w.coord.SetActionAdmissions(repository.NewActionAdmissionRepository(w.db), repository.NewWorkItemLeaseRepository(w.db))
	return w
}

func TestStoppedTaskCannotDispatchOrRepin(t *testing.T) {
	w := admissionIntentWorld(t)
	ctx := context.Background()
	if _, err := repository.NewStopRepository(w.db).Stop(ctx, repository.StopTarget{TaskID: w.task.TaskID}, "admin", "closed", 3); err != nil {
		t.Fatal(err)
	}
	_, err := w.coord.Start(ctx, w.task, w.run, w.request)
	if !errors.Is(err, repository.ErrTaskInterrupted) || w.runnerA.createCalls != 0 || w.runnerB.createCalls != 0 {
		t.Fatalf("stopped task created execution: A=%d B=%d err=%v", w.runnerA.createCalls, w.runnerB.createCalls, err)
	}
	var count int64
	if err := w.db.Model(&model.AgentExecution{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("stop-first created a dispatch intent: %d err=%v", count, err)
	}
}

func TestAdmittedCreateCrashBeforeSendReplaysSameID(t *testing.T) {
	w := admissionIntentWorld(t)
	ctx := context.Background()
	hash, err := runner.RequestHash(w.request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.coord.persistIntent(ctx, w.run, w.request, "runner-a", hash); err != nil {
		t.Fatal(err)
	}
	if err := w.db.Create(&model.ActionAdmission{AdmissionID: "before-crash", TaskID: w.task.TaskID, RunID: w.run.RunID, Kind: model.ActionExecutionCreate, ActionKey: w.run.RunID, Attempt: 1, LeaseOwner: "old-owner", LeaseGeneration: 1, RequestHash: hash}).Error; err != nil {
		t.Fatal(err)
	}
	remote, err := w.coord.Start(ctx, w.task, w.run, w.request)
	if err != nil || remote == nil || w.runnerA.createCalls != 1 || w.runnerB.createCalls != 0 {
		t.Fatalf("persisted create was not recovered on original runner: A=%d B=%d err=%v", w.runnerA.createCalls, w.runnerB.createCalls, err)
	}
	current, _ := w.tasks.GetByTaskID(ctx, w.task.TaskID)
	if current.AttemptCount != w.task.AttemptCount || remote.ExecutionID != w.request.ExecutionID || remote.RunnerEndpoint != "runner-a" {
		t.Fatalf("replay changed work identity: %+v task=%+v", remote, current)
	}
}

func TestAdmittedCreateLostResponseReplaysSameID(t *testing.T) {
	w := admissionIntentWorld(t)
	ctx := context.Background()
	w.runnerA.createErr = errors.New("create response lost")
	w.runnerA.getErr = errors.New("read response lost")
	if _, err := w.coord.Start(ctx, w.task, w.run, w.request); err != nil {
		t.Fatal(err)
	}
	w.runnerA.createErr, w.runnerA.getErr = nil, nil
	if _, err := w.coord.Start(ctx, w.task, w.run, w.request); err != nil || w.runnerA.createCalls != 2 || w.runnerB.createCalls != 0 {
		t.Fatalf("lost response recovery changed target: A=%d B=%d err=%v", w.runnerA.createCalls, w.runnerB.createCalls, err)
	}
}

func TestDispatchSnapshotContainsNoCredentials(t *testing.T) {
	w := admissionIntentWorld(t)
	w.request.Provider.APIKey = "private-provider-key"
	w.request.Provider.Headers = map[string]string{"X-Private": "private-header-value"}
	w.request.ResultSubmission = &runner.ResultSubmissionChannel{EndpointURL: "https://coordinator.example/mcp", BearerToken: "private-result-token"}
	if _, err := w.coord.Start(context.Background(), w.task, w.run, w.request); err != nil {
		t.Fatal(err)
	}
	var snapshot string
	if err := w.db.Model(&model.AgentExecution{}).Select("create_spec_json").Where("run_id = ?", w.run.RunID).Scan(&snapshot).Error; err != nil {
		t.Fatalf("dispatch snapshot was not persisted: %v", err)
	}
	if snapshot == "" || !strings.Contains(snapshot, w.request.WorkspacePath) {
		t.Fatalf("missing business snapshot: %q", snapshot)
	}
	for _, secret := range []string{"private-provider-key", "private-header-value", "private-result-token"} {
		if strings.Contains(snapshot, secret) {
			t.Fatalf("dispatch snapshot persisted credential %q", secret)
		}
	}
}

func TestStoppedIdempotentActionCannotReplay(t *testing.T) {
	w := admissionIntentWorld(t)
	ctx := context.Background()
	hash, _ := runner.RequestHash(w.request)
	if _, err := w.coord.persistIntent(ctx, w.run, w.request, "runner-a", hash); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.NewStopRepository(w.db).Stop(ctx, repository.StopTarget{TaskID: w.task.TaskID}, "admin", "", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := w.coord.Start(ctx, w.task, w.run, w.request); !errors.Is(err, repository.ErrTaskInterrupted) || w.runnerA.createCalls != 0 || w.runnerB.createCalls != 0 {
		t.Fatalf("cancelled intent replayed: A=%d B=%d err=%v", w.runnerA.createCalls, w.runnerB.createCalls, err)
	}
}

func TestStopBetweenDispatchIntentAndCreate(t *testing.T) {
	w := admissionIntentWorld(t)
	realAdmissions := repository.NewActionAdmissionRepository(w.db)
	w.coord.admissions = &stopAtCreateAdmission{ActionAdmissionRepository: realAdmissions, stop: func() error {
		_, err := repository.NewStopRepository(w.db).Stop(context.Background(), repository.StopTarget{TaskID: w.task.TaskID}, "admin", "before create", 3)
		return err
	}}
	if _, err := w.coord.Start(context.Background(), w.task, w.run, w.request); !errors.Is(err, repository.ErrTaskInterrupted) || w.runnerA.createCalls != 0 {
		t.Fatalf("stop at send boundary was reported as dispatched: calls=%d err=%v", w.runnerA.createCalls, err)
	}
}

type stopAtCreateAdmission struct {
	*repository.ActionAdmissionRepository
	stop func() error
}

func (a *stopAtCreateAdmission) AdmitAction(ctx context.Context, request ActionAdmissionRequest) (*model.ActionAdmission, error) {
	if err := a.stop(); err != nil {
		return nil, err
	}
	return a.ActionAdmissionRepository.AdmitAction(ctx, request)
}

func TestDispatchReplayNeverUsesFallbackClient(t *testing.T) {
	w := admissionIntentWorld(t)
	hash, _ := runner.RequestHash(w.request)
	if _, err := w.coord.persistIntent(context.Background(), w.run, w.request, "runner-a", hash); err != nil {
		t.Fatal(err)
	}
	delete(w.router.byEndpt, "runner-a")
	w.coord.runner = w.runnerB
	if _, err := w.coord.Start(context.Background(), w.task, w.run, w.request); err == nil || w.runnerB.createCalls != 0 {
		t.Fatalf("missing pinned client fell back to another runner: B=%d err=%v", w.runnerB.createCalls, err)
	}
}

func TestDispatchReplayRefusesCredentialsForChangedHost(t *testing.T) {
	w := admissionIntentWorld(t)
	w.request.Provider.BaseURL = "https://original.example"
	hash, _ := runner.RequestHash(w.request)
	if _, err := w.coord.persistIntent(context.Background(), w.run, w.request, "runner-a", hash); err != nil {
		t.Fatal(err)
	}
	w.request.Provider.BaseURL, w.request.Provider.APIKey = "https://replacement.example", "new-key"
	if _, err := w.coord.Start(context.Background(), w.task, w.run, w.request); err == nil || w.runnerA.createCalls != 0 {
		t.Fatalf("new-host credentials leaked to old host: calls=%d err=%v", w.runnerA.createCalls, err)
	}
}

func TestDispatchLegacyIntentCannotReplayWithoutSnapshot(t *testing.T) {
	w := admissionIntentWorld(t)
	hash, _ := runner.RequestHash(w.request)
	w.insertIntent(t, "runner-a", hash)
	if _, err := w.coord.Start(context.Background(), w.task, w.run, w.request); err == nil || w.runnerA.createCalls != 0 {
		t.Fatalf("legacy intent guessed a create: calls=%d err=%v", w.runnerA.createCalls, err)
	}
}

func TestDispatchObservationRecoversWithFreshCredentials(t *testing.T) {
	w := admissionIntentWorld(t)
	w.request.Provider.Headers = map[string]string{"X-Key": "old-header"}
	hash, _ := runner.RequestHash(w.request)
	if _, err := w.coord.persistIntent(context.Background(), w.run, w.request, "runner-a", hash); err != nil {
		t.Fatal(err)
	}
	w.coord.dispatchCredentials = func(_ context.Context, snapshot runner.ProviderConfig) (runner.ProviderConfig, error) {
		current := w.request.Provider
		current.APIKey, current.Headers = "rotated-key", map[string]string{"x-key": "rotated-header"}
		return RefreshDispatchCredentials(current, snapshot)
	}
	w.runnerA.onCreate = func(request runner.CreateExecutionRequest) {
		if request.Provider.APIKey != "rotated-key" || request.Provider.Headers["X-Key"] != "rotated-header" {
			t.Errorf("stale credentials: %+v", request.Provider)
		}
	}
	if _, err := w.coord.Observe(context.Background(), w.run.RunID); err != nil || w.runnerA.createCalls != 1 {
		t.Fatalf("observation did not recover original create: calls=%d err=%v", w.runnerA.createCalls, err)
	}
}

func TestWorkerActionAdmissionWiring(t *testing.T) {
	w := admissionIntentWorld(t)
	admissions, leases := repository.NewActionAdmissionRepository(w.db), repository.NewWorkItemLeaseRepository(w.db)
	developer := &DeveloperRunnerCoordinator{core: w.coord, channel: NewResultChannelCoordinator(nil, nil, ReviewerRunnerSettings{}, "test")}
	reviewer := &ReviewerRunnerCoordinator{core: NewAgentRunnerCoordinator(w.tasks, w.runs, w.executions, w.runnerA), channel: NewResultChannelCoordinator(nil, nil, ReviewerRunnerSettings{}, "test")}
	tester := &TesterRunnerCoordinator{core: NewAgentRunnerCoordinator(w.tasks, w.runs, w.executions, w.runnerA), channel: NewResultChannelCoordinator(nil, nil, ReviewerRunnerSettings{}, "test")}
	developer.SetActionAdmissions(admissions, leases, w.request.Provider)
	reviewer.SetActionAdmissions(admissions, leases, w.request.Provider)
	tester.SetActionAdmissions(admissions, leases, w.request.Provider)
	for _, core := range []*AgentRunnerCoordinator{developer.core, reviewer.core, tester.core} {
		if core.admissions != admissions || core.leases != leases || core.dispatchCredentials == nil {
			t.Fatal("worker admission or recovery wiring missing")
		}
	}
	if developer.channel.admissions != admissions || developer.channel.deliveryRight != leases {
		t.Fatal("text delivery bypasses admission")
	}
}
