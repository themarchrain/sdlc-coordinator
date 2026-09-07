package repository

import (
	"context"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
)

func newResultRejectionFixture(t *testing.T) *ResultRejectionRepository {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.ResultSubmissionRejection{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewResultRejectionRepository(db)
}

func TestResultRejectionRepositoryRecordsAndCountsByRun(t *testing.T) {
	ctx := context.Background()
	repo := newResultRejectionFixture(t)
	if count, err := repo.CountByRun(ctx, "run-1"); err != nil || count != 0 {
		t.Fatalf("initial count must be 0, got %d err %v", count, err)
	}
	for i := 0; i < 3; i++ {
		err := repo.Record(ctx, &model.ResultSubmissionRejection{
			RunID: "run-1", TaskID: "task-1", ToolName: "submit_developer_result", Role: "DeveloperWorker",
			ProblemsJSON: `[{"problem":"summary is required"}]`,
		})
		if err != nil {
			t.Fatalf("record rejection %d: %v", i, err)
		}
	}
	if err := repo.Record(ctx, &model.ResultSubmissionRejection{RunID: "run-2", TaskID: "task-2", ToolName: "submit_reviewer_result", Role: "ReviewerWorker", ProblemsJSON: `[{"problem":"x"}]`}); err != nil {
		t.Fatalf("record other run rejection: %v", err)
	}
	count, err := repo.CountByRun(ctx, "run-1")
	if err != nil {
		t.Fatalf("count by run: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 rejections for run-1, got %d", count)
	}
	if other, err := repo.CountByRun(ctx, "run-2"); err != nil || other != 1 {
		t.Fatalf("runs must be counted in isolation, run-2=%d err=%v", other, err)
	}
}

func TestResultSubmissionModelCarriesNormalizationTrail(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.ResultSubmission{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	submission := &model.ResultSubmission{
		TaskID: "task-1", RunID: "run-1", Source: "mcp", Role: "DeveloperWorker", PayloadHash: "hash",
		RawPayloadJSON: `{}`, CanonicalJSON: `{}`, NormalizationJSON: `[{"op":"rename_key","path":"taskId","before":"taskId","after":"task_id"}]`,
	}
	if err := db.WithContext(ctx).Create(submission).Error; err != nil {
		t.Fatalf("create submission: %v", err)
	}
	var reloaded model.ResultSubmission
	if err := db.WithContext(ctx).First(&reloaded, submission.ID).Error; err != nil {
		t.Fatalf("reload submission: %v", err)
	}
	if reloaded.NormalizationJSON == "" {
		t.Fatal("normalization trail was not persisted")
	}
	if time.Time.IsZero(reloaded.CreatedAt) {
		t.Fatal("timestamps missing")
	}
}
