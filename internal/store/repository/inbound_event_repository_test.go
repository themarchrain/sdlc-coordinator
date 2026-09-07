package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
)

func TestInboundEventRepositoryCreateAndGetByDedupeKey(t *testing.T) {
	db, err := store.Open("file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.InboundEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewInboundEventRepository(db)
	ctx := context.Background()

	event := &model.InboundEvent{
		DedupeKey:      "gitlab:mr:24207:12:abc123",
		Source:         "gitlab",
		EventType:      "merge_request",
		EventAction:    "update",
		ExternalID:     "24207:12",
		Status:         model.InboundEventStatusReceived,
		PayloadJSON:    `{"object_kind":"merge_request"}`,
		NormalizedJSON: `{"kind":"change_request_updated"}`,
	}
	if err := repo.Create(ctx, event); err != nil {
		t.Fatalf("create inbound event: %v", err)
	}
	if event.EventID == "" {
		t.Fatal("expected event id to be generated")
	}
	if event.ReceivedAt.IsZero() {
		t.Fatal("expected received at to be generated")
	}

	got, err := repo.GetByDedupeKey(ctx, "gitlab:mr:24207:12:abc123")
	if err != nil {
		t.Fatalf("get inbound event by dedupe key: %v", err)
	}
	if got.Source != "gitlab" || got.EventAction != "update" {
		t.Fatalf("unexpected event: %#v", got)
	}
	if got.PayloadJSON != `{"object_kind":"merge_request"}` {
		t.Fatalf("unexpected payload json: %s", got.PayloadJSON)
	}
}

func TestInboundEventRepositoryUpdate(t *testing.T) {
	db, err := store.Open("file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.InboundEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewInboundEventRepository(db)
	ctx := context.Background()

	event := &model.InboundEvent{
		DedupeKey:   "gitlab:issue:24207:42:v1",
		Source:      "gitlab",
		EventType:   "issue",
		EventAction: "open",
		ExternalID:  "24207:42",
		PayloadJSON: `{"object_kind":"issue"}`,
	}
	if err := repo.Create(ctx, event); err != nil {
		t.Fatalf("create inbound event: %v", err)
	}

	event.Status = model.InboundEventStatusNormalized
	event.NormalizedJSON = `{"kind":"work_item_updated"}`
	event.ErrorMessage = ""
	if err := repo.Update(ctx, event); err != nil {
		t.Fatalf("update inbound event: %v", err)
	}

	got, err := repo.GetByDedupeKey(ctx, "gitlab:issue:24207:42:v1")
	if err != nil {
		t.Fatalf("get inbound event by dedupe key: %v", err)
	}
	if got.Status != model.InboundEventStatusNormalized {
		t.Fatalf("expected normalized status, got %s", got.Status)
	}
	if got.NormalizedJSON != `{"kind":"work_item_updated"}` {
		t.Fatalf("unexpected normalized json: %s", got.NormalizedJSON)
	}
}

func TestInboundEventRepositoryPersistsNullableRepositoryID(t *testing.T) {
	db, err := store.Open("file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.InboundEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewInboundEventRepository(db)
	orphan := &model.InboundEvent{DedupeKey: "orphan", Source: "gitlab", EventType: "issue", PayloadJSON: "{}"}
	if err := repo.Create(context.Background(), orphan); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.GetByDedupeKey(context.Background(), "orphan")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RepositoryID != nil {
		t.Fatalf("expected nil, got %v", loaded.RepositoryID)
	}
	id := "repo_a"
	owned := &model.InboundEvent{DedupeKey: "owned", Source: "gitlab", EventType: "issue", PayloadJSON: "{}", RepositoryID: &id}
	if err := repo.Create(context.Background(), owned); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByDedupeKey(context.Background(), "owned")
	if err != nil {
		t.Fatal(err)
	}
	if got.RepositoryID == nil || *got.RepositoryID != "repo_a" {
		t.Fatalf("got %#v", got.RepositoryID)
	}
}

func TestInboundEventRepositoryListByStatusOrdersByReceivedAt(t *testing.T) {
	db, err := store.Open("file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.InboundEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewInboundEventRepository(db)
	ctx := context.Background()
	base := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)

	events := []*model.InboundEvent{
		{
			DedupeKey:  "gitlab:mr:24207:3:new",
			Source:     "gitlab",
			EventType:  "merge_request",
			Status:     model.InboundEventStatusReceived,
			ExternalID: "24207:3",
			ReceivedAt: base.Add(2 * time.Minute),
		},
		{
			DedupeKey:  "gitlab:mr:24207:1:old",
			Source:     "gitlab",
			EventType:  "merge_request",
			Status:     model.InboundEventStatusReceived,
			ExternalID: "24207:1",
			ReceivedAt: base,
		},
		{
			DedupeKey:  "gitlab:mr:24207:2:normalized",
			Source:     "gitlab",
			EventType:  "merge_request",
			Status:     model.InboundEventStatusNormalized,
			ExternalID: "24207:2",
			ReceivedAt: base.Add(time.Minute),
		},
	}
	for _, event := range events {
		if err := repo.Create(ctx, event); err != nil {
			t.Fatalf("create inbound event %s: %v", event.DedupeKey, err)
		}
	}

	received, err := repo.ListByStatus(ctx, model.InboundEventStatusReceived, 10)
	if err != nil {
		t.Fatalf("list received inbound events: %v", err)
	}
	if len(received) != 2 {
		t.Fatalf("expected two received events, got %d", len(received))
	}
	if received[0].DedupeKey != "gitlab:mr:24207:1:old" {
		t.Fatalf("expected oldest event first, got %s", received[0].DedupeKey)
	}
	if received[1].DedupeKey != "gitlab:mr:24207:3:new" {
		t.Fatalf("expected newest received event second, got %s", received[1].DedupeKey)
	}

	limited, err := repo.ListByStatus(ctx, model.InboundEventStatusReceived, 1)
	if err != nil {
		t.Fatalf("list limited inbound events: %v", err)
	}
	if len(limited) != 1 || limited[0].DedupeKey != "gitlab:mr:24207:1:old" {
		t.Fatalf("unexpected limited events: %#v", limited)
	}
}
