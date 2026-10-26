package repository

import (
	"context"
	"gorm.io/gorm"
	"sdlc-coordinator/internal/model"
	"time"
)

func stoppedCleanupQuery(db *gorm.DB, now time.Time) *gorm.DB {
	leaseScope := "? || runs.run_id"
	var leaseNow any = now
	if db.Dialector.Name() == "mysql" {
		leaseScope = "CONCAT(?, runs.run_id)"
		leaseNow = gorm.Expr("NOW(6)")
	}
	return db.Model(&model.Run{}).
		Where("runs.status IN ?", []model.RunStatus{model.RunStatusInterrupted, model.RunStatusSucceeded, model.RunStatusFailed, model.RunStatusBlocked}).
		Where("EXISTS (SELECT 1 FROM stop_requests s WHERE s.run_id = runs.run_id AND s.task_id = runs.task_id AND s.delivery_status = ? AND s.confirmation_kind IN ?)", model.StopDeliveryConfirmed, []model.StopConfirmationKind{model.StopConfirmationNoRemoteTarget, model.StopConfirmationCreateBlocked, model.StopConfirmationExecutionExited}).
		Where("NOT EXISTS (SELECT 1 FROM work_item_leases l WHERE l.scope = "+leaseScope+" AND l.lease_until > ?)", "developer-delivery:", leaseNow).
		Where("NOT EXISTS (SELECT 1 FROM action_admissions a WHERE a.run_id = runs.run_id AND a.kind IN ? AND a.not_sent_at IS NULL AND a.completed_at IS NULL)", []model.ActionKind{model.ActionGitCommit, model.ActionGitPush}).
		Where("NOT EXISTS (SELECT 1 FROM action_admissions a WHERE a.run_id = runs.run_id AND a.kind = ? AND a.not_sent_at IS NULL AND NOT EXISTS (SELECT 1 FROM merge_gate_audits m WHERE m.task_id = a.task_id AND m.run_id = a.run_id AND m.decision = ?))", model.ActionAutoMerge, "merged").
		Where("NOT EXISTS (SELECT 1 FROM action_admissions a WHERE a.run_id = runs.run_id AND a.kind = ? AND a.not_sent_at IS NULL AND NOT EXISTS (SELECT 1 FROM writebacks w WHERE w.dedupe_key = a.action_key AND w.task_id = a.task_id AND w.run_id = a.run_id AND w.status = ?))", model.ActionWriteback, model.WritebackStatusSucceeded).
		Where("NOT EXISTS (SELECT 1 FROM writebacks w WHERE w.run_id = runs.run_id AND w.status NOT IN ?)", []model.WritebackStatus{model.WritebackStatusSucceeded, model.WritebackStatusSkipped}).
		Where("NOT EXISTS (SELECT 1 FROM effects e WHERE (e.run_id = runs.run_id OR (COALESCE(e.run_id, '') = '' AND e.task_id = runs.task_id)) AND e.status NOT IN ?)", []model.EffectStatus{model.EffectStatusSucceeded, model.EffectStatusSkipped})
}

func (r *StopRepository) cleanupQuery(ctx context.Context, db *gorm.DB, now time.Time) *gorm.DB {
	return stoppedCleanupQuery(db.WithContext(ctx), now)
}

func (r *StopRepository) CleanupReady(ctx context.Context, runID string) (bool, error) {
	var count int64
	err := r.cleanupQuery(ctx, r.db, time.Now().UTC()).Where("runs.run_id = ?", runID).Count(&count).Error
	return count == 1, err
}

func (r *StopRepository) ClaimCleanup(ctx context.Context, runID, claimID string, now, staleBefore time.Time) (bool, error) {
	var run model.Run
	if err := r.db.WithContext(ctx).Where("run_id = ?", runID).First(&run).Error; err != nil {
		return false, stopLookupError(err)
	}
	claimed := false
	err := withLockedTask(ctx, r.db, run.TaskID, func(tx *gorm.DB, _ *model.Task, dbNow time.Time) error {
		var count int64
		if err := r.cleanupQuery(ctx, tx, dbNow).Where("runs.run_id = ?", runID).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return nil
		}
		result := tx.Model(&model.Run{}).Where("run_id = ? AND artifacts_purged_at IS NULL AND (artifacts_purge_started_at IS NULL OR artifacts_purge_started_at <= ?)", runID, staleBefore).Updates(map[string]any{"artifacts_purge_started_at": now, "artifacts_purge_claim_id": claimID})
		claimed = result.RowsAffected == 1
		return result.Error
	})
	return claimed, err
}

func (r *StopRepository) ListCleanupReady(ctx context.Context, before, staleBefore time.Time, limit int) ([]model.Run, error) {
	if limit <= 0 || limit > 10 {
		limit = 10
	}
	var runs []model.Run
	err := r.cleanupQuery(ctx, r.db, time.Now().UTC()).Where("completed_at <= ? AND artifacts_purged_at IS NULL AND (artifacts_purge_started_at IS NULL OR artifacts_purge_started_at <= ?)", before, staleBefore).Order("completed_at ASC, runs.id ASC").Limit(limit).Find(&runs).Error
	return runs, err
}
