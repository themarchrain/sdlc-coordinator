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
	// claude usage folds cache reads/creations into input ("其中缓存" 子集口径):
	// 200+150+10=360;codex input 已含 cached。
	if summary.InputTokens != 510 || summary.CachedInputTokens != 250 || summary.OutputTokens != 70 || summary.TurnCount != 3 {
		t.Fatalf("unexpected usage: %#v", summary)
	}
	if summary.CacheHitRatio == nil || *summary.CacheHitRatio < 0.490 || *summary.CacheHitRatio > 0.491 {
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
	if claudeSummary.TotalExecutions != 1 || claudeSummary.InputTokens != 360 || claudeSummary.CachedInputTokens != 150 || claudeSummary.OutputTokens != 40 || claudeSummary.TurnCount != 1 {
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

// TestExecutionQueryPrefersTerminalClaudeResultUsage 复刻兼容中转形态:
// 每条 claude_usage 都是零,真值只在终态 claude_result.usage;此时逐回合
// 事件只计回合数,不得把零值或缺终态的双计混进 token 总量。
func TestExecutionQueryPrefersTerminalClaudeResultUsage(t *testing.T) {
	db := newExecutionQueryTestDB(t)
	started := time.Date(2026, 9, 9, 12, 10, 30, 0, time.UTC)
	finished := started.Add(68 * time.Second)
	entities := []any{
		&model.Task{TaskID: "task-relay", DedupeKey: "task-relay", Type: "develop_from_issue", Role: "DeveloperWorker", TargetRepo: "octo-org/external", Status: model.TaskStatusSucceeded},
		&model.Run{RunID: "run-relay", TaskID: "task-relay", WorkerRole: "DeveloperWorker", Status: model.RunStatusSucceeded, StartedAt: &started, CompletedAt: &finished},
		&model.AgentExecution{RunID: "run-relay", ExecutionID: "run-relay", Provider: "claude-cli", ProviderModel: "glm-5.3-flash", Status: model.AgentExecutionStatusSucceeded, RequestHash: "hash-relay", StartedAt: &started, FinishedAt: &finished},
		&model.AgentExecutionEvent{ExecutionID: "run-relay", RunID: "run-relay", Provider: "claude-cli", Seq: 2, Type: "claude_usage", OccurredAt: started, PayloadJSON: `{"model":"glm-5.3-flash","usage":{"input_tokens":0,"output_tokens":0}}`},
		&model.AgentExecutionEvent{ExecutionID: "run-relay", RunID: "run-relay", Provider: "claude-cli", Seq: 3, Type: "claude_usage", OccurredAt: started, PayloadJSON: `{"model":"glm-5.3-flash","usage":{"input_tokens":0,"output_tokens":0}}`},
		&model.AgentExecutionEvent{ExecutionID: "run-relay", RunID: "run-relay", Provider: "claude-cli", Seq: 4, Type: "claude_result", OccurredAt: finished, PayloadJSON: `{"subtype":"success","total_cost_usd":0.0987,"usage":{"input_tokens":7061,"output_tokens":1749,"cache_creation_input_tokens":0,"cache_read_input_tokens":39360}}`},
		// 同库再放一个有真值逐回合事件的 claude 执行:有终态时逐回合不得叠加。
		&model.Task{TaskID: "task-native", DedupeKey: "task-native", Type: "review_mr", Role: "ReviewerWorker", TargetRepo: "octo-org/external", Status: model.TaskStatusSucceeded},
		&model.Run{RunID: "run-native", TaskID: "task-native", WorkerRole: "ReviewerWorker", Status: model.RunStatusSucceeded, StartedAt: &started, CompletedAt: &finished},
		&model.AgentExecution{RunID: "run-native", ExecutionID: "run-native", Provider: "claude-cli", ProviderModel: "claude-alpha", Status: model.AgentExecutionStatusSucceeded, RequestHash: "hash-native", StartedAt: &started, FinishedAt: &finished},
		&model.AgentExecutionEvent{ExecutionID: "run-native", RunID: "run-native", Provider: "claude-cli", Seq: 1, Type: "claude_usage", OccurredAt: started, PayloadJSON: `{"model":"claude-alpha","usage":{"input_tokens":100,"output_tokens":10}}`},
		&model.AgentExecutionEvent{ExecutionID: "run-native", RunID: "run-native", Provider: "claude-cli", Seq: 2, Type: "claude_result", OccurredAt: finished, PayloadJSON: `{"subtype":"success","usage":{"input_tokens":500,"output_tokens":50,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`},
	}
	for _, entity := range entities {
		if err := db.Create(entity).Error; err != nil {
			t.Fatalf("seed %T: %v", entity, err)
		}
	}

	summary, err := NewQuery(db).ExecutionSummary(context.Background(), ExecutionFilter{})
	if err != nil {
		t.Fatalf("query summary: %v", err)
	}
	// relay: 7061+39360=46421/39360/1749;native 终态 500/0/50,逐回合 100/10 不叠加。
	if summary.InputTokens != 46921 || summary.CachedInputTokens != 39360 || summary.OutputTokens != 1799 {
		t.Fatalf("unexpected usage totals: %#v", summary)
	}
	if summary.TurnCount != 3 {
		t.Fatalf("unexpected turn count: %#v", summary)
	}
	if summary.CacheHitRatio == nil || *summary.CacheHitRatio < 0.838 || *summary.CacheHitRatio > 0.839 {
		t.Fatalf("unexpected cache ratio: %#v", summary.CacheHitRatio)
	}
}

func newExecutionQueryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.StopRequest{}, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.AgentExecutionEvent{}); err != nil {
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
