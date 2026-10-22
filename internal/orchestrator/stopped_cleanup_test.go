package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/workspace"
	"testing"
	"time"
)

func TestStoppedCleanupWaitsForExitAndRetainsCaches(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	f := newArtifactCleanupFixture(t, root)
	if err := store.Migrate(f.db, &model.StopRequest{}, &model.ActionAdmission{}, &model.WorkItemLease{}, &model.MergeGateAudit{}); err != nil {
		t.Fatal(err)
	}
	run := seedFullySucceededRun(t, ctx, f, root, "stopped", f.now.Add(-8*24*time.Hour))
	if err := f.db.Model(&model.Task{}).Where("task_id = ?", run.TaskID).Update("status", model.TaskStatusRunning).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Model(run).Update("status", model.RunStatusRunning).Error; err != nil {
		t.Fatal(err)
	}
	stops := repository.NewStopRepository(f.db)
	f.orch.stops = stops
	decision, err := stops.Stop(ctx, repository.StopTarget{TaskID: run.TaskID}, "admin", "", 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.orch.cleanupExpiredArtifacts(ctx); len(got) != 0 {
		t.Fatalf("premature cleanup: %+v", got)
	}
	if err := f.db.Model(run).Update("completed_at", f.now.Add(-8*24*time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Model(decision.Stop).Updates(map[string]any{"delivery_status": model.StopDeliveryConfirmed, "confirmation_kind": model.StopConfirmationExecutionExited}).Error; err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(root, "shared-cache")
	if err := os.WriteFile(cache, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	got := f.orch.cleanupExpiredArtifacts(ctx)
	if len(got) != 1 || !got[0].Purged || len(f.store.deleted) != 1 {
		t.Fatalf("stopped cleanup missing: %+v deleted=%v", got, f.store.deleted)
	}
	if _, err := os.Stat(filepath.Join(root, "runs", run.RunID)); !os.IsNotExist(err) {
		t.Fatalf("workspace retained: %v", err)
	}
	if _, err := os.Stat(cache); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := f.db.Model(&model.StopRequest{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("stop authority lost: %v %v", count, err)
	}
}

func TestSuccessfulGitRecordsCompletion(t *testing.T) {
	db, _, _, _ := externalAdmissionFixture(t)
	f := newResultChannelFixture("DeveloperWorker")
	f.task.DedupeKey = "receipt"
	for _, row := range []any{f.task, f.run} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	f.core.SetActionAdmissions(repository.NewActionAdmissionRepository(db))
	f.core.SetDeliveryExecutionRight(repository.NewWorkItemLeaseRepository(db))
	lease, won, err := f.core.deliveryRight.Acquire(context.Background(), "developer-delivery:"+f.run.RunID, "test", "test", time.Minute)
	if err != nil || !won {
		t.Fatalf("lease: %v", err)
	}
	if _, err := f.core.commitAdmitted(context.Background(), workspace.DeveloperCommitRequest{TaskID: f.task.TaskID, RunID: f.run.RunID, DeliveryOwner: lease.Owner, DeliveryGeneration: lease.Generation}); err != nil {
		t.Fatal(err)
	}
	var admission model.ActionAdmission
	if err := db.Where("kind = ?", model.ActionGitCommit).First(&admission).Error; err != nil || admission.CompletedAt == nil {
		t.Fatalf("missing completion: %+v %v", admission, err)
	}
}
