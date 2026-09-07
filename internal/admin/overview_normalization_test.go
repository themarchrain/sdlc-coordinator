package admin

import (
	"context"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
)

func TestOverviewCountsSubmissionNormalizationBuckets(t *testing.T) {
	service, db := newQueryServiceTestDB(t)
	now := time.Now().UTC()

	task := &model.Task{TaskID: "task-norm", DedupeKey: "task-norm", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusSucceeded, ContextJSON: `{}`, CreatedAt: now, UpdatedAt: now}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("seed task: %v", err)
	}
	submissions := []model.ResultSubmission{
		{SubmissionID: "submission-clean", TaskID: task.TaskID, RunID: "run-clean", Source: "mcp", Role: "DeveloperWorker", PayloadHash: "h1", RawPayloadJSON: "{}", CanonicalJSON: "{}", Status: model.ResultSubmissionStatusSucceeded},
		{SubmissionID: "submission-norm", TaskID: task.TaskID, RunID: "run-norm", Source: "mcp", Role: "DeveloperWorker", PayloadHash: "h2", RawPayloadJSON: "{}", CanonicalJSON: "{}", NormalizationJSON: `[{"op":"rename_key","path":"taskId"}]`, Status: model.ResultSubmissionStatusSucceeded},
	}
	for i := range submissions {
		if err := db.Create(&submissions[i]).Error; err != nil {
			t.Fatalf("seed submission: %v", err)
		}
	}
	rejection := &model.ResultSubmissionRejection{RunID: "run-norm", TaskID: task.TaskID, ToolName: "submit_developer_result", Role: "DeveloperWorker", ProblemsJSON: `[]`}
	if err := db.Create(rejection).Error; err != nil {
		t.Fatalf("seed rejection: %v", err)
	}

	got, err := service.Overview(context.Background())
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if got.SubmissionsByNormalization["clean"] != 1 || got.SubmissionsByNormalization["normalized"] != 1 || got.SubmissionsByNormalization["rejected"] != 1 {
		t.Fatalf("unexpected normalization buckets: %#v", got.SubmissionsByNormalization)
	}
}
