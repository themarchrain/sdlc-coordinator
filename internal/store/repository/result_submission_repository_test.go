package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
)

func TestResultSubmissionRepositoryAcceptsResultAndFinalizesOwnershipAtomically(t *testing.T) {
	ctx := context.Background()
	repo, taskRepo, runRepo := newResultSubmissionFixture(t)
	createRunningSubmissionFixture(t, ctx, taskRepo, runRepo, "task-1", "run-1")
	lease := time.Now().UTC().Add(time.Minute)
	submission := &model.ResultSubmission{
		TaskID: "task-1", RunID: "run-1", Source: "mcp_developer", ToolName: "submit_developer_result",
		Role: "DeveloperWorker", PayloadHash: "hash-1", RawPayloadJSON: `{}`, CanonicalJSON: `{}`,
	}

	if err := repo.Accept(ctx, submission, lease); err != nil {
		t.Fatalf("accept result submission: %v", err)
	}
	if submission.SubmissionID == "" || submission.Status != model.ResultSubmissionStatusProcessing {
		t.Fatalf("unexpected submission: %#v", submission)
	}
	task, _ := taskRepo.GetByTaskID(ctx, "task-1")
	run, _ := runRepo.GetByRunID(ctx, "run-1")
	if task.Status != model.TaskStatusFinalizing || run.Status != model.RunStatusFinalizing {
		t.Fatalf("expected finalizing ownership, got task=%s run=%s", task.Status, run.Status)
	}
}

func TestResultSubmissionRepositoryRejectsSecondResultForRun(t *testing.T) {
	ctx := context.Background()
	repo, taskRepo, runRepo := newResultSubmissionFixture(t)
	createRunningSubmissionFixture(t, ctx, taskRepo, runRepo, "task-2", "run-2")
	first := &model.ResultSubmission{TaskID: "task-2", RunID: "run-2", Source: "mcp", Role: "TesterWorker", PayloadHash: "hash-a", RawPayloadJSON: `{}`, CanonicalJSON: `{}`}
	if err := repo.Accept(ctx, first, time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("accept first submission: %v", err)
	}
	second := &model.ResultSubmission{TaskID: "task-2", RunID: "run-2", Source: "mcp", Role: "TesterWorker", PayloadHash: "hash-b", RawPayloadJSON: `{}`, CanonicalJSON: `{}`}
	if err := repo.Accept(ctx, second, time.Now().Add(time.Minute)); !errors.Is(err, ErrResultAlreadyAccepted) {
		t.Fatalf("expected result already accepted, got %v", err)
	}
}

func TestResultSubmissionRepositoryRollsBackWhenRunIsNotCurrent(t *testing.T) {
	ctx := context.Background()
	repo, taskRepo, runRepo := newResultSubmissionFixture(t)
	createRunningSubmissionFixture(t, ctx, taskRepo, runRepo, "task-3", "run-current")
	if err := runRepo.Create(ctx, &model.Run{RunID: "run-stale", TaskID: "task-3", WorkerRole: "TesterWorker", Status: model.RunStatusRunning}); err != nil {
		t.Fatalf("create stale run: %v", err)
	}
	submission := &model.ResultSubmission{TaskID: "task-3", RunID: "run-stale", Source: "mcp", Role: "TesterWorker", PayloadHash: "hash", RawPayloadJSON: `{}`, CanonicalJSON: `{}`}
	if err := repo.Accept(ctx, submission, time.Now().Add(time.Minute)); !errors.Is(err, ErrResultNotAcceptable) {
		t.Fatalf("expected result not acceptable, got %v", err)
	}
	if _, err := repo.GetByRunID(ctx, "run-stale"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected rolled back submission, got %v", err)
	}
}

func TestResultSubmissionRepositoryClaimsExpiredProcessingLease(t *testing.T) {
	ctx := context.Background()
	repo, taskRepo, runRepo := newResultSubmissionFixture(t)
	createRunningSubmissionFixture(t, ctx, taskRepo, runRepo, "task-4", "run-4")
	now := time.Now().UTC()
	submission := &model.ResultSubmission{TaskID: "task-4", RunID: "run-4", Source: "mcp", Role: "TesterWorker", PayloadHash: "hash", RawPayloadJSON: `{}`, CanonicalJSON: `{}`}
	if err := repo.Accept(ctx, submission, now.Add(-time.Minute)); err != nil {
		t.Fatalf("accept submission: %v", err)
	}
	claimed, err := repo.ClaimRecoverable(ctx, submission.SubmissionID, now, now.Add(time.Minute), 3)
	if err != nil {
		t.Fatalf("claim recoverable submission: %v", err)
	}
	if claimed.AttemptCount != 2 || claimed.LeaseUntil == nil || !claimed.LeaseUntil.After(now) {
		t.Fatalf("unexpected claimed submission: %#v", claimed)
	}
}

func TestResultSubmissionRepositoryRecoversExpiredProcessingBeyondFailureLimit(t *testing.T) {
	ctx := context.Background()
	repo, taskRepo, runRepo := newResultSubmissionFixture(t)
	createRunningSubmissionFixture(t, ctx, taskRepo, runRepo, "task-processing-recovery", "run-processing-recovery")
	now := time.Now().UTC()
	submission := &model.ResultSubmission{
		TaskID: "task-processing-recovery", RunID: "run-processing-recovery", Source: "mcp", Role: "TesterWorker",
		PayloadHash: "hash", RawPayloadJSON: `{}`, CanonicalJSON: `{}`,
	}
	if err := repo.Accept(ctx, submission, now.Add(-time.Minute)); err != nil {
		t.Fatalf("accept submission: %v", err)
	}
	submission.AttemptCount = 3
	if err := repo.Update(ctx, submission); err != nil {
		t.Fatalf("set interrupted attempt count: %v", err)
	}

	recoverable, err := repo.ListRecoverable(ctx, now, 3, 10)
	if err != nil {
		t.Fatalf("list recoverable: %v", err)
	}
	if len(recoverable) != 1 || recoverable[0].SubmissionID != submission.SubmissionID {
		t.Fatalf("expired processing submission must remain recoverable, got %#v", recoverable)
	}
}

func newResultSubmissionFixture(t *testing.T) (*ResultSubmissionRepository, *TaskRepository, *RunRepository) {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.ResultSubmission{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewResultSubmissionRepository(db), NewTaskRepository(db), NewRunRepository(db)
}

func createRunningSubmissionFixture(t *testing.T, ctx context.Context, tasks *TaskRepository, runs *RunRepository, taskID, runID string) {
	t.Helper()
	currentRunID := runID
	if err := tasks.Create(ctx, &model.Task{TaskID: taskID, DedupeKey: "dedupe-" + taskID, Type: "test", Role: "TesterWorker", Status: model.TaskStatusRunning, CurrentRunID: &currentRunID}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := runs.Create(ctx, &model.Run{RunID: runID, TaskID: taskID, WorkerRole: "TesterWorker", Status: model.RunStatusRunning}); err != nil {
		t.Fatalf("create run: %v", err)
	}
}
