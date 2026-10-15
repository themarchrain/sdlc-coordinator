package orchestrator

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/store/storetest"

	"gorm.io/gorm"
)

// openSharedOrchestrators 打开同一 MySQL 库的两个编排器（模拟两台协调器）。
func openSharedOrchestrators(t *testing.T, dsn string, options func(*gorm.DB) []Option) []*Orchestrator {
	t.Helper()
	orchs := make([]*Orchestrator, 2)
	for i := range orchs {
		db, err := store.Open(dsn)
		if err != nil {
			t.Fatalf("open instance %d: %v", i, err)
		}
		t.Cleanup(func() {
			if sqlDB, err := db.DB(); err == nil {
				_ = sqlDB.Close()
			}
		})
		var opts []Option
		if options != nil {
			opts = options(db)
		}
		orchs[i] = New(
			repository.NewTaskRepository(db),
			repository.NewRunRepository(db),
			opts...,
		)
	}
	return orchs
}

// TestRecoverExpiredDualInstanceSingleWinner 双实例同时回收同一租约过期任务：
// 恰好一个实例推进状态机,attempt_count 恰好 +1,败者不覆盖胜者写入。
func TestRecoverExpiredDualInstanceSingleWinner(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, admin, "recover_race")

	seed, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open seed: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := seed.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := store.Migrate(seed, &model.Task{}, &model.Run{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orchs := openSharedOrchestrators(t, dsn, nil)

	ctx := context.Background()
	expired := time.Now().Add(-time.Hour)
	if err := repository.NewTaskRepository(seed).Create(ctx, &model.Task{
		TaskID: "task-recover-race", DedupeKey: "dedupe-recover-race", Type: "bugfix", Role: "TesterWorker",
		Status: model.TaskStatusRunning, LeaseUntil: &expired, AttemptCount: 1,
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	results := make([]*model.Task, 2)
	errs := make([]error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range orchs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = orchs[i].RecoverTask(ctx, "task-recover-race")
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("instance %d: %v", i, err)
		}
	}
	if results[0].Status != model.TaskStatusRetryPending || results[1].Status != model.TaskStatusRetryPending {
		t.Fatalf("both instances must observe the recovered state, got %s / %s", results[0].Status, results[1].Status)
	}

	final, err := repository.NewTaskRepository(seed).GetByTaskID(ctx, "task-recover-race")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if final.Status != model.TaskStatusRetryPending {
		t.Fatalf("status = %s, want retry_pending", final.Status)
	}
	if final.AttemptCount != 2 {
		t.Fatalf("attempt_count = %d, want 2 (exactly one atomic increment)", final.AttemptCount)
	}
	if final.LeaseUntil != nil {
		t.Fatalf("lease must be cleared, got %v", final.LeaseUntil)
	}
}

// TestRestoreWaitingParentGuardedSingleWinner 双实例同时解除同一父任务的依赖
// 链：恰好一次从 waiting_upstream 到 pending 的推进,attempt_count 不变。
func TestRestoreWaitingParentGuardedSingleWinner(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, admin, "parent_restore")

	seed, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open seed: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := seed.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := store.Migrate(seed, &model.Task{}, &model.Dependency{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	taskRepo := repository.NewTaskRepository(seed)

	ctx := context.Background()
	if err := taskRepo.Create(ctx, &model.Task{
		TaskID: "task-parent-race", DedupeKey: "dedupe-parent-race", Type: "bugfix", Role: "DeveloperWorker",
		Status: model.TaskStatusWaitingUpstream, AttemptCount: 2,
	}); err != nil {
		t.Fatalf("seed parent: %v", err)
	}

	repos := []*repository.TaskRepository{
		repository.NewTaskRepository(mustOpenShared(t, dsn)),
		repository.NewTaskRepository(mustOpenShared(t, dsn)),
	}
	start := make(chan struct{})
	applied := make([]bool, 2)
	var wg sync.WaitGroup
	for i := range repos {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			applied[i], _ = repos[i].RestoreWaitingParentGuarded(ctx, "task-parent-race")
		}(i)
	}
	close(start)
	wg.Wait()

	wins := 0
	for _, ok := range applied {
		if ok {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("winners = %d, want exactly 1 (%v)", wins, applied)
	}
	final, err := taskRepo.GetByTaskID(ctx, "task-parent-race")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if final.Status != model.TaskStatusPending || final.AttemptCount != 2 {
		t.Fatalf("parent = %s/%d, want pending with attempt untouched", final.Status, final.AttemptCount)
	}
}

// countingWorkspaceCleaner 统计远端工作区删除调用的桩。
type countingWorkspaceCleaner struct {
	calls atomic.Int32
}

func (c *countingWorkspaceCleaner) RemoveRunWorkspace(ctx context.Context, taskID, runID string) error {
	c.calls.Add(1)
	return nil
}

// TestArtifactCleanupDualInstanceSinglePurge 双实例同时进入清理批次：清理
// 归属栅栏保证远端工作区删除恰好执行一次,胜者落 purge 终态。
func TestArtifactCleanupDualInstanceSinglePurge(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, admin, "cleanup_race")

	seed, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open seed: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := seed.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := store.Migrate(seed, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.ResultSubmission{}, &model.Artifact{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctx := context.Background()
	completed := time.Now().Add(-8 * 24 * time.Hour)
	runRepo := repository.NewRunRepository(seed)
	if err := runRepo.Create(ctx, &model.Run{
		RunID: "run-cleanup-race", TaskID: "task-cleanup-race", WorkerRole: "DeveloperWorker",
		Status: model.RunStatusSucceeded, CompletedAt: &completed, ExternalSystem: "agent-runner",
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	// run 的产物链全绿(执行成功+回执成功),满足清理前置条件。
	agentExecRepo := repository.NewAgentExecutionRepository(seed)
	if err := agentExecRepo.Create(ctx, &model.AgentExecution{
		ExecutionID: "exec-cleanup-race", RunID: "run-cleanup-race",
		Status: model.AgentExecutionStatusSucceeded, ResultHash: "hash-cleanup",
	}); err != nil {
		t.Fatalf("seed execution: %v", err)
	}
	if err := repository.NewResultSubmissionRepository(seed).Update(ctx, &model.ResultSubmission{
		TaskID: "task-cleanup-race", RunID: "run-cleanup-race", Source: "agent-runner", Role: "DeveloperWorker",
		PayloadHash: "payload-race", RawPayloadJSON: `{}`, CanonicalJSON: `{}`,
		Status: model.ResultSubmissionStatusSucceeded,
	}); err != nil {
		t.Fatalf("seed submission: %v", err)
	}

	cleaner := &countingWorkspaceCleaner{}
	orchs := openSharedOrchestrators(t, dsn, func(db *gorm.DB) []Option {
		return []Option{
			WithAgentExecutionLookup(repository.NewAgentExecutionRepository(db)),
			WithResultSubmissionRepository(repository.NewResultSubmissionRepository(db)),
			WithRunnerArtifactStore(&recordingArtifactStore{}),
			WithRunWorkspaceCleaner(cleaner),
			WithReviewerRunner(&fakeReviewerRunnerCoordinator{}, ReviewerRunnerSettings{WorkspaceRoot: "D:/tmp/ws"}),
		}
	})

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range orchs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			orchs[i].cleanupExpiredArtifacts(ctx)
		}(i)
	}
	close(start)
	wg.Wait()

	if got := cleaner.calls.Load(); got != 1 {
		t.Fatalf("RemoveRunWorkspace called %d times across instances, want exactly 1", got)
	}
	final, err := runRepo.GetByRunID(ctx, "run-cleanup-race")
	if err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if final.ArtifactsPurgedAt == nil {
		t.Fatal("winner must persist the purge timestamp")
	}
}

func mustOpenShared(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open shared: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
