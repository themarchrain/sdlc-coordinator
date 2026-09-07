package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

func TestPolicyRecoveryDecisionUsesHumanRequiredAtMaxAttempts(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	policy := Policy{LeaseDuration: time.Minute, MaxAttempts: 2}
	task := &model.Task{
		Status:       model.TaskStatusRunning,
		LeaseUntil:   &expired,
		AttemptCount: 2,
	}

	next, shouldRecover := policy.RecoveryStatus(task, now)
	if !shouldRecover {
		t.Fatal("expected expired running task to be recoverable")
	}
	if next != model.TaskStatusHumanRequired {
		t.Fatalf("expected human_required after max recovery attempts, got %s", next)
	}
}

func TestRecoverTaskRestoresExpiredClaimedTaskToRetryPending(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	taskRepo, runRepo := newRecoveryRepositories(t)
	orch := New(
		taskRepo,
		runRepo,
		WithClock(func() time.Time { return now }),
		WithPolicy(Policy{LeaseDuration: time.Minute, MaxAttempts: 2}),
	)
	task := &model.Task{
		TaskID:       "task-recover-claimed",
		DedupeKey:    "dedupe-recover-claimed",
		Type:         "bugfix",
		Role:         "DeveloperWorker",
		Status:       model.TaskStatusClaimed,
		LeaseUntil:   &expired,
		AttemptCount: 0,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	recovered, err := orch.RecoverTask(ctx, "task-recover-claimed")
	if err != nil {
		t.Fatalf("recover task: %v", err)
	}

	if recovered.Status != model.TaskStatusRetryPending {
		t.Fatalf("expected retry_pending, got %s", recovered.Status)
	}
	if recovered.LeaseUntil != nil {
		t.Fatalf("expected lease to be cleared, got %v", recovered.LeaseUntil)
	}
	if recovered.LastError == "" {
		t.Fatal("expected recovery to record last_error")
	}
}

func TestRecoverTaskMaterializesLeaseRecoveryIMEffectAfterSafeRetry(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	taskRepo, runRepo, effectRepo := newRecoveryRepositoriesWithEffects(t)
	runID := "run-recover-notification"
	orch := New(
		taskRepo,
		runRepo,
		WithEffectRepository(effectRepo),
		WithClock(func() time.Time { return now }),
		WithPolicy(Policy{LeaseDuration: time.Minute, MaxAttempts: 3}),
	)
	task := &model.Task{
		TaskID:       "task-recover-notification",
		DedupeKey:    "dedupe-recover-notification",
		Type:         "bugfix",
		Role:         "TesterWorker",
		Status:       model.TaskStatusRunning,
		LeaseUntil:   &expired,
		AttemptCount: 1,
		CurrentRunID: &runID,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	recovered, err := orch.RecoverTask(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("recover task: %v", err)
	}
	if recovered.Status != model.TaskStatusRetryPending {
		t.Fatalf("expected retry_pending, got %s", recovered.Status)
	}

	effect, err := effectRepo.GetByDedupeKey(ctx, "lease-recovery-im:task-recover-notification:1")
	if err != nil {
		t.Fatalf("get lease recovery effect: %v", err)
	}
	if effect.TaskID != task.TaskID || effect.RunID != runID {
		t.Fatalf("unexpected effect identity: task=%s run=%s", effect.TaskID, effect.RunID)
	}
	if effect.Kind != "send_im_message" || effect.Target != "default" || effect.Critical {
		t.Fatalf("unexpected effect delivery fields: kind=%s target=%s critical=%v", effect.Kind, effect.Target, effect.Critical)
	}
	if effect.Status != model.EffectStatusPending {
		t.Fatalf("expected pending effect, got %s", effect.Status)
	}
	for _, expected := range []string{"task-recover-notification", "run-recover-notification", "TesterWorker", "lease expired", "retry_pending"} {
		if !strings.Contains(effect.RequestJSON, expected) {
			t.Fatalf("expected effect request to contain %q, got %s", expected, effect.RequestJSON)
		}
	}
}

func TestRecoverTaskMaterializesHumanRequiredLeaseRecoveryIMEffect(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	taskRepo, runRepo, effectRepo := newRecoveryRepositoriesWithEffects(t)
	orch := New(
		taskRepo,
		runRepo,
		WithEffectRepository(effectRepo),
		WithClock(func() time.Time { return now }),
		WithPolicy(Policy{LeaseDuration: time.Minute, MaxAttempts: 1}),
	)
	task := &model.Task{
		TaskID:       "task-recover-human-notification",
		DedupeKey:    "dedupe-recover-human-notification",
		Type:         "bugfix",
		Role:         "ReviewerWorker",
		Status:       model.TaskStatusRunning,
		LeaseUntil:   &expired,
		AttemptCount: 1,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	recovered, err := orch.RecoverTask(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("recover task: %v", err)
	}
	if recovered.Status != model.TaskStatusHumanRequired {
		t.Fatalf("expected human_required, got %s", recovered.Status)
	}
	if _, err := effectRepo.GetByDedupeKey(ctx, "lease-recovery-im:task-recover-human-notification:1"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected no retry notification effect, got %v", err)
	}
	effect, err := effectRepo.GetByDedupeKey(ctx, "lease-human-required-im:task-recover-human-notification:1")
	if err != nil {
		t.Fatalf("get human-required notification effect: %v", err)
	}
	for _, expected := range []string{"task-recover-human-notification", "ReviewerWorker", "Attempt: 1", "human_required", "人工核验"} {
		if !strings.Contains(effect.RequestJSON, expected) {
			t.Fatalf("expected effect request to contain %q, got %s", expected, effect.RequestJSON)
		}
	}
	firstID := effect.ID
	orch.materializeLeaseHumanRequiredIMEffect(ctx, recovered)
	replayed, err := effectRepo.GetByDedupeKey(ctx, effect.DedupeKey)
	if err != nil {
		t.Fatalf("get replayed human-required effect: %v", err)
	}
	if replayed.ID != firstID {
		t.Fatalf("expected idempotent effect id %d, got %d", firstID, replayed.ID)
	}
}

func TestRecoverTaskLeaseRecoveryIMEffectIsIdempotent(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	taskRepo, runRepo, effectRepo := newRecoveryRepositoriesWithEffects(t)
	orch := New(
		taskRepo,
		runRepo,
		WithEffectRepository(effectRepo),
		WithClock(func() time.Time { return now }),
		WithPolicy(Policy{LeaseDuration: time.Minute, MaxAttempts: 3}),
	)
	task := &model.Task{
		TaskID:       "task-recover-idempotent-notification",
		DedupeKey:    "dedupe-recover-idempotent-notification",
		Type:         "bugfix",
		Role:         "DeveloperWorker",
		Status:       model.TaskStatusRunning,
		LeaseUntil:   &expired,
		AttemptCount: 1,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	if _, err := orch.RecoverTask(ctx, task.TaskID); err != nil {
		t.Fatalf("first recover task: %v", err)
	}
	if _, err := orch.RecoverTask(ctx, task.TaskID); err != nil {
		t.Fatalf("second recover task: %v", err)
	}

	effects, err := effectRepo.ListPending(ctx, 10)
	if err != nil {
		t.Fatalf("list pending effects: %v", err)
	}
	if len(effects) != 1 {
		t.Fatalf("expected one idempotent recovery effect, got %d", len(effects))
	}
}

func TestRecoverTaskContinuesWhenLeaseRecoveryIMEffectPersistenceFails(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	taskRepo, runRepo, effectRepo := newRecoveryRepositoriesWithEffects(t)
	orch := New(
		taskRepo,
		runRepo,
		WithEffectRepository(failingRecoveryEffectRepository{EffectRepository: effectRepo, err: errors.New("effect storage unavailable")}),
		WithClock(func() time.Time { return now }),
		WithPolicy(Policy{LeaseDuration: time.Minute, MaxAttempts: 3}),
	)
	task := &model.Task{
		TaskID:       "task-recover-effect-persistence-failure",
		DedupeKey:    "dedupe-recover-effect-persistence-failure",
		Type:         "bugfix",
		Role:         "DeveloperWorker",
		Status:       model.TaskStatusRunning,
		LeaseUntil:   &expired,
		AttemptCount: 1,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	recovered, err := orch.RecoverTask(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("recover task must not be blocked by notification persistence: %v", err)
	}
	if recovered.Status != model.TaskStatusRetryPending {
		t.Fatalf("expected retry_pending, got %s", recovered.Status)
	}
	stored, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("get recovered task: %v", err)
	}
	if stored.Status != model.TaskStatusRetryPending {
		t.Fatalf("expected persisted retry_pending, got %s", stored.Status)
	}
}

func TestRecoverTaskMovesExpiredRunningTaskToHumanRequiredAtMaxAttempts(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	taskRepo, runRepo := newRecoveryRepositories(t)
	orch := New(
		taskRepo,
		runRepo,
		WithClock(func() time.Time { return now }),
		WithPolicy(Policy{LeaseDuration: time.Minute, MaxAttempts: 2}),
	)
	task := &model.Task{
		TaskID:       "task-recover-running-max",
		DedupeKey:    "dedupe-recover-running-max",
		Type:         "bugfix",
		Role:         "DeveloperWorker",
		Status:       model.TaskStatusRunning,
		LeaseUntil:   &expired,
		AttemptCount: 2,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	recovered, err := orch.RecoverTask(ctx, "task-recover-running-max")
	if err != nil {
		t.Fatalf("recover task: %v", err)
	}

	if recovered.Status != model.TaskStatusHumanRequired {
		t.Fatalf("expected human_required, got %s", recovered.Status)
	}
	if recovered.LeaseUntil != nil {
		t.Fatalf("expected lease to be cleared, got %v", recovered.LeaseUntil)
	}
	if recovered.LastError == "" {
		t.Fatal("expected recovery to record last_error")
	}
}

func TestRecoverExpiredTasksSweepsOnlyExpiredLeases(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	active := now.Add(time.Minute)
	taskRepo, runRepo := newRecoveryRepositories(t)
	orch := New(
		taskRepo,
		runRepo,
		WithClock(func() time.Time { return now }),
		WithPolicy(Policy{LeaseDuration: time.Minute, MaxAttempts: 2}),
	)

	fixtures := []*model.Task{
		{
			TaskID:       "task-sweep-retry",
			DedupeKey:    "dedupe-sweep-retry",
			Type:         "bugfix",
			Role:         "DeveloperWorker",
			Status:       model.TaskStatusRunning,
			LeaseUntil:   &expired,
			AttemptCount: 1,
		},
		{
			TaskID:       "task-sweep-human",
			DedupeKey:    "dedupe-sweep-human",
			Type:         "bugfix",
			Role:         "DeveloperWorker",
			Status:       model.TaskStatusRunning,
			LeaseUntil:   &expired,
			AttemptCount: 2,
		},
		{
			TaskID:       "task-sweep-active",
			DedupeKey:    "dedupe-sweep-active",
			Type:         "bugfix",
			Role:         "DeveloperWorker",
			Status:       model.TaskStatusRunning,
			LeaseUntil:   &active,
			AttemptCount: 1,
		},
		{
			TaskID:       "task-sweep-pending",
			DedupeKey:    "dedupe-sweep-pending",
			Type:         "bugfix",
			Role:         "DeveloperWorker",
			Status:       model.TaskStatusPending,
			LeaseUntil:   &expired,
			AttemptCount: 1,
		},
	}
	for _, task := range fixtures {
		if err := taskRepo.Create(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", task.TaskID, err)
		}
	}

	results, err := orch.RecoverExpiredTasks(ctx, 10)
	if err != nil {
		t.Fatalf("recover expired tasks: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected two recovery results, got %d", len(results))
	}
	assertRecoveryResult(t, results, "task-sweep-retry", model.TaskStatusRunning, model.TaskStatusRetryPending, true)
	assertRecoveryResult(t, results, "task-sweep-human", model.TaskStatusRunning, model.TaskStatusHumanRequired, true)
	assertTaskStatus(t, ctx, taskRepo, "task-sweep-active", model.TaskStatusRunning)
	assertTaskStatus(t, ctx, taskRepo, "task-sweep-pending", model.TaskStatusPending)
}

func newRecoveryRepositories(t *testing.T) (*repository.TaskRepository, *repository.RunRepository) {
	t.Helper()

	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return repository.NewTaskRepository(db), repository.NewRunRepository(db)
}

func newRecoveryRepositoriesWithEffects(t *testing.T) (*repository.TaskRepository, *repository.RunRepository, *repository.EffectRepository) {
	t.Helper()

	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Effect{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return repository.NewTaskRepository(db), repository.NewRunRepository(db), repository.NewEffectRepository(db)
}

type failingRecoveryEffectRepository struct {
	EffectRepository
	err error
}

func (r failingRecoveryEffectRepository) UpsertByDedupeKey(context.Context, *model.Effect) error {
	return r.err
}

func assertRecoveryResult(
	t *testing.T,
	results []RecoveryResult,
	taskID string,
	previous model.TaskStatus,
	status model.TaskStatus,
	recovered bool,
) {
	t.Helper()

	for _, result := range results {
		if result.TaskID == taskID {
			if result.PreviousStatus != previous {
				t.Fatalf("expected previous status %s for %s, got %s", previous, taskID, result.PreviousStatus)
			}
			if result.Status != status {
				t.Fatalf("expected status %s for %s, got %s", status, taskID, result.Status)
			}
			if result.Recovered != recovered {
				t.Fatalf("expected recovered=%v for %s, got %v", recovered, taskID, result.Recovered)
			}
			return
		}
	}
	t.Fatalf("recovery result for task %s not found", taskID)
}

func assertTaskStatus(
	t *testing.T,
	ctx context.Context,
	taskRepo *repository.TaskRepository,
	taskID string,
	expected model.TaskStatus,
) {
	t.Helper()

	task, err := taskRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		t.Fatalf("get task %s: %v", taskID, err)
	}
	if task.Status != expected {
		t.Fatalf("expected task %s status %s, got %s", taskID, expected, task.Status)
	}
}
