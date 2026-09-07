package repository

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
)

func TestEffectRepositoryUpsertByDedupeKeyCreatesAndUpdates(t *testing.T) {
	ctx := context.Background()
	repo := newEffectRepositoryForTest(t)

	effect := &model.Effect{
		TaskID:      "task-1",
		RunID:       "run-1",
		DedupeKey:   "im:mr-review:1",
		Kind:        "send_im_message",
		Target:      "team-a",
		Critical:    false,
		Status:      model.EffectStatusPending,
		RequestJSON: `{"body":"first"}`,
	}
	if err := repo.UpsertByDedupeKey(ctx, effect); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if effect.EffectID == "" {
		t.Fatal("expected generated effect id")
	}
	firstID := effect.EffectID

	updated := &model.Effect{
		TaskID:       "task-1",
		RunID:        "run-1",
		DedupeKey:    "im:mr-review:1",
		Kind:         "send_im_message",
		Target:       "team-b",
		Critical:     true,
		Status:       model.EffectStatusPending,
		AttemptCount: 1,
		RequestJSON:  `{"body":"second"}`,
		ErrorMessage: "previous network error",
	}
	if err := repo.UpsertByDedupeKey(ctx, updated); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if updated.EffectID != firstID {
		t.Fatalf("expected stable effect id %s, got %s", firstID, updated.EffectID)
	}

	found, err := repo.GetByDedupeKey(ctx, "im:mr-review:1")
	if err != nil {
		t.Fatalf("get effect: %v", err)
	}
	if found.Target != "team-b" {
		t.Fatalf("expected updated target, got %s", found.Target)
	}
	if !found.Critical {
		t.Fatal("expected updated critical flag")
	}
	if found.RequestJSON != `{"body":"second"}` {
		t.Fatalf("expected updated request json, got %s", found.RequestJSON)
	}
	if found.AttemptCount != 1 {
		t.Fatalf("expected updated attempt count, got %d", found.AttemptCount)
	}
	if found.ErrorMessage != "previous network error" {
		t.Fatalf("expected updated error message, got %s", found.ErrorMessage)
	}
}

func TestEffectRepositoryListByRunID(t *testing.T) {
	ctx := context.Background()
	repo := newEffectRepositoryForTest(t)

	effects := []model.Effect{
		{
			TaskID:    "task-1",
			RunID:     "run-1",
			DedupeKey: "effect-1",
			Kind:      "send_im_message",
			Target:    "team-a",
			Status:    model.EffectStatusPending,
		},
		{
			TaskID:    "task-1",
			RunID:     "run-1",
			DedupeKey: "effect-2",
			Kind:      "update_source_task",
			Target:    "SRC-1",
			Status:    model.EffectStatusPending,
		},
		{
			TaskID:    "task-2",
			RunID:     "run-2",
			DedupeKey: "effect-3",
			Kind:      "send_im_message",
			Target:    "team-b",
			Status:    model.EffectStatusPending,
		},
	}
	for i := range effects {
		if err := repo.Create(ctx, &effects[i]); err != nil {
			t.Fatalf("create effect %d: %v", i, err)
		}
	}

	listed, err := repo.ListByRunID(ctx, "run-1")
	if err != nil {
		t.Fatalf("list by run id: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected two effects, got %d", len(listed))
	}
	if listed[0].DedupeKey != "effect-1" || listed[1].DedupeKey != "effect-2" {
		t.Fatalf("expected insertion order, got %s then %s", listed[0].DedupeKey, listed[1].DedupeKey)
	}
}

func TestEffectRepositoryListPending(t *testing.T) {
	ctx := context.Background()
	repo := newEffectRepositoryForTest(t)

	effects := []model.Effect{
		{
			TaskID:    "task-1",
			RunID:     "run-1",
			DedupeKey: "pending-1",
			Kind:      "send_im_message",
			Target:    "team-a",
			Status:    model.EffectStatusPending,
		},
		{
			TaskID:    "task-1",
			RunID:     "run-1",
			DedupeKey: "succeeded-1",
			Kind:      "send_im_message",
			Target:    "team-a",
			Status:    model.EffectStatusSucceeded,
		},
		{
			TaskID:    "task-2",
			RunID:     "run-2",
			DedupeKey: "pending-2",
			Kind:      "send_im_message",
			Target:    "team-b",
			Status:    model.EffectStatusPending,
		},
	}
	for i := range effects {
		if err := repo.Create(ctx, &effects[i]); err != nil {
			t.Fatalf("create effect %d: %v", i, err)
		}
	}

	listed, err := repo.ListPending(ctx, 1)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected one pending effect because of limit, got %d", len(listed))
	}
	if listed[0].DedupeKey != "pending-1" {
		t.Fatalf("expected first pending effect, got %s", listed[0].DedupeKey)
	}

	listed, err = repo.ListPending(ctx, 0)
	if err != nil {
		t.Fatalf("list pending with default limit: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected two pending effects, got %d", len(listed))
	}
	if listed[0].DedupeKey != "pending-1" || listed[1].DedupeKey != "pending-2" {
		t.Fatalf("expected pending effects in insertion order, got %s then %s", listed[0].DedupeKey, listed[1].DedupeKey)
	}
}

func newEffectRepositoryForTest(t *testing.T) *EffectRepository {
	t.Helper()

	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Effect{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewEffectRepository(db)
}
