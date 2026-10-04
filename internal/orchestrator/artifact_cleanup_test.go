package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
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
	if len(result.CleanedArtifacts) != 2 {
		t.Fatalf("expected both candidates to be inspected, got %#v", result.CleanedArtifacts)
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
