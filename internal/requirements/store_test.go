package requirements

import (
	"context"
	"errors"
	"strings"
	"testing"

	"sdlc-coordinator/internal/store"

	"gorm.io/gorm"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	requirementStore := NewStore(db)
	if err := requirementStore.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return requirementStore
}

func TestStoreCreateDefaultsQueuedAndGeneratesID(t *testing.T) {
	requirementStore := newTestStore(t)
	item := &Requirement{Title: "登录页重构", RepositoryID: "repo-1", Labels: EncodeLabels([]string{"frontend"})}
	if err := requirementStore.Create(context.Background(), item); err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.HasPrefix(item.RequirementID, "req_") {
		t.Fatalf("requirement id must be generated, got %q", item.RequirementID)
	}
	if item.Status != StatusQueued {
		t.Fatalf("default status must be queued, got %q", item.Status)
	}
	got, err := requirementStore.Get(context.Background(), item.RequirementID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "登录页重构" || len(got.LabelList()) != 1 || got.LabelList()[0] != "frontend" {
		t.Fatalf("unexpected row: %+v", got)
	}
}

func TestStoreRejectsDuplicateRequirementID(t *testing.T) {
	requirementStore := newTestStore(t)
	ctx := context.Background()
	if err := requirementStore.Create(ctx, &Requirement{RequirementID: "req_dup", Title: "a", RepositoryID: "r"}); err != nil {
		t.Fatalf("create first: %v", err)
	}
	if err := requirementStore.Create(ctx, &Requirement{RequirementID: "req_dup", Title: "b", RepositoryID: "r"}); err == nil {
		t.Fatal("duplicate requirement_id must fail")
	}
}

func TestStoreListProcessableReturnsQueuedAndFailedInOrder(t *testing.T) {
	requirementStore := newTestStore(t)
	ctx := context.Background()
	seed := []*Requirement{
		{Title: "a", RepositoryID: "r", Status: StatusDone},
		{Title: "b", RepositoryID: "r", Status: StatusQueued},
		{Title: "c", RepositoryID: "r", Status: StatusFailed},
		{Title: "d", RepositoryID: "r", Status: StatusCancelled},
	}
	for _, item := range seed {
		if err := requirementStore.Create(ctx, item); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	items, err := requirementStore.ListProcessable(ctx, []Status{StatusQueued, StatusFailed}, 10)
	if err != nil {
		t.Fatalf("list processable: %v", err)
	}
	if len(items) != 2 || items[0].Title != "b" || items[1].Title != "c" {
		t.Fatalf("unexpected processable order: %+v", items)
	}
	items, err = requirementStore.ListProcessable(ctx, []Status{StatusQueued, StatusFailed}, 1)
	if err != nil || len(items) != 1 || items[0].Title != "b" {
		t.Fatalf("limit must apply: %+v err=%v", items, err)
	}
}

func TestStoreListFiltersAndPaginates(t *testing.T) {
	requirementStore := newTestStore(t)
	ctx := context.Background()
	seed := []*Requirement{
		{Title: "a", RepositoryID: "r1", Status: StatusQueued},
		{Title: "b", RepositoryID: "r2", Status: StatusPublished},
		{Title: "c", RepositoryID: "r1", Status: StatusDone},
	}
	for _, item := range seed {
		if err := requirementStore.Create(ctx, item); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	items, total, err := requirementStore.List(ctx, ListFilter{RepositoryID: "r1", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("repository filter failed: total=%d items=%d", total, len(items))
	}
	items, total, err = requirementStore.List(ctx, ListFilter{Status: StatusPublished, Page: 1, PageSize: 20})
	if err != nil || total != 1 || items[0].Title != "b" {
		t.Fatalf("status filter failed: total=%d err=%v", total, err)
	}
}

func TestStoreGetMissingReturnsRecordNotFound(t *testing.T) {
	requirementStore := newTestStore(t)
	_, err := requirementStore.Get(context.Background(), "req_missing")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected gorm.ErrRecordNotFound, got %v", err)
	}
}
