package repository

import (
	"context"
	"errors"
	"fmt"
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

func TestRecoverySnapshotFindsLeasesBeyondTheFirstThousand(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.ResultSubmission{},
		&model.Writeback{}, &model.Effect{}, &model.DeveloperDeliveryCheckpoint{}, &model.ActionAdmission{},
		&model.ExecutionExitConfirmation{}, &model.WorkItemLease{}, &model.MergeGateAudit{}, &model.StopRequest{},
		&model.TaskRecovery{}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	runID := "run-lease-visible"
	task := &model.Task{
		TaskID: "task-lease-visible", DedupeKey: "dedupe-lease-visible", Type: "bugfix", Role: "DeveloperWorker",
		Status: model.TaskStatusBlocked, CurrentRunID: &runID, AttemptCount: 5,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusBlocked}).Error; err != nil {
		t.Fatal(err)
	}
	noise := make([]model.WorkItemLease, 0, 1000)
	for index := 0; index < 1000; index++ {
		noise = append(noise, model.WorkItemLease{
			Scope: fmt.Sprintf("aaa-unrelated:%04d", index), Owner: "owner", Generation: 1,
			LeaseUntil: now.Add(time.Hour), AcquiredAt: now, UpdatedAt: now,
		})
	}
	if err := db.CreateInBatches(&noise, 200).Error; err != nil {
		t.Fatal(err)
	}
	delivery := model.WorkItemLease{Scope: "developer-delivery:" + runID, Owner: "delivery", Generation: 1, LeaseUntil: now.Add(time.Hour), AcquiredAt: now, UpdatedAt: now}
	otherDelivery := model.WorkItemLease{Scope: "developer-delivery:run-foreign", Owner: "foreign", Generation: 1, LeaseUntil: now.Add(time.Hour), AcquiredAt: now, UpdatedAt: now}
	mergeGate := model.WorkItemLease{Scope: "merge-gate:" + task.TaskID, Owner: "gate", Generation: 1, LeaseUntil: now.Add(time.Hour), AcquiredAt: now, UpdatedAt: now}
	for _, lease := range []*model.WorkItemLease{&delivery, &otherDelivery, &mergeGate} {
		if err := db.Create(lease).Error; err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := NewTaskRecoveryRepository(db).LoadRecoverySnapshot(context.Background(), task.TaskID)
	if err != nil {
		t.Fatalf("load recovery snapshot: %v", err)
	}
	scopes := make(map[string]bool, len(snapshot.WorkItemLeases))
	for _, lease := range snapshot.WorkItemLeases {
		scopes[lease.Scope] = true
	}
	if !scopes[delivery.Scope] {
		t.Fatalf("active target execution lease was omitted: %v", scopes)
	}
	if !scopes[mergeGate.Scope] {
		t.Fatalf("merge gate lease was omitted: %v", scopes)
	}
	if scopes[otherDelivery.Scope] {
		t.Fatal("lease of another run was loaded")
	}
}

func TestRecoverySafetyDigestTracksFailureEvidence(t *testing.T) {
	task := &model.Task{TaskID: "task-digest", Status: model.TaskStatusBlocked, AttemptCount: 5}
	snapshot := func(failureJSON string) *RecoverySnapshot {
		return &RecoverySnapshot{
			Task: task,
			Runs: []model.Run{{RunID: "run-digest", TaskID: task.TaskID, Status: model.RunStatusBlocked}},
			Executions: []model.AgentExecution{{
				RunID: "run-digest", ExecutionID: "run-digest", Status: model.AgentExecutionStatusFailed,
				RequestHash: "request-hash", FailureJSON: failureJSON,
			}},
		}
	}
	unknown := `{"version":1,"stage":"result","source":"runner","code":"provider_failed","summary":"provider failed"}`
	fatal := `{"version":1,"stage":"provider","source":"provider","code":"unsupported_schema","summary":"schema rejected"}`
	if RecoverySafetyDigest(snapshot(unknown)) != RecoverySafetyDigest(snapshot(unknown)) {
		t.Fatal("digest is not stable for the same evidence")
	}
	if RecoverySafetyDigest(snapshot(unknown)) == RecoverySafetyDigest(snapshot(fatal)) {
		t.Fatal("digest ignored a classification change in the stored failure evidence")
	}
	summaryOnly := `{"version":1,"stage":"result","source":"runner","code":"provider_failed","summary":"different wording"}`
	if RecoverySafetyDigest(snapshot(unknown)) != RecoverySafetyDigest(snapshot(summaryOnly)) {
		t.Fatal("digest churned on a summary-only change")
	}
}
