package repository

import (
	"context"
	"errors"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
)

func openDeliveryCheckpointTestDB(t *testing.T) *DeveloperDeliveryCheckpointRepository {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.DeveloperDeliveryCheckpoint{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewDeveloperDeliveryCheckpointRepository(db)
}

func baselineRecord(runID string) *model.DeveloperDeliveryCheckpoint {
	return &model.DeveloperDeliveryCheckpoint{
		TaskID: "task-1", RunID: runID, TaskType: "develop_from_issue",
		RepositoryURL: "https://gitlab.example.com/group/a.git", DeliveryBranch: "agent/task-1",
		BaselineSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
}

func TestRecordBaselineIsIdempotentForIdenticalMetadata(t *testing.T) {
	repo := openDeliveryCheckpointTestDB(t)
	ctx := context.Background()
	if err := repo.RecordBaseline(ctx, baselineRecord("run-baseline")); err != nil {
		t.Fatalf("record baseline: %v", err)
	}
	if err := repo.RecordBaseline(ctx, baselineRecord("run-baseline")); err != nil {
		t.Fatalf("re-record identical baseline must be idempotent: %v", err)
	}
}

func TestRecordBaselineRejectsDivergentMetadata(t *testing.T) {
	repo := openDeliveryCheckpointTestDB(t)
	ctx := context.Background()
	if err := repo.RecordBaseline(ctx, baselineRecord("run-diverge")); err != nil {
		t.Fatal(err)
	}
	divergent := baselineRecord("run-diverge")
	divergent.BaselineSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := repo.RecordBaseline(ctx, divergent); !errors.Is(err, ErrDeliveryCheckpointConflict) {
		t.Fatalf("divergent baseline must conflict, got %v", err)
	}
	otherBranch := baselineRecord("run-diverge")
	otherBranch.DeliveryBranch = "agent/other"
	if err := repo.RecordBaseline(ctx, otherBranch); !errors.Is(err, ErrDeliveryCheckpointConflict) {
		t.Fatalf("different branch must conflict, got %v", err)
	}
}

func TestRecordCommitFlipsOnceAndIsIdempotentForSameSHA(t *testing.T) {
	repo := openDeliveryCheckpointTestDB(t)
	ctx := context.Background()
	if err := repo.RecordBaseline(ctx, baselineRecord("run-commit")); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordCommit(ctx, "run-commit", "CCCCccccCCCCccccCCCCccccCCCCccccCCCCcccc"); err != nil {
		t.Fatalf("record commit: %v", err)
	}
	if err := repo.RecordCommit(ctx, "run-commit", "cccccccccccccccccccccccccccccccccccccccc"); err != nil {
		t.Fatalf("re-record same commit must be idempotent: %v", err)
	}
	record, err := repo.Load(ctx, "run-commit")
	if err != nil || record == nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if record.CommitSHA != "cccccccccccccccccccccccccccccccccccccccc" {
		t.Fatalf("unexpected recorded commit: %s", record.CommitSHA)
	}
}

func TestRecordCommitRejectsSecondDifferentSHA(t *testing.T) {
	repo := openDeliveryCheckpointTestDB(t)
	ctx := context.Background()
	if err := repo.RecordBaseline(ctx, baselineRecord("run-second")); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordCommit(ctx, "run-second", "cccccccccccccccccccccccccccccccccccccccc"); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordCommit(ctx, "run-second", "dddddddddddddddddddddddddddddddddddddddd"); !errors.Is(err, ErrDeliveryCheckpointConflict) {
		t.Fatalf("second different commit must conflict, got %v", err)
	}
}

func TestRecordCommitRequiresPinnedBaseline(t *testing.T) {
	repo := openDeliveryCheckpointTestDB(t)
	if err := repo.RecordCommit(context.Background(), "run-missing", "cccccccccccccccccccccccccccccccccccccccc"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("commit without a pinned baseline must be rejected with not-found, got %v", err)
	}
}

func TestLoadReturnsNilForUnknownRun(t *testing.T) {
	repo := openDeliveryCheckpointTestDB(t)
	record, err := repo.Load(context.Background(), "run-unknown")
	if err != nil || record != nil {
		t.Fatalf("unknown run must load as nil,nil, got %v,%v", record, err)
	}
}
