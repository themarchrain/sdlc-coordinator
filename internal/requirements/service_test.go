package requirements

import (
	"context"
	"errors"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"

	"gorm.io/gorm"
)

func newTestService(t *testing.T) (*Service, *Store) {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	requirementStore := NewStore(db)
	if err := requirementStore.Migrate(); err != nil {
		t.Fatalf("migrate requirements: %v", err)
	}
	repoStore := registeredrepo.NewStore(db)
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate repos: %v", err)
	}
	ctx := context.Background()
	if err := repoStore.Create(ctx, &registeredrepo.RegisteredRepository{
		RepositoryID: "repo-1", DisplayName: "Demo", PathWithNamespace: "group/demo", Status: registeredrepo.StatusEnabled,
	}); err != nil {
		t.Fatalf("seed repo: %v", err)
	}
	return NewService(requirementStore, repoStore, nil), requirementStore
}

func TestWithDBRebindsWorkItemLeases(t *testing.T) {
	service, _ := newTestService(t)
	tenantDB, err := store.Open("file:" + t.Name() + "_tenant?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(tenantDB, &model.WorkItemLease{}); err != nil {
		t.Fatal(err)
	}
	service.WithWorkItemLeaseFactory(func(db *gorm.DB) WorkItemLeasePort {
		return repository.NewWorkItemLeaseRepository(db)
	}, "instance-a")
	bound := service.WithDB(tenantDB)
	lease, won, err := bound.leases.Acquire(context.Background(), "requirements-publish:test", "instance-a:requirements", "publish", time.Minute)
	if err != nil || !won || lease.Scope != "requirements-publish:test" {
		t.Fatalf("tenant lease acquisition: won=%v lease=%+v err=%v", won, lease, err)
	}
	if bound.instanceID != "instance-a" {
		t.Fatalf("instance identity lost during tenant binding: %q", bound.instanceID)
	}
}

func TestServiceCreateValidatesAndQueues(t *testing.T) {
	service, _ := newTestService(t)
	ctx := context.Background()

	if _, err := service.Create(ctx, CreateRequest{Title: "  ", RepositoryID: "repo-1"}); !errors.Is(err, ErrInvalidRequirement) {
		t.Fatalf("empty title must be invalid, got %v", err)
	}
	if _, err := service.Create(ctx, CreateRequest{Title: "x", RepositoryID: " "}); !errors.Is(err, ErrInvalidRequirement) {
		t.Fatalf("empty repository must be invalid, got %v", err)
	}
	if _, err := service.Create(ctx, CreateRequest{Title: "x", RepositoryID: "repo-1", Priority: "urgent"}); !errors.Is(err, ErrInvalidRequirement) {
		t.Fatalf("bad priority must be invalid, got %v", err)
	}
	if _, err := service.Create(ctx, CreateRequest{Title: "x", RepositoryID: "repo-missing"}); !errors.Is(err, registeredrepo.ErrNotFound) {
		t.Fatalf("missing repository must map to ErrNotFound, got %v", err)
	}

	item, err := service.Create(ctx, CreateRequest{
		Title: "重构登录", Description: "用新的设计", RepositoryID: "repo-1",
		Labels: []string{"frontend", " "}, Priority: "high",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if item.Status != StatusQueued || item.Priority != "high" {
		t.Fatalf("unexpected item: %+v", item)
	}
	labels := item.LabelList()
	if len(labels) != 1 || labels[0] != "frontend" {
		t.Fatalf("labels must be trimmed, got %v", labels)
	}
}

func TestServiceCreateRejectsDisabledRepository(t *testing.T) {
	service, _ := newTestService(t)
	repo, err := service.repositories.Get(context.Background(), "repo-1")
	if err != nil {
		t.Fatalf("get repo: %v", err)
	}
	repo.Status = registeredrepo.StatusDisabled
	if err := service.repositories.Update(context.Background(), repo); err != nil {
		t.Fatalf("disable repo: %v", err)
	}
	if _, err := service.Create(context.Background(), CreateRequest{Title: "x", RepositoryID: "repo-1"}); !errors.Is(err, ErrRepositoryNotEnabled) {
		t.Fatalf("disabled repository must be rejected, got %v", err)
	}
}

func TestServicePatchOnlyInEditableStates(t *testing.T) {
	service, _ := newTestService(t)
	ctx := context.Background()
	item, err := service.Create(ctx, CreateRequest{Title: "初稿", RepositoryID: "repo-1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	newTitle := "改标题"
	patched, err := service.Patch(ctx, item.RequirementID, PatchRequest{Title: &newTitle})
	if err != nil || patched.Title != "改标题" {
		t.Fatalf("patch queued: err=%v title=%s", err, patched.Title)
	}
	item.Status = StatusPublished
	if err := service.store.Update(ctx, item); err != nil {
		t.Fatalf("force publish: %v", err)
	}
	if _, err := service.Patch(ctx, item.RequirementID, PatchRequest{Title: &newTitle}); !errors.Is(err, ErrNotEditable) {
		t.Fatalf("published must not be editable, got %v", err)
	}
}

func TestServiceTransitionGuards(t *testing.T) {
	service, requirementStore := newTestService(t)
	ctx := context.Background()
	item, err := service.Create(ctx, CreateRequest{Title: "x", RepositoryID: "repo-1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := service.Complete(ctx, item.RequirementID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("queued cannot complete, got %v", err)
	}
	cancelled, err := service.Cancel(ctx, item.RequirementID)
	if err != nil || cancelled.Status != StatusCancelled {
		t.Fatalf("cancel queued: %v %+v", err, cancelled)
	}
	if _, err := service.Cancel(ctx, item.RequirementID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("cancelled cannot cancel again, got %v", err)
	}

	retry := &Requirement{Title: "y", RepositoryID: "repo-1", Status: StatusFailed, LastError: "boom"}
	if err := requirementStore.Create(ctx, retry); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	requeued, err := service.Publish(ctx, retry.RequirementID)
	if err != nil || requeued.Status != StatusQueued || requeued.LastError != "" {
		t.Fatalf("publish failed item: %v %+v", err, requeued)
	}
	if _, err := service.Publish(ctx, item.RequirementID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("cancelled cannot publish, got %v", err)
	}

	item.Status = StatusPublished
	item.IssueIID = 3
	if err := requirementStore.Update(ctx, item); err != nil {
		t.Fatalf("force publish: %v", err)
	}
	done, err := service.Complete(ctx, item.RequirementID)
	if err != nil || done.Status != StatusDone || done.CompletedAt == nil {
		t.Fatalf("complete published: %v %+v", err, done)
	}
	if _, err := service.Get(ctx, "req_missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing must map to ErrNotFound, got %v", err)
	}
}
