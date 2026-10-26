package repository

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/storetest"
	"sync"
	"testing"
	"time"
)

func TestCleanupWaitsForExitAndGit(t *testing.T) {
	db, stops, task := stopFixture(t, model.TaskStatusRunning)
	run, _ := bindStopExecution(t, db, task, model.RunStatusRunning)
	if err := store.Migrate(db, &model.WorkItemLease{}, &model.Writeback{}, &model.Effect{}, &model.MergeGateAudit{}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	decision, err := stops.Stop(ctx, StopTarget{TaskID: task.TaskID}, "admin", "", 3)
	if err != nil {
		t.Fatal(err)
	}
	check := func(want bool) {
		t.Helper()
		got, err := stops.CleanupReady(ctx, run.RunID)
		if err != nil || got != want {
			t.Fatalf("cleanup ready=%v want=%v err=%v", got, want, err)
		}
	}
	check(false)
	if err := db.Model(decision.Stop).Updates(map[string]any{"delivery_status": model.StopDeliveryConfirmed, "confirmation_kind": model.StopConfirmationExecutionExited}).Error; err != nil {
		t.Fatal(err)
	}
	check(true)
	lease := model.WorkItemLease{Scope: "developer-delivery:" + run.RunID, Owner: "delivery", Generation: 1, LeaseUntil: time.Now().Add(time.Hour)}
	if err := db.Create(&lease).Error; err != nil {
		t.Fatal(err)
	}
	check(false)
	if err := db.Delete(&lease).Error; err != nil {
		t.Fatal(err)
	}
	admission := &model.ActionAdmission{AdmissionID: "git", TaskID: task.TaskID, RunID: run.RunID, Kind: model.ActionGitPush, ActionKey: "push-" + run.RunID, Attempt: 1, LeaseOwner: "delivery", LeaseGeneration: 1, AdmittedAt: time.Now()}
	if err := db.Create(admission).Error; err != nil {
		t.Fatal(err)
	}
	check(false)
	if err := NewActionAdmissionRepository(db).ConfirmCompleted(ctx, admission); err != nil {
		t.Fatal(err)
	}
	check(true)
	wb := &model.Writeback{TaskID: task.TaskID, RunID: run.RunID, DedupeKey: "wb", Kind: "comment", Status: model.WritebackStatusUnknown}
	if err := db.Create(wb).Error; err != nil {
		t.Fatal(err)
	}
	check(false)
	if err := db.Model(wb).Update("status", model.WritebackStatusSucceeded).Error; err != nil {
		t.Fatal(err)
	}
	effect := &model.Effect{TaskID: task.TaskID, RunID: run.RunID, DedupeKey: "im", Kind: "im_message", Status: model.EffectStatusDeliveryUnverified}
	if err := db.Create(effect).Error; err != nil {
		t.Fatal(err)
	}
	check(false)
	if err := db.Model(effect).Update("status", model.EffectStatusSkipped).Error; err != nil {
		t.Fatal(err)
	}
	check(true)
	if err := db.Model(wb).Update("status", model.WritebackStatusSkipped).Error; err != nil {
		t.Fatal(err)
	}
	permit := &model.ActionAdmission{AdmissionID: "unknown-wb", TaskID: task.TaskID, RunID: run.RunID, Kind: model.ActionWriteback, ActionKey: wb.DedupeKey, Attempt: 1, LeaseOwner: "sender", LeaseGeneration: 1, AdmittedAt: time.Now()}
	if err := db.Create(permit).Error; err != nil {
		t.Fatal(err)
	}
	check(false)
	if err := NewActionAdmissionRepository(db).ConfirmNotSent(ctx, permit); err != nil {
		t.Fatal(err)
	}
	check(true)
	auto := &model.ActionAdmission{AdmissionID: "auto", TaskID: task.TaskID, RunID: run.RunID, Kind: model.ActionAutoMerge, ActionKey: task.TaskID, Attempt: 1, LeaseOwner: "merge", LeaseGeneration: 1, AdmittedAt: time.Now()}
	if err := db.Create(auto).Error; err != nil {
		t.Fatal(err)
	}
	check(false)
	if err := db.Create(&model.MergeGateAudit{AuditID: "merged", TaskID: task.TaskID, RunID: run.RunID, Decision: "merged"}).Error; err != nil {
		t.Fatal(err)
	}
	check(true)
	if err := db.Model(&model.MergeGateAudit{}).Where("audit_id = ?", "merged").Update("decision", "fallback").Error; err != nil {
		t.Fatal(err)
	}
	check(false)
	if err := NewActionAdmissionRepository(db).ConfirmNotSent(ctx, auto); err != nil {
		t.Fatal(err)
	}
	check(true)
	claimed, err := stops.ClaimCleanup(ctx, run.RunID, "claim", time.Now(), time.Now().Add(-time.Hour))
	if err != nil || !claimed {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	claimed, err = stops.ClaimCleanup(ctx, run.RunID, "other", time.Now(), time.Now().Add(-time.Hour))
	if err != nil || claimed {
		t.Fatalf("duplicate claim=%v err=%v", claimed, err)
	}
}

func TestStoppedCleanupClaimMySQL(t *testing.T) {
	dsn := storetest.NewDatabase(t, storetest.RequireMySQL(t), "stopped_cleanup")
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	for _, handle := range []*gorm.DB{db, other} {
		sqlDB, _ := handle.DB()
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.StopRequest{}, &model.WorkItemLease{}, &model.ActionAdmission{}, &model.Writeback{}, &model.Effect{}, &model.MergeGateAudit{}); err != nil {
		t.Fatal(err)
	}
	runID := "run"
	now := time.Now().UTC()
	old := now.Add(-8 * 24 * time.Hour)
	for _, row := range []any{&model.Task{TaskID: "task", DedupeKey: "task", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusInterrupted, CurrentRunID: &runID}, &model.Run{RunID: runID, TaskID: "task", Status: model.RunStatusInterrupted, CompletedAt: &old}, &model.StopRequest{StopID: "stop", DedupeKey: "task:task", TaskID: "task", RunID: runID, Scope: "task", RequestedBy: "admin", RequestedAt: now, DeliveryStatus: model.StopDeliveryConfirmed, ConfirmationKind: model.StopConfirmationExecutionExited}} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	repo := NewStopRepository(db)
	candidates, err := repo.ListCleanupReady(ctx, now.Add(-7*24*time.Hour), now.Add(-time.Hour), 10)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("candidates=%+v %v", candidates, err)
	}
	start := make(chan struct{})
	results := make(chan bool, 2)
	var wg sync.WaitGroup
	for i, handle := range []*gorm.DB{db, other} {
		wg.Add(1)
		go func(i int, handle *gorm.DB) {
			defer wg.Done()
			<-start
			won, err := NewStopRepository(handle).ClaimCleanup(ctx, runID, fmt.Sprintf("claim-%d", i), now, now.Add(-time.Hour))
			if err != nil {
				t.Error(err)
			}
			results <- won
		}(i, handle)
	}
	close(start)
	wg.Wait()
	close(results)
	wins := 0
	for won := range results {
		if won {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("cleanup claim winners=%d", wins)
	}
}

func TestGitCompletionReceiptFencedAndRetainedAfterStop(t *testing.T) {
	db, repo, request := actionAdmissionFixture(t, model.ActionGitCommit)
	admission, err := repo.AdmitAction(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewStopRepository(db).Stop(context.Background(), StopTarget{TaskID: request.TaskID}, "admin", "", 3); err != nil {
		t.Fatal(err)
	}
	stale := *admission
	stale.LeaseGeneration++
	if err := repo.ConfirmCompleted(context.Background(), &stale); err == nil {
		t.Fatal("forged completion accepted")
	}
	if err := repo.ConfirmCompleted(context.Background(), admission); err != nil {
		t.Fatal(err)
	}
	var got model.ActionAdmission
	if err := db.First(&got, admission.ID).Error; err != nil || got.CompletedAt == nil || got.NotSentAt != nil {
		t.Fatalf("receipt missing: %+v %v", got, err)
	}
}
