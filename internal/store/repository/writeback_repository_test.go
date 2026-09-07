package repository

import (
	"context"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
)

func TestWritebackRepositoryCreateGetAndList(t *testing.T) {
	db, err := store.Open("file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Writeback{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewWritebackRepository(db)
	ctx := context.Background()

	writeback := &model.Writeback{
		TaskID:      "task-writeback-1",
		RunID:       "run-writeback-1",
		DedupeKey:   "gitlab:task-writeback-1:run-writeback-1:0:create_issue",
		Kind:        "create_issue",
		ProjectID:   "group/framework",
		Status:      model.WritebackStatusPending,
		RequestJSON: `{"title":"framework defect"}`,
	}
	if err := repo.Create(ctx, writeback); err != nil {
		t.Fatalf("create writeback: %v", err)
	}
	if writeback.WritebackID == "" {
		t.Fatal("expected generated writeback id")
	}

	byDedupe, err := repo.GetByDedupeKey(ctx, "gitlab:task-writeback-1:run-writeback-1:0:create_issue")
	if err != nil {
		t.Fatalf("get by dedupe: %v", err)
	}
	if byDedupe.Kind != "create_issue" {
		t.Fatalf("expected create_issue kind, got %s", byDedupe.Kind)
	}

	byRun, err := repo.ListByRunID(ctx, "run-writeback-1")
	if err != nil {
		t.Fatalf("list by run: %v", err)
	}
	if len(byRun) != 1 {
		t.Fatalf("expected one writeback by run, got %d", len(byRun))
	}
	if byRun[0].DedupeKey != "gitlab:task-writeback-1:run-writeback-1:0:create_issue" {
		t.Fatalf("unexpected dedupe key: %s", byRun[0].DedupeKey)
	}
}

func TestWritebackRepositoryUpsertAndUpdate(t *testing.T) {
	db, err := store.Open("file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Writeback{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewWritebackRepository(db)
	ctx := context.Background()

	first := &model.Writeback{
		TaskID:      "task-writeback-upsert",
		RunID:       "run-writeback-upsert",
		DedupeKey:   "gitlab:writeback-upsert",
		Kind:        "create_merge_request",
		ProjectID:   "group/framework",
		Status:      model.WritebackStatusPending,
		RequestJSON: `{"title":"initial","source_branch":"agent/a","target_branch":"main"}`,
	}
	if err := repo.UpsertByDedupeKey(ctx, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	second := &model.Writeback{
		TaskID:      "task-writeback-upsert",
		RunID:       "run-writeback-upsert",
		DedupeKey:   "gitlab:writeback-upsert",
		Kind:        "create_merge_request",
		ProjectID:   "group/framework",
		Status:      model.WritebackStatusRunning,
		RequestJSON: `{"title":"updated","source_branch":"agent/b","target_branch":"main"}`,
	}
	if err := repo.UpsertByDedupeKey(ctx, second); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	byRun, err := repo.ListByRunID(ctx, "run-writeback-upsert")
	if err != nil {
		t.Fatalf("list by run: %v", err)
	}
	if len(byRun) != 1 {
		t.Fatalf("expected one writeback after upsert, got %d", len(byRun))
	}
	if byRun[0].RequestJSON != `{"title":"updated","source_branch":"agent/b","target_branch":"main"}` {
		t.Fatalf("expected updated request json, got %s", byRun[0].RequestJSON)
	}
	if byRun[0].Status != model.WritebackStatusRunning {
		t.Fatalf("expected running status, got %s", byRun[0].Status)
	}

	byRun[0].Status = model.WritebackStatusSucceeded
	byRun[0].ResponseJSON = `{"iid":7,"web_url":"https://gitlab.example/group/framework/-/merge_requests/7"}`
	byRun[0].ExternalID = "7"
	byRun[0].ExternalURL = "https://gitlab.example/group/framework/-/merge_requests/7"
	byRun[0].AttemptCount = 1
	if err := repo.Update(ctx, &byRun[0]); err != nil {
		t.Fatalf("update succeeded writeback: %v", err)
	}

	succeeded, err := repo.GetByDedupeKey(ctx, "gitlab:writeback-upsert")
	if err != nil {
		t.Fatalf("get succeeded writeback: %v", err)
	}
	if succeeded.Status != model.WritebackStatusSucceeded {
		t.Fatalf("expected succeeded status, got %s", succeeded.Status)
	}
	if succeeded.ExternalID != "7" {
		t.Fatalf("expected external id 7, got %s", succeeded.ExternalID)
	}

	succeeded.Status = model.WritebackStatusFailed
	succeeded.ErrorMessage = "gitlab unavailable"
	if err := repo.Update(ctx, succeeded); err != nil {
		t.Fatalf("update failed writeback: %v", err)
	}

	failed, err := repo.GetByDedupeKey(ctx, "gitlab:writeback-upsert")
	if err != nil {
		t.Fatalf("get failed writeback: %v", err)
	}
	if failed.Status != model.WritebackStatusFailed {
		t.Fatalf("expected failed status, got %s", failed.Status)
	}
	if failed.ErrorMessage != "gitlab unavailable" {
		t.Fatalf("expected error message, got %s", failed.ErrorMessage)
	}
}

func TestWritebackRepositoryGetsSucceededMergeRequestByExternalIdentity(t *testing.T) {
	db, err := store.Open("file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Writeback{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewWritebackRepository(db)
	ctx := context.Background()

	fixtures := []*model.Writeback{
		{TaskID: "wrong-status", RunID: "run-1", DedupeKey: "mr-wrong-status", Kind: "create_merge_request", ProjectID: "15594", ExternalID: "12", Status: model.WritebackStatusFailed},
		{TaskID: "wrong-project", RunID: "run-2", DedupeKey: "mr-wrong-project", Kind: "create_merge_request", ProjectID: "999", ExternalID: "12", Status: model.WritebackStatusSucceeded},
		{TaskID: "origin-developer-task", RunID: "run-3", DedupeKey: "mr-origin", Kind: "create_merge_request", ProjectID: "15594", ExternalID: "12", Status: model.WritebackStatusSucceeded},
	}
	for _, fixture := range fixtures {
		if err := repo.Create(ctx, fixture); err != nil {
			t.Fatalf("create fixture %s: %v", fixture.DedupeKey, err)
		}
	}

	writeback, err := repo.GetSucceededMergeRequest(ctx, "15594", 12)
	if err != nil {
		t.Fatalf("get succeeded MR writeback: %v", err)
	}
	if writeback.TaskID != "origin-developer-task" {
		t.Fatalf("unexpected origin task %q", writeback.TaskID)
	}
}

func TestWritebackRepositoryGetsLatestSucceededReviewerComment(t *testing.T) {
	db, err := store.Open("file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Writeback{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewWritebackRepository(db)
	ctx := context.Background()

	tasks := []*model.Task{
		{TaskID: "review-old", DedupeKey: "review-old", Type: "review_mr", Role: "ReviewerWorker", Status: model.TaskStatusSucceeded},
		{TaskID: "developer-comment", DedupeKey: "developer-comment", Type: "rework_merge_request", Role: "DeveloperWorker", Status: model.TaskStatusSucceeded},
		{TaskID: "review-new", DedupeKey: "review-new", Type: "review_mr", Role: "ReviewerWorker", Status: model.TaskStatusSucceeded},
	}
	for _, task := range tasks {
		if err := db.WithContext(ctx).Create(task).Error; err != nil {
			t.Fatalf("create task %s: %v", task.TaskID, err)
		}
	}

	marker := func(namespace, taskID, runID string) string {
		return "<!-- " + namespace + "-coordinator-operation:codehost:" + taskID + ":" + runID + ":0:create_merge_request_note -->"
	}
	fixtures := []*model.Writeback{
		{TaskID: "review-old", RunID: "run-review-old", DedupeKey: "review-old-note", Kind: "create_merge_request_note", ProjectID: "group/project", TargetType: "merge_request", TargetIID: 21, Status: model.WritebackStatusSucceeded, OperationMarker: marker("legacy", "review-old", "run-review-old"), RequestJSON: `{"body":"old review"}`},
		{TaskID: "review-new", RunID: "run-review-failed", DedupeKey: "review-failed-note", Kind: "create_merge_request_note", ProjectID: "group/project", TargetType: "merge_request", TargetIID: 21, Status: model.WritebackStatusFailed, OperationMarker: marker("agent", "review-new", "run-review-failed"), RequestJSON: `{"body":"failed review"}`},
		{TaskID: "developer-comment", RunID: "run-developer-comment", DedupeKey: "developer-comment-note", Kind: "create_merge_request_note", ProjectID: "group/project", TargetType: "merge_request", TargetIID: 21, Status: model.WritebackStatusSucceeded, OperationMarker: marker("agent", "developer-comment", "run-developer-comment"), RequestJSON: `{"body":"not a review"}`},
		{TaskID: "review-new", RunID: "run-review-new", DedupeKey: "review-new-note", Kind: "create_merge_request_note", ProjectID: "group/project", TargetType: "merge_request", TargetIID: 21, Status: model.WritebackStatusSucceeded, OperationMarker: marker("agent", "review-new", "run-review-new"), RequestJSON: `{"body":"latest review"}`},
		{TaskID: "review-new", RunID: "run-other-mr", DedupeKey: "other-mr-note", Kind: "create_merge_request_note", ProjectID: "group/project", TargetType: "merge_request", TargetIID: 22, Status: model.WritebackStatusSucceeded, OperationMarker: marker("agent", "review-new", "run-other-mr"), RequestJSON: `{"body":"other MR"}`},
		{TaskID: "review-new", RunID: "run-invalid-marker", DedupeKey: "invalid-marker-note", Kind: "create_merge_request_note", ProjectID: "group/project", TargetType: "merge_request", TargetIID: 21, Status: model.WritebackStatusSucceeded, OperationMarker: "<!-- another-system:codehost:review-new:run-invalid-marker:0:create_merge_request_note -->", RequestJSON: `{"body":"not a coordinator review"}`},
	}
	for _, fixture := range fixtures {
		if err := repo.Create(ctx, fixture); err != nil {
			t.Fatalf("create fixture %s: %v", fixture.DedupeKey, err)
		}
	}

	comment, err := repo.GetLatestSucceededReviewerComment(ctx, "group/project", 21)
	if err != nil {
		t.Fatalf("get latest reviewer comment: %v", err)
	}
	if comment.TaskID != "review-new" || comment.RunID != "run-review-new" || comment.RequestJSON != `{"body":"latest review"}` {
		t.Fatalf("unexpected latest reviewer comment: %#v", comment)
	}
}
