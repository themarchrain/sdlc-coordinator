package agent

import (
	"context"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"

	"gorm.io/gorm"
)

func TestExecutionQuerySummarizesUsageAcrossProviders(t *testing.T) {
	db := newExecutionQueryTestDB(t)
	seedExecutionQueryTestData(t, db)
	query := NewQuery(db)

	summary, err := query.ExecutionSummary(context.Background(), ExecutionFilter{})
	if err != nil {
		t.Fatalf("query summary: %v", err)
	}
	if summary.TotalExecutions != 3 || summary.StatusCounts[string(model.AgentExecutionStatusSucceeded)] != 2 || summary.StatusCounts[string(model.AgentExecutionStatusFailed)] != 1 {
		t.Fatalf("unexpected execution counts: %#v", summary)
	}
	if summary.AverageDurationSeconds != 45 || summary.P50DurationSeconds != 30 || summary.P90DurationSeconds != 60 {
		t.Fatalf("unexpected durations: %#v", summary)
	}
	if summary.InputTokens != 350 || summary.CachedInputTokens != 250 || summary.OutputTokens != 70 || summary.TurnCount != 3 {
		t.Fatalf("unexpected usage: %#v", summary)
	}
	if summary.CacheHitRatio == nil || *summary.CacheHitRatio < 0.714 || *summary.CacheHitRatio > 0.715 {
		t.Fatalf("unexpected cache ratio: %#v", summary.CacheHitRatio)
	}
	if summary.ErrorCount != 1 || summary.TimeoutCount != 1 {
		t.Fatalf("unexpected failure counts: %#v", summary)
	}
}

func TestExecutionQueryFiltersByProvider(t *testing.T) {
	db := newExecutionQueryTestDB(t)
	seedExecutionQueryTestData(t, db)
	query := NewQuery(db)

	tests := []struct {
		name      string
		provider  string
		wantTotal int64
		wantCount int
	}{
		{name: "all providers by default", provider: "", wantTotal: 3, wantCount: 3},
		{name: "codex only", provider: "codex-cli", wantTotal: 2, wantCount: 2},
		{name: "claude only", provider: "claude-cli", wantTotal: 1, wantCount: 1},
		{name: "unknown provider yields empty set", provider: "some-other-cli", wantTotal: 0, wantCount: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			page, err := query.ListExecutions(context.Background(), ExecutionFilter{Provider: test.provider, Page: 1, PageSize: 10})
			if err != nil {
				t.Fatalf("list executions: %v", err)
			}
			if page.Total != test.wantTotal {
				t.Fatalf("unexpected total: %#v", page)
			}
			if len(page.Items) != test.wantCount {
				t.Fatalf("unexpected item count: %#v", page.Items)
			}
			for _, item := range page.Items {
				if item.Provider != test.provider && test.provider != "" {
					t.Fatalf("provider leak in items: %#v", item)
				}
			}
		})
	}

	claudeSummary, err := query.ExecutionSummary(context.Background(), ExecutionFilter{Provider: "claude-cli"})
	if err != nil {
		t.Fatalf("claude summary: %v", err)
	}
	if claudeSummary.TotalExecutions != 1 || claudeSummary.InputTokens != 200 || claudeSummary.CachedInputTokens != 150 || claudeSummary.OutputTokens != 40 || claudeSummary.TurnCount != 1 {
		t.Fatalf("unexpected claude summary: %#v", claudeSummary)
	}
}

func TestExecutionQueryListsCorrelatedExecutionsAndDetailEvents(t *testing.T) {
	db := newExecutionQueryTestDB(t)
	seedExecutionQueryTestData(t, db)
	query := NewQuery(db)

	page, err := query.ListExecutions(context.Background(), ExecutionFilter{Status: "succeeded", Provider: "codex-cli", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list executions: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("unexpected execution page: %#v", page)
	}
	item := page.Items[0]
	if item.ExecutionID != "run-codex-ok" || item.TaskID != "task-codex-ok" || item.Role != "ReviewerWorker" || item.Model != "gpt-alpha" || item.Effort != "high" || item.Provider != "codex-cli" {
		t.Fatalf("unexpected correlated execution: %#v", item)
	}

	detail, err := query.GetExecution(context.Background(), item.ExecutionID)
	if err != nil {
		t.Fatalf("get execution: %v", err)
	}
	if len(detail.Events) != 3 || detail.Events[0].Type != "accepted" || detail.Events[0].Payload == nil || detail.Events[2].Type != "codex_usage" || detail.Events[2].Payload["input_tokens"] != float64(100) {
		t.Fatalf("unexpected execution detail: %#v", detail)
	}

	claudeDetail, err := query.GetExecution(context.Background(), "run-claude")
	if err != nil {
		t.Fatalf("get claude execution: %v", err)
	}
	if claudeDetail.Provider != "claude-cli" || len(claudeDetail.Events) != 2 || claudeDetail.Events[0].Type != "claude_session" || claudeDetail.Events[1].Type != "claude_usage" {
		t.Fatalf("claude detail must expose only its own provider's events: %#v", claudeDetail)
	}
}

func newExecutionQueryTestDB(t *testing.T) *gorm.DB {
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

func seedExecutionQueryTestData(t *testing.T, db *gorm.DB) {
	t.Helper()
	started := time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC)
	finished := started.Add(time.Minute)
	claudeStarted := started.Add(time.Hour)
	claudeFinished := claudeStarted.Add(30 * time.Second)
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
		&model.Run{RunID: "run-claude", TaskID: "task-claude", WorkerRole: "ReviewerWorker", Status: model.RunStatusSucceeded, StartedAt: &claudeStarted, CompletedAt: &claudeFinished},
		&model.AgentExecution{RunID: "run-claude", ExecutionID: "run-claude", Provider: "claude-cli", ProviderModel: "claude-alpha", Status: model.AgentExecutionStatusSucceeded, RequestHash: "hash-claude", StartedAt: &claudeStarted, FinishedAt: &claudeFinished},
		&model.AgentExecutionEvent{ExecutionID: "run-claude", RunID: "run-claude", Provider: "claude-cli", Seq: 1, Type: "claude_session", OccurredAt: claudeStarted, PayloadJSON: `{"session_id":"session-claude"}`},
		&model.AgentExecutionEvent{ExecutionID: "run-claude", RunID: "run-claude", Provider: "claude-cli", Seq: 2, Type: "claude_usage", OccurredAt: claudeFinished, PayloadJSON: `{"model":"claude-alpha","usage":{"input_tokens":200,"cache_read_input_tokens":150,"cache_creation_input_tokens":10,"output_tokens":40}}`},
	}
	for _, entity := range entities {
		if err := db.Create(entity).Error; err != nil {
			t.Fatalf("seed %T: %v", entity, err)
		}
	}
}
