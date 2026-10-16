package repository

import (
	"context"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
)

func TestRunRepositoryCreateAndLookup(t *testing.T) {
	db, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := store.Migrate(db, &model.Run{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewRunRepository(db)
	ctx := context.Background()

	run := &model.Run{
		TaskID:     "task-001",
		WorkerRole: "DeveloperWorker",
		Status:     model.RunStatusRunning,
		InputJSON:  `{"objective":"repair upstream failure"}`,
	}

	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if run.RunID == "" {
		t.Fatal("expected run id to be generated")
	}

	loaded, err := repo.GetByRunID(ctx, run.RunID)
	if err != nil {
		t.Fatalf("get by run id: %v", err)
	}
	if loaded.RunID != run.RunID {
		t.Fatalf("expected run id %s, got %s", run.RunID, loaded.RunID)
	}
	if loaded.WorkerRole != "DeveloperWorker" {
		t.Fatalf("expected worker role DeveloperWorker, got %s", loaded.WorkerRole)
	}
}

func TestRunRepositoryPersistsExternalExecutionIdentity(t *testing.T) {
	db, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := store.Migrate(db, &model.Run{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewRunRepository(db)
	ctx := context.Background()

	run := &model.Run{
		RunID:          "run-external-1",
		TaskID:         "task-external-1",
		WorkerRole:     "DeveloperWorker",
		Status:         model.RunStatusRunning,
		ExternalSystem: "agent-runner",
		ExternalID:     "conversation-123",
		ExternalURL:    "http://agent-runner.example/executions/conversation-123",
	}

	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	got, err := repo.GetByRunID(ctx, "run-external-1")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.ExternalSystem != "agent-runner" {
		t.Fatalf("expected external system, got %q", got.ExternalSystem)
	}
	if got.ExternalID != "conversation-123" {
		t.Fatalf("expected external id, got %q", got.ExternalID)
	}
	if got.ExternalURL != "http://agent-runner.example/executions/conversation-123" {
		t.Fatalf("expected external url, got %q", got.ExternalURL)
	}
}

func TestRunRepositoryListRunning(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := store.Migrate(db, &model.Run{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewRunRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	older := now.Add(-2 * time.Minute)
	newer := now.Add(-time.Minute)

	runs := []*model.Run{
		{
			RunID:      "run-succeeded",
			TaskID:     "task-succeeded",
			WorkerRole: "DeveloperWorker",
			Status:     model.RunStatusSucceeded,
			UpdatedAt:  older,
		},
		{
			RunID:      "run-running-newer",
			TaskID:     "task-running-newer",
			WorkerRole: "DeveloperWorker",
			Status:     model.RunStatusRunning,
			UpdatedAt:  newer,
		},
		{
			RunID:      "run-running-older",
			TaskID:     "task-running-older",
			WorkerRole: "TesterWorker",
			Status:     model.RunStatusRunning,
			UpdatedAt:  older,
		},
		{
			RunID:      "run-failed",
			TaskID:     "task-failed",
			WorkerRole: "DeveloperWorker",
			Status:     model.RunStatusFailed,
			UpdatedAt:  older,
		},
	}
	for _, run := range runs {
		if err := repo.Create(ctx, run); err != nil {
			t.Fatalf("create run %s: %v", run.RunID, err)
		}
	}

	running, err := repo.ListRunning(ctx, 10)
	if err != nil {
		t.Fatalf("list running: %v", err)
	}

	if len(running) != 2 {
		t.Fatalf("expected two running runs, got %d", len(running))
	}
	if running[0].RunID != "run-running-older" {
		t.Fatalf("expected older running run first, got %s", running[0].RunID)
	}
	if running[1].RunID != "run-running-newer" {
		t.Fatalf("expected newer running run second, got %s", running[1].RunID)
	}

	limited, err := repo.ListRunning(ctx, 1)
	if err != nil {
		t.Fatalf("list limited running: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected one limited running run, got %d", len(limited))
	}
	if limited[0].RunID != "run-running-older" {
		t.Fatalf("expected limited result to keep ordering, got %s", limited[0].RunID)
	}
}

func TestRunRepositoryListsSucceededRunsForArtifactCleanup(t *testing.T) {
	db, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Run{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewRunRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	old := now.Add(-8 * 24 * time.Hour)
	recent := now.Add(-time.Hour)
	freshStarted := now.Add(-time.Minute)
	eligible := &model.Run{
		RunID: "run-old-success", TaskID: "task-1", WorkerRole: "ReviewerWorker",
		Status: model.RunStatusSucceeded, ExternalSystem: "agent-runner", CompletedAt: &old,
	}
	tooNew := &model.Run{
		RunID: "run-new-success", TaskID: "task-2", WorkerRole: "ReviewerWorker",
		Status: model.RunStatusSucceeded, ExternalSystem: "agent-runner", CompletedAt: &recent,
	}
	failed := &model.Run{
		RunID: "run-old-failed", TaskID: "task-3", WorkerRole: "ReviewerWorker",
		Status: model.RunStatusFailed, ExternalSystem: "agent-runner", CompletedAt: &old, ErrorMessage: "boom",
	}
	purged := &model.Run{
		RunID: "run-already-purged", TaskID: "task-4", WorkerRole: "ReviewerWorker",
		Status: model.RunStatusSucceeded, ExternalSystem: "agent-runner", CompletedAt: &old, ArtifactsPurgedAt: &old,
	}
	freshClaimed := &model.Run{
		RunID: "run-fresh-claim", TaskID: "task-5", WorkerRole: "ReviewerWorker",
		Status: model.RunStatusSucceeded, ExternalSystem: "agent-runner", CompletedAt: &old,
		ArtifactsPurgeStartedAt: &freshStarted, ArtifactsPurgeClaimID: "fresh",
	}
	staleStarted := now.Add(-time.Hour)
	staleClaimed := &model.Run{
		RunID: "run-stale-claim", TaskID: "task-6", WorkerRole: "ReviewerWorker",
		Status: model.RunStatusSucceeded, ExternalSystem: "agent-runner", CompletedAt: &old,
		ArtifactsPurgeStartedAt: &staleStarted, ArtifactsPurgeClaimID: "stale",
	}
	for _, run := range []*model.Run{eligible, tooNew, failed, purged, freshClaimed, staleClaimed} {
		if err := repo.Create(ctx, run); err != nil {
			t.Fatalf("create %s: %v", run.RunID, err)
		}
	}
	got, err := repo.ListSucceededForArtifactCleanup(ctx, now.Add(-7*24*time.Hour), now.Add(-30*time.Minute), 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 || got[0].RunID != "run-old-success" || got[1].RunID != "run-stale-claim" {
		t.Fatalf("expected unclaimed and stale-claimed successes, got %#v", got)
	}
}
