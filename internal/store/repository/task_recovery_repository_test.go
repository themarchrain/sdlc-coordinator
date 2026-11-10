package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
)

func TestTaskRecoveryRepositoryGetByRequest(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db, &model.TaskRecovery{}); err != nil {
		t.Fatal(err)
	}
	repo := NewTaskRecoveryRepository(db)
	row := &model.TaskRecovery{
		TaskID: "task-recovery-query", RequestID: "request-1", RequestHash: "hash-1", SourceGeneration: 3,
		TargetGeneration: 4, BlockRevision: "revision-1", RequestedBy: "user-1", RequestedAt: time.Now().UTC(),
		Classification: "unknown", AttemptCountBefore: 5, CycleMaxAttempts: 3,
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByRequest(context.Background(), row.TaskID, row.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if got.RecoveryID != row.RecoveryID || got.TaskID != row.TaskID || got.RequestID != row.RequestID {
		t.Fatalf("unexpected recovery row: %+v", got)
	}
	if _, err := repo.GetByRequest(context.Background(), row.TaskID, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing request error = %v, want ErrNotFound", err)
	}
}

func TestTaskRecoveryUniqueKeys(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db, &model.TaskRecovery{}); err != nil {
		t.Fatal(err)
	}
	base := model.TaskRecovery{
		TaskID: "task-recovery-unique", RequestID: "request-1", RequestHash: "hash-1", SourceGeneration: 3,
		TargetGeneration: 4, BlockRevision: "revision-1", RequestedBy: "user-1", RequestedAt: time.Now().UTC(),
		Classification: "unknown", AttemptCountBefore: 5, CycleMaxAttempts: 3,
	}
	if err := db.Create(&base).Error; err != nil {
		t.Fatal(err)
	}
	sameRequest := base
	sameRequest.ID, sameRequest.RecoveryID = 0, ""
	sameRequest.SourceGeneration = 4
	if err := db.Create(&sameRequest).Error; err == nil {
		t.Fatal("duplicate task/request was accepted")
	}
	sameGeneration := base
	sameGeneration.ID, sameGeneration.RecoveryID = 0, ""
	sameGeneration.RequestID = "request-2"
	if err := db.Create(&sameGeneration).Error; err == nil {
		t.Fatal("duplicate task/source generation was accepted")
	}
}
