package agent

import (
	"context"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"

	"gorm.io/gorm"
)

func TestCodexQuerySummarizesUsageAndExcludesOtherProviders(t *testing.T) {
	db := newCodexQueryTestDB(t)
	seedCodexQueryTestData(t, db)
	query := NewQuery(db)

	summary, err := query.CodexSummary(context.Background(), CodexFilter{})
	if err != nil {
		t.Fatalf("query summary: %v", err)
	}
	if summary.TotalExecutions != 2 || summary.StatusCounts[string(model.AgentExecutionStatusSucceeded)] != 1 || summary.StatusCounts[string(model.AgentExecutionStatusFailed)] != 1 {
		t.Fatalf("unexpected execution counts: %#v", summary)
	}
	if summary.AverageDurationSeconds != 60 || summary.P50DurationSeconds != 60 || summary.P90DurationSeconds != 60 {
		t.Fatalf("unexpected durations: %#v", summary)
	}
	if summary.InputTokens != 150 || summary.CachedInputTokens != 100 || summary.OutputTokens != 30 || summary.TurnCount != 2 {
		t.Fatalf("unexpected usage: %#v", summary)
	}
	if summary.CacheHitRatio == nil || *summary.CacheHitRatio < 0.666 || *summary.CacheHitRatio > 0.667 {
		t.Fatalf("unexpected cache ratio: %#v", summary.CacheHitRatio)
	}
	if summary.ErrorCount != 1 || summary.TimeoutCount != 1 {
		t.Fatalf("unexpected failure counts: %#v", summary)
	}
}

func TestCodexQueryListsCorrelatedExecutionsAndDetailEvents(t *testing.T) {
	db := newCodexQueryTestDB(t)
	seedCodexQueryTestData(t, db)
	query := NewQuery(db)

	page, err := query.ListCodexExecutions(context.Background(), CodexFilter{Status: "succeeded", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list executions: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("unexpected execution page: %#v", page)
	}
	item := page.Items[0]
	if item.ExecutionID != "run-codex-ok" || item.TaskID != "task-codex-ok" || item.Role != "ReviewerWorker" || item.Model != "gpt-alpha" || item.Effort != "high" {
		t.Fatalf("unexpected correlated execution: %#v", item)
	}

	detail, err := query.GetCodexExecution(context.Background(), item.ExecutionID)
	if err != nil {
		t.Fatalf("get execution: %v", err)
	}
	if len(detail.Events) != 3 || detail.Events[0].Type != "accepted" || detail.Events[0].Payload == nil || detail.Events[2].Type != "codex_usage" || detail.Events[2].Payload["input_tokens"] != float64(100) {
		t.Fatalf("unexpected execution detail: %#v", detail)
	}
	if _, err := query.GetCodexExecution(context.Background(), "run-claude"); err == nil {
		t.Fatal("Codex query exposed another provider")
	}
}

func newCodexQueryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.AgentExecutionEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedCodexQueryTestData(t *testing.T, db *gorm.DB) {
	t.Helper()
	started := time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC)
	finished := started.Add(time.Minute)
	entities := []any{
		&model.Task{TaskID: "task-codex-ok", DedupeKey: "task-codex-ok", Type: "review_mr", Role: "ReviewerWorker", TargetRepo: "group/review", Status: model.TaskStatusSucceeded},
		&model.Run{RunID: "run-codex-ok", TaskID: "task-codex-ok", WorkerRole: "ReviewerWorker", Status: model.RunStatusSucceeded, StartedAt: &started, CompletedAt: &finished},
		&model.AgentExecution{RunID: "run-codex-ok", ExecutionID: "run-codex-ok", Provider: "codex-cli", ProviderModel: "gpt-alpha", ProviderEffort: "high", Status: model.AgentExecutionStatusSucceeded, RequestHash: "hash-ok", StartedAt: &started, FinishedAt: &finished},
		&model.AgentExecutionEvent{ExecutionID: "run-codex-ok", RunID: "run-codex-ok", Provider: "codex-cli", Seq: 1, Type: "accepted", OccurredAt: started, PayloadJSON: `null`},
		&model.AgentExecutionEvent{ExecutionID: "run-codex-ok", RunID: "run-codex-ok", Provider: "codex-cli", Seq: 2, Type: "codex_session", OccurredAt: started, PayloadJSON: `{"thread_id":"thread-ok"}`},
		&model.AgentExecutionEvent{ExecutionID: "run-codex-ok", RunID: "run-codex-ok", Provider: "codex-cli", Seq: 3, Type: "codex_usage", OccurredAt: finished, PayloadJSON: `{"input_tokens":100,"cached_input_tokens":80,"output_tokens":20}`},
		&model.Task{TaskID: "task-codex-failed", DedupeKey: "task-codex-failed", Type: "develop_from_issue", Role: "DeveloperWorker", TargetRepo: "group/develop", Status: model.TaskStatusBlocked},
		&model.Run{RunID: "run-codex-failed", TaskID: "task-codex-failed", WorkerRole: "DeveloperWorker", Status: model.RunStatusFailed},
		&model.AgentExecution{RunID: "run-codex-failed", ExecutionID: "run-codex-failed", Provider: "codex-cli", ProviderModel: "gpt-beta", ProviderEffort: "medium", Status: model.AgentExecutionStatusFailed, RequestHash: "hash-failed", ErrorCode: "execution_timeout"},
		&model.AgentExecutionEvent{ExecutionID: "run-codex-failed", RunID: "run-codex-failed", Provider: "codex-cli", Seq: 1, Type: "codex_usage", OccurredAt: finished, PayloadJSON: `{"input_tokens":50,"cached_input_tokens":20,"output_tokens":10}`},
		&model.Task{TaskID: "task-claude", DedupeKey: "task-claude", Type: "review_mr", Role: "ReviewerWorker", Status: model.TaskStatusSucceeded},
		&model.Run{RunID: "run-claude", TaskID: "task-claude", WorkerRole: "ReviewerWorker", Status: model.RunStatusSucceeded},
		&model.AgentExecution{RunID: "run-claude", ExecutionID: "run-claude", Provider: "claude-cli", Status: model.AgentExecutionStatusSucceeded, RequestHash: "hash-claude"},
		&model.AgentExecutionEvent{ExecutionID: "run-claude", RunID: "run-claude", Provider: "claude-cli", Seq: 1, Type: "claude_usage", OccurredAt: finished, PayloadJSON: `{"input_tokens":999}`},
	}
	for _, entity := range entities {
		if err := db.Create(entity).Error; err != nil {
			t.Fatalf("seed %T: %v", entity, err)
		}
	}
}
