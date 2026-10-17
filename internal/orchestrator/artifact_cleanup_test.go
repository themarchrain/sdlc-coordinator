package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/store/storetest"
)

type recordingArtifactStore struct {
	deleted []string
	err     error
}

func (s *recordingArtifactStore) Delete(_ context.Context, executionID string) error {
	s.deleted = append(s.deleted, executionID)
	return s.err
}

func TestTickPurgesFullySucceededExpiredArtifacts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	fixture := newArtifactCleanupFixture(t, root)
	completed := fixture.now.Add(-8 * 24 * time.Hour)
	seedFullySucceededRun(t, ctx, fixture, root, "run-clean", completed)

	result, err := fixture.orch.Tick(ctx, TickRequest{})
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(result.CleanedArtifacts) != 1 || !result.CleanedArtifacts[0].Purged || result.CleanedArtifacts[0].RunID != "run-clean" {
		t.Fatalf("expected purged run, got %#v", result.CleanedArtifacts)
	}
	if _, err := os.Stat(filepath.Join(root, "runs", "run-clean")); !os.IsNotExist(err) {
		t.Fatalf("workspace directory still exists: %v", err)
	}
	if len(fixture.store.deleted) != 1 || fixture.store.deleted[0] != "run-clean" {
		t.Fatalf("runner artifacts not deleted: %#v", fixture.store.deleted)
	}
	run, err := fixture.runs.GetByRunID(ctx, "run-clean")
	if err != nil || run.ArtifactsPurgedAt == nil {
		t.Fatalf("purged marker missing: run=%#v err=%v", run, err)
	}

	fixture.store.deleted = nil
	repeated, err := fixture.orch.Tick(ctx, TickRequest{})
	if err != nil {
		t.Fatalf("repeat tick: %v", err)
	}
	if len(repeated.CleanedArtifacts) != 0 {
		t.Fatalf("purged run was selected again: %#v", repeated.CleanedArtifacts)
	}
}

func TestTickSkipsAnomalousRunsDuringArtifactCleanup(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	fixture := newArtifactCleanupFixture(t, root)
	completed := fixture.now.Add(-8 * 24 * time.Hour)

	failedRun := seedFullySucceededRun(t, ctx, fixture, root, "run-failed-writeback", completed)
	if err := fixture.writebacks.Create(ctx, &model.Writeback{
		TaskID: failedRun.TaskID, RunID: failedRun.RunID, DedupeKey: "wb-failed", Kind: "create_merge_request",
		ProjectID: "group/project", Status: model.WritebackStatusFailed, ErrorMessage: "gitlab down",
	}); err != nil {
		t.Fatalf("create failed writeback: %v", err)
	}
	pendingRun := seedFullySucceededRun(t, ctx, fixture, root, "run-pending-effect", completed)
	if err := fixture.effects.Create(ctx, &model.Effect{
		TaskID: pendingRun.TaskID, RunID: pendingRun.RunID, DedupeKey: "im-pending", Kind: "send_im_message",
		Target: "team", Status: model.EffectStatusPending,
	}); err != nil {
		t.Fatalf("create pending effect: %v", err)
	}

	result, err := fixture.orch.Tick(ctx, TickRequest{})
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(result.CleanedArtifacts) != 0 {
		t.Fatalf("ineligible runs entered the cleanup batch: %#v", result.CleanedArtifacts)
	}
	for _, item := range result.CleanedArtifacts {
		if item.Purged {
			t.Fatalf("anomalous run was purged: %#v", item)
		}
	}
	if len(fixture.store.deleted) != 0 {
		t.Fatalf("runner delete called for anomalies: %#v", fixture.store.deleted)
	}
}

func TestTickArtifactCleanupSkipsIneligibleBatchHead(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	fixture := newArtifactCleanupFixture(t, root)
	old := fixture.now.Add(-9 * 24 * time.Hour)
	for i := 0; i < artifactCleanupBatchSize; i++ {
		id := fmt.Sprintf("run-blocked-%02d", i)
		run := seedFullySucceededRun(t, ctx, fixture, root, id, old)
		if err := fixture.writebacks.Create(ctx, &model.Writeback{
			TaskID: run.TaskID, RunID: run.RunID, DedupeKey: "wb-" + id,
			Kind: "create_merge_request", ProjectID: "group/project", Status: model.WritebackStatusFailed,
		}); err != nil {
			t.Fatal(err)
		}
	}
	seedFullySucceededRun(t, ctx, fixture, root, "run-eligible", fixture.now.Add(-8*24*time.Hour))
	result, err := fixture.orch.Tick(ctx, TickRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.CleanedArtifacts) != 1 || result.CleanedArtifacts[0].RunID != "run-eligible" || !result.CleanedArtifacts[0].Purged {
		t.Fatalf("eligible run starved by ineligible batch head: %+v", result.CleanedArtifacts)
	}
}

func TestTickPurgesRunWithDeliveryUnverifiedEffect(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	fixture := newArtifactCleanupFixture(t, root)
	completed := fixture.now.Add(-8 * 24 * time.Hour)
	run := seedFullySucceededRun(t, ctx, fixture, root, "run-unverified-effect", completed)
	// 投递未核验是终态且无人工出路:不能像 pending/failed 那样把 run 产物
	// 永久留在清理候选里;送达凭据与原因元数据都在效果记录中,不依赖产物。
	if err := fixture.effects.Create(ctx, &model.Effect{
		TaskID: run.TaskID, RunID: run.RunID, DedupeKey: "im-unverified", Kind: "send_im_message",
		Target:       "team",
		Status:       model.EffectStatusDeliveryUnverified,
		ErrorMessage: "effect delivery unverified: receipt exceeds limit 65536 (http 200, read at least 65537)",
	}); err != nil {
		t.Fatalf("create unverified effect: %v", err)
	}

	result, err := fixture.orch.Tick(ctx, TickRequest{})
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(result.CleanedArtifacts) != 1 || !result.CleanedArtifacts[0].Purged || result.CleanedArtifacts[0].RunID != "run-unverified-effect" {
		t.Fatalf("expected purged run with unverified effect, got %#v", result.CleanedArtifacts)
	}
}

func TestTickKeepsGoingWhenArtifactDeleteFails(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	fixture := newArtifactCleanupFixture(t, root)
	fixture.store.err = errors.New("runner unavailable")
	completed := fixture.now.Add(-8 * 24 * time.Hour)
	seedFullySucceededRun(t, ctx, fixture, root, "run-delete-fail", completed)

	result, err := fixture.orch.Tick(ctx, TickRequest{DispatchLimit: 1})
	if err != nil {
		t.Fatalf("tick must not fail: %v", err)
	}
	if len(result.CleanedArtifacts) != 1 || result.CleanedArtifacts[0].Purged || result.CleanedArtifacts[0].Error == "" {
		t.Fatalf("expected recorded cleanup error, got %#v", result.CleanedArtifacts)
	}
	run, err := fixture.runs.GetByRunID(ctx, "run-delete-fail")
	if err != nil || run.ArtifactsPurgedAt != nil {
		t.Fatalf("failed cleanup must not stamp purged: run=%#v err=%v", run, err)
	}
	if run.ArtifactsPurgeStartedAt == nil {
		t.Fatal("failed cleanup must retain its claim until retry window expires")
	}
}

func TestTickArtifactCleanupRotatesPastFailedBatch(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	fixture := newArtifactCleanupFixture(t, root)
	old := fixture.now.Add(-9 * 24 * time.Hour)
	for i := 0; i < artifactCleanupBatchSize; i++ {
		seedFullySucceededRun(t, ctx, fixture, root, fmt.Sprintf("run-delete-fail-%02d", i), old)
	}
	seedFullySucceededRun(t, ctx, fixture, root, "run-behind-failures", fixture.now.Add(-8*24*time.Hour))
	fixture.store.err = errors.New("runner unavailable")
	first, err := fixture.orch.Tick(ctx, TickRequest{})
	if err != nil || len(first.CleanedArtifacts) != artifactCleanupBatchSize {
		t.Fatalf("first cleanup batch = %+v, err=%v", first, err)
	}
	fixture.orch.now = func() time.Time { return fixture.now.Add(artifactPurgeLeaseWindow + time.Minute) }
	fixture.store.err = nil
	second, err := fixture.orch.Tick(ctx, TickRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.CleanedArtifacts) != artifactCleanupBatchSize || second.CleanedArtifacts[0].RunID != "run-behind-failures" || !second.CleanedArtifacts[0].Purged {
		t.Fatalf("later run starved by failed batch: %+v", second.CleanedArtifacts)
	}
}

type artifactCleanupFixture struct {
	now         time.Time
	orch        *Orchestrator
	tasks       *repository.TaskRepository
	runs        *repository.RunRepository
	executions  *repository.AgentExecutionRepository
	submissions *repository.ResultSubmissionRepository
	writebacks  *repository.WritebackRepository
	effects     *repository.EffectRepository
	store       *recordingArtifactStore
}

func newArtifactCleanupFixture(t *testing.T, workspaceRoot string) artifactCleanupFixture {
	t.Helper()
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.ResultSubmission{}, &model.Writeback{}, &model.Effect{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	storeRec := &recordingArtifactStore{}
	tasks := repository.NewTaskRepository(db)
	runs := repository.NewRunRepository(db)
	executions := repository.NewAgentExecutionRepository(db)
	submissions := repository.NewResultSubmissionRepository(db)
	writebacks := repository.NewWritebackRepository(db)
	effects := repository.NewEffectRepository(db)
	orch := New(tasks, runs,
		WithResultSubmissionRepository(submissions),
		WithWritebackRepository(writebacks),
		WithEffectRepository(effects),
		WithAgentExecutionLookup(executions),
		WithRunnerArtifactStore(storeRec),
		WithReviewerRunner(&fakeReviewerRunnerCoordinator{}, ReviewerRunnerSettings{WorkspaceRoot: workspaceRoot}),
		WithClock(func() time.Time { return now }),
	)
	return artifactCleanupFixture{
		now: now, orch: orch, tasks: tasks, runs: runs, executions: executions,
		submissions: submissions, writebacks: writebacks, effects: effects, store: storeRec,
	}
}

func seedFullySucceededRun(t *testing.T, ctx context.Context, fixture artifactCleanupFixture, workspaceRoot, runID string, completed time.Time) *model.Run {
	t.Helper()
	taskID := "task-" + runID
	task := &model.Task{
		TaskID: taskID, DedupeKey: "dedupe-" + runID, Type: "review_mr", Role: "ReviewerWorker",
		Status: model.TaskStatusSucceeded, CurrentRunID: &runID,
	}
	if err := fixture.tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	run := &model.Run{
		RunID: runID, TaskID: taskID, WorkerRole: "ReviewerWorker", Status: model.RunStatusSucceeded,
		ExternalSystem: "agent-runner", ExternalID: runID, CompletedAt: &completed,
	}
	if err := fixture.runs.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := fixture.executions.Create(ctx, &model.AgentExecution{
		RunID: runID, ExecutionID: runID, Provider: "codex-cli",
		Status: model.AgentExecutionStatusSucceeded, RequestHash: "hash-" + runID, ResultHash: "result-" + runID,
		FinishedAt: &completed,
	}); err != nil {
		t.Fatalf("create execution: %v", err)
	}
	completedCopy := completed
	if err := fixture.submissions.Update(ctx, &model.ResultSubmission{
		TaskID: taskID, RunID: runID, Source: "agent-runner", Role: "ReviewerWorker",
		PayloadHash: "payload-" + runID, RawPayloadJSON: `{}`, CanonicalJSON: `{}`,
		Status: model.ResultSubmissionStatusSucceeded, CompletedAt: &completedCopy,
	}); err != nil {
		t.Fatalf("create submission: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "runs", runID, "repo"), 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	return run
}

// TestCleanupClaimRecoversAfterHolderExit 审查 #9 的回归：抢占哨兵写入后
// 持有者进程退出（未回滚哨兵），超过有效期的遗留哨兵必须能被后续批次接管；
// 有效期内不得被抢（防双删）。锁定未完成清理不会因崩溃永久失去候选资格。
func TestCleanupClaimRecoversAfterHolderExit(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, admin, "purge_recover")
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := store.Migrate(db, &model.Run{}, &model.AgentExecution{}, &model.ResultSubmission{}, &model.Writeback{}, &model.Effect{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	runs := repository.NewRunRepository(db)
	now := time.Now().UTC().Truncate(time.Microsecond)
	completedAt := now.Add(-8 * 24 * time.Hour)
	run := &model.Run{
		RunID: "run-purge-recover", TaskID: "task-purge-recover", WorkerRole: "DeveloperWorker",
		Status: model.RunStatusSucceeded, ExternalSystem: "agent-runner", CompletedAt: &completedAt,
	}
	if err := runs.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := db.Create(&model.AgentExecution{RunID: run.RunID, ExecutionID: run.RunID, Provider: "codex-cli", Status: model.AgentExecutionStatusSucceeded, ResultHash: "result"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ResultSubmission{TaskID: run.TaskID, RunID: run.RunID, Source: "agent-runner", Role: run.WorkerRole, Status: model.ResultSubmissionStatusSucceeded, PayloadHash: "payload", RawPayloadJSON: "{}", CanonicalJSON: "{}"}).Error; err != nil {
		t.Fatal(err)
	}

	// 持有者抢占（哨兵 = 当下）。
	if applied, err := runs.MarkArtifactsPurgingIfUnclean(ctx, run.RunID, "claim-a", now, now); err != nil || !applied {
		t.Fatalf("initial claim: applied=%v err=%v", applied, err)
	}
	// 有效期内的哨兵不可被抢（防双删）。
	fresh, err := runs.MarkArtifactsPurgingIfUnclean(ctx, run.RunID, "claim-b", now.Add(time.Minute), now.Add(-30*time.Minute))
	if err != nil {
		t.Fatalf("fresh claim: %v", err)
	}
	if fresh {
		t.Fatal("a fresh purge claim must not be stolen by another instance")
	}
	// 持有者进程退出（未回滚哨兵）：把哨兵推回 1 小时前模拟崩溃遗留，超过
	// 有效期（30 分钟）后可被后续批次接管。
	if err := db.Model(&model.Run{}).Where("run_id = ?", run.RunID).
		Update("artifacts_purge_started_at", now.Add(-time.Hour)).Error; err != nil {
		t.Fatalf("age purge sentinel: %v", err)
	}
	candidates, err := runs.ListSucceededForArtifactCleanup(ctx, now.Add(-7*24*time.Hour), now.Add(-30*time.Minute), 10)
	if err != nil || len(candidates) != 1 || candidates[0].RunID != run.RunID {
		t.Fatalf("stale claim must return to cleanup candidates: runs=%+v err=%v", candidates, err)
	}
	applied, err := runs.MarkArtifactsPurgingIfUnclean(ctx, run.RunID, "claim-b", now, now.Add(-30*time.Minute))
	if err != nil || !applied {
		t.Fatalf("stale claim must be recoverable: applied=%v err=%v", applied, err)
	}
	if completed, err := runs.CompleteArtifactsPurging(ctx, run.RunID, "claim-a", now); err != nil || completed {
		t.Fatalf("old holder must not complete new claim: completed=%v err=%v", completed, err)
	}
	if completed, err := runs.CompleteArtifactsPurging(ctx, run.RunID, "claim-b", now); err != nil || !completed {
		t.Fatalf("new holder complete: completed=%v err=%v", completed, err)
	}
	candidates, err = runs.ListSucceededForArtifactCleanup(ctx, now.Add(-7*24*time.Hour), now.Add(time.Hour), 10)
	if err != nil || len(candidates) != 0 {
		t.Fatalf("completed purge must never return to cleanup candidates: runs=%+v err=%v", candidates, err)
	}
}
