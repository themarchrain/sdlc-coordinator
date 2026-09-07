package repository

import (
	"context"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
)

func TestArtifactRepositoryCreateAndList(t *testing.T) {
	db, err := store.Open("file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Artifact{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewArtifactRepository(db)
	ctx := context.Background()

	if err := repo.Create(ctx, &model.Artifact{
		TaskID:       "task-artifact-1",
		RunID:        "run-artifact-1",
		Kind:         "merge_request",
		Name:         "mr_url",
		URL:          "https://gitlab.example/group/project/-/merge_requests/1",
		MetadataJSON: `{"source_key":"mr_url","value":"https://gitlab.example/group/project/-/merge_requests/1"}`,
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	byTask, err := repo.ListByTaskID(ctx, "task-artifact-1")
	if err != nil {
		t.Fatalf("list by task: %v", err)
	}
	if len(byTask) != 1 {
		t.Fatalf("expected one task artifact, got %d", len(byTask))
	}
	if byTask[0].Kind != "merge_request" {
		t.Fatalf("expected merge_request kind, got %s", byTask[0].Kind)
	}

	byRun, err := repo.ListByRunID(ctx, "run-artifact-1")
	if err != nil {
		t.Fatalf("list by run: %v", err)
	}
	if len(byRun) != 1 {
		t.Fatalf("expected one run artifact, got %d", len(byRun))
	}
	if byRun[0].URL != "https://gitlab.example/group/project/-/merge_requests/1" {
		t.Fatalf("unexpected artifact url: %s", byRun[0].URL)
	}
}

func TestArtifactRepositoryUpsertByIdentity(t *testing.T) {
	db, err := store.Open("file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Artifact{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewArtifactRepository(db)
	ctx := context.Background()

	first := &model.Artifact{
		TaskID:       "task-artifact-upsert",
		RunID:        "run-artifact-upsert",
		Kind:         "merge_request",
		Name:         "mr_url",
		URL:          "https://gitlab.example/group/project/-/merge_requests/1",
		MetadataJSON: `{"source_key":"mr_url","value":"https://gitlab.example/group/project/-/merge_requests/1"}`,
	}
	if err := repo.UpsertByIdentity(ctx, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	second := &model.Artifact{
		TaskID:       "task-artifact-upsert",
		RunID:        "run-artifact-upsert",
		Kind:         "merge_request",
		Name:         "mr_url",
		URL:          "https://gitlab.example/group/project/-/merge_requests/2",
		MetadataJSON: `{"source_key":"mr_url","value":"https://gitlab.example/group/project/-/merge_requests/2"}`,
	}
	if err := repo.UpsertByIdentity(ctx, second); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	artifacts, err := repo.ListByRunID(ctx, "run-artifact-upsert")
	if err != nil {
		t.Fatalf("list by run: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected one upserted artifact, got %d", len(artifacts))
	}
	if artifacts[0].URL != "https://gitlab.example/group/project/-/merge_requests/2" {
		t.Fatalf("expected updated URL, got %s", artifacts[0].URL)
	}
}
