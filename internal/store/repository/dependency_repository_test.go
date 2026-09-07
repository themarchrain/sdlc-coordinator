package repository

import (
	"context"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
)

func TestDependencyRepositoryCreateAndList(t *testing.T) {
	db, err := store.Open("file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Dependency{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewDependencyRepository(db)
	ctx := context.Background()

	dependency := &model.Dependency{
		ParentTaskID: "task-parent-1",
		ChildTaskID:  "task-child-1",
		Relation:     "upstream_fix",
		Status:       model.DependencyStatusPending,
		MetadataJSON: `{"reason":"framework failure"}`,
	}
	if err := repo.Create(ctx, dependency); err != nil {
		t.Fatalf("create dependency: %v", err)
	}

	byParent, err := repo.ListByParentTaskID(ctx, "task-parent-1")
	if err != nil {
		t.Fatalf("list by parent: %v", err)
	}
	if len(byParent) != 1 {
		t.Fatalf("expected one dependency by parent, got %d", len(byParent))
	}
	if byParent[0].ChildTaskID != "task-child-1" {
		t.Fatalf("expected child task task-child-1, got %s", byParent[0].ChildTaskID)
	}

	byChild, err := repo.ListByChildTaskID(ctx, "task-child-1")
	if err != nil {
		t.Fatalf("list by child: %v", err)
	}
	if len(byChild) != 1 {
		t.Fatalf("expected one dependency by child, got %d", len(byChild))
	}
	if byChild[0].ParentTaskID != "task-parent-1" {
		t.Fatalf("expected parent task task-parent-1, got %s", byChild[0].ParentTaskID)
	}
}

func TestDependencyRepositoryUpsertAndUpdate(t *testing.T) {
	db, err := store.Open("file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Dependency{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewDependencyRepository(db)
	ctx := context.Background()

	first := &model.Dependency{
		ParentTaskID: "task-parent-upsert",
		ChildTaskID:  "task-child-upsert",
		Relation:     "upstream_fix",
		Status:       model.DependencyStatusPending,
		MetadataJSON: `{"reason":"initial"}`,
	}
	if err := repo.UpsertByIdentity(ctx, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	second := &model.Dependency{
		ParentTaskID: "task-parent-upsert",
		ChildTaskID:  "task-child-upsert",
		Relation:     "upstream_fix",
		Status:       model.DependencyStatusBlocked,
		MetadataJSON: `{"reason":"updated"}`,
	}
	if err := repo.UpsertByIdentity(ctx, second); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	dependencies, err := repo.ListByParentTaskID(ctx, "task-parent-upsert")
	if err != nil {
		t.Fatalf("list by parent: %v", err)
	}
	if len(dependencies) != 1 {
		t.Fatalf("expected one upserted dependency, got %d", len(dependencies))
	}
	if dependencies[0].Status != model.DependencyStatusBlocked {
		t.Fatalf("expected blocked status after upsert, got %s", dependencies[0].Status)
	}
	if dependencies[0].MetadataJSON != `{"reason":"updated"}` {
		t.Fatalf("expected updated metadata, got %s", dependencies[0].MetadataJSON)
	}

	dependencies[0].Status = model.DependencyStatusResolved
	if err := repo.Update(ctx, &dependencies[0]); err != nil {
		t.Fatalf("update dependency: %v", err)
	}

	updated, err := repo.ListByChildTaskID(ctx, "task-child-upsert")
	if err != nil {
		t.Fatalf("list by child: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one updated dependency, got %d", len(updated))
	}
	if updated[0].Status != model.DependencyStatusResolved {
		t.Fatalf("expected resolved status, got %s", updated[0].Status)
	}
}

func TestDependencyRepositoryUpsertDoesNotDowngradeResolvedDependency(t *testing.T) {
	db, err := store.Open("file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Dependency{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewDependencyRepository(db)
	ctx := context.Background()

	first := &model.Dependency{
		ParentTaskID: "task-parent-replay",
		ChildTaskID:  "task-child-replay",
		Relation:     "upstream_fix",
		Status:       model.DependencyStatusResolved,
		MetadataJSON: `{"reason":"resolved"}`,
	}
	if err := repo.UpsertByIdentity(ctx, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	replayed := &model.Dependency{
		ParentTaskID: "task-parent-replay",
		ChildTaskID:  "task-child-replay",
		Relation:     "upstream_fix",
		Status:       model.DependencyStatusPending,
		MetadataJSON: `{"reason":"replayed"}`,
	}
	if err := repo.UpsertByIdentity(ctx, replayed); err != nil {
		t.Fatalf("replayed upsert: %v", err)
	}

	dependencies, err := repo.ListByParentTaskID(ctx, "task-parent-replay")
	if err != nil {
		t.Fatalf("list by parent: %v", err)
	}
	if len(dependencies) != 1 {
		t.Fatalf("expected one dependency, got %d", len(dependencies))
	}
	if dependencies[0].Status != model.DependencyStatusResolved {
		t.Fatalf("expected resolved dependency to stay resolved, got %s", dependencies[0].Status)
	}
	if dependencies[0].MetadataJSON != `{"reason":"replayed"}` {
		t.Fatalf("expected replay metadata to be stored, got %s", dependencies[0].MetadataJSON)
	}
}
