package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
)

func TestAgentExecutionRepositoryPersistsRunnerProjection(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.AgentExecution{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewAgentExecutionRepository(db)
	ctx := context.Background()
	observedAt := time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC)
	startedAt := observedAt.Add(-time.Minute)
	finishedAt := observedAt.Add(time.Minute)
	execution := &model.AgentExecution{
		RunID: "run-projection", ExecutionID: "run-projection", Provider: "claude_cli",
		ProviderConfigRevision: 7, Status: model.AgentExecutionStatusSucceeded,
		RequestHash: "request-hash", LastEventSeq: 4, LastObservedAt: &observedAt,
		ResultHash: "result-hash", StartedAt: &startedAt, FinishedAt: &finishedAt,
	}
	if err := repo.Create(ctx, execution); err != nil {
		t.Fatalf("create projection: %v", err)
	}

	loaded, err := repo.GetByRunID(ctx, execution.RunID)
	if err != nil {
		t.Fatalf("get projection: %v", err)
	}
	if loaded.ExecutionID != execution.RunID || loaded.Provider != "claude_cli" || loaded.ProviderConfigRevision != 7 {
		t.Fatalf("unexpected identity fields: %#v", loaded)
	}
	if loaded.Status != model.AgentExecutionStatusSucceeded || loaded.RequestHash != "request-hash" || loaded.LastEventSeq != 4 {
		t.Fatalf("unexpected observation fields: %#v", loaded)
	}
	if loaded.ResultHash != "result-hash" || loaded.StartedAt == nil || loaded.FinishedAt == nil || loaded.LastObservedAt == nil {
		t.Fatalf("unexpected result fields: %#v", loaded)
	}

	loaded.Status = model.AgentExecutionStatusFailed
	loaded.ErrorCode = "provider_failed"
	loaded.ErrorMessage = "provider exited"
	if err := repo.Update(ctx, loaded); err != nil {
		t.Fatalf("update projection: %v", err)
	}
	updated, err := repo.GetByRunID(ctx, execution.RunID)
	if err != nil || updated.ErrorCode != "provider_failed" || updated.ErrorMessage != "provider exited" {
		t.Fatalf("unexpected updated projection: projection=%#v err=%v", updated, err)
	}
}

func TestAgentExecutionRepositoryUpsertsEventsByExecutionAndSequence(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.AgentExecution{}, &model.AgentExecutionEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewAgentExecutionRepository(db)
	ctx := context.Background()
	occurredAt := time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	events := []model.AgentExecutionEvent{
		{ExecutionID: "run-events", RunID: "run-events", Provider: "codex-cli", Seq: 1, Type: "codex_session", OccurredAt: occurredAt, PayloadJSON: `{"thread_id":"thread-1"}`},
		{ExecutionID: "run-events", RunID: "run-events", Provider: "codex-cli", Seq: 2, Type: "codex_usage", OccurredAt: occurredAt.Add(time.Second), PayloadJSON: `{"input_tokens":120,"cached_input_tokens":80}`},
	}
	if err := repo.UpsertEvents(ctx, events); err != nil {
		t.Fatalf("upsert events: %v", err)
	}
	if err := repo.UpsertEvents(ctx, events); err != nil {
		t.Fatalf("repeat upsert events: %v", err)
	}

	var stored []model.AgentExecutionEvent
	if err := db.Order("seq ASC").Find(&stored).Error; err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(stored) != 2 || stored[1].Type != "codex_usage" || !json.Valid([]byte(stored[1].PayloadJSON)) {
		t.Fatalf("unexpected persisted events: %#v", stored)
	}
}

func TestAgentExecutionRepositoryEnforcesOneExecutionPerRun(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.AgentExecution{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewAgentExecutionRepository(db)
	ctx := context.Background()
	if err := repo.Create(ctx, &model.AgentExecution{RunID: "run-one", ExecutionID: "execution-one", Provider: "fake", Status: model.AgentExecutionStatusAccepted}); err != nil {
		t.Fatalf("create first projection: %v", err)
	}
	if err := repo.Create(ctx, &model.AgentExecution{RunID: "run-one", ExecutionID: "execution-two", Provider: "fake", Status: model.AgentExecutionStatusAccepted}); err == nil {
		t.Fatal("expected duplicate run id to fail")
	}
	if err := repo.Create(ctx, &model.AgentExecution{RunID: "run-two", ExecutionID: "execution-one", Provider: "fake", Status: model.AgentExecutionStatusAccepted}); err == nil {
		t.Fatal("expected duplicate execution id to fail")
	}
}

func TestAgentExecutionRepositoryListsOnlyActiveOldestFirst(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.AgentExecution{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewAgentExecutionRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 8, 18, 11, 0, 0, 0, time.UTC)
	older := now.Add(-time.Minute)
	newer := now
	items := []*model.AgentExecution{
		{RunID: "run-finished", ExecutionID: "run-finished", Provider: "fake", Status: model.AgentExecutionStatusSucceeded, LastObservedAt: &older},
		{RunID: "run-running-newer", ExecutionID: "run-running-newer", Provider: "fake", Status: model.AgentExecutionStatusRunning, LastObservedAt: &newer},
		{RunID: "run-accepted", ExecutionID: "run-accepted", Provider: "fake", Status: model.AgentExecutionStatusAccepted},
		{RunID: "run-starting", ExecutionID: "run-starting", Provider: "fake", Status: model.AgentExecutionStatusStarting, LastObservedAt: &older},
	}
	for _, item := range items {
		if err := repo.Create(ctx, item); err != nil {
			t.Fatalf("create %s: %v", item.RunID, err)
		}
	}

	active, err := repo.ListActive(ctx, 10)
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 3 {
		t.Fatalf("expected three active projections, got %#v", active)
	}
	if active[0].RunID != "run-accepted" || active[1].RunID != "run-starting" || active[2].RunID != "run-running-newer" {
		t.Fatalf("unexpected active ordering: %#v", active)
	}

	limited, err := repo.ListActive(ctx, 1)
	if err != nil || len(limited) != 1 || limited[0].RunID != "run-accepted" {
		t.Fatalf("unexpected limited active list: items=%#v err=%v", limited, err)
	}
}
