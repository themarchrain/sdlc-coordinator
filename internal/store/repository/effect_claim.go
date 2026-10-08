package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

// effectUpdatedAtExpr 返回 updated_at 的推进表达式：MySQL 用数据库时钟
// （跨实例一致），SQLite 用本机时钟。
func (r *EffectRepository) effectUpdatedAtExpr() any {
	if r.db.Dialector.Name() == "mysql" {
		return gorm.Expr("NOW(6)")
	}
	return time.Now()
}

// Claim 原子抢占一条可执行效果并推进 attempt 计数：pending 恒可抢占；
// running 仅在停留时间超过 staleWindow 后可接管（上一执行方被判定中断）；
// 终态行不可抢占。won=false 表示执行权在其他实例手中。到期判定在 MySQL 上
// 由数据库时钟完成（跨实例时钟偏移免疫），SQLite 上使用本机时钟。
func (r *EffectRepository) Claim(ctx context.Context, dedupeKey string, staleWindow time.Duration) (*model.Effect, bool, error) {
	if r.db.Dialector.Name() == "mysql" {
		return r.claimMySQL(ctx, dedupeKey, staleWindow)
	}
	return r.claimSQLite(ctx, dedupeKey, staleWindow)
}

func (r *EffectRepository) claimMySQL(ctx context.Context, dedupeKey string, staleWindow time.Duration) (*model.Effect, bool, error) {
	res := r.db.WithContext(ctx).Exec(`
UPDATE effects SET
  status = ?, attempt_count = attempt_count + 1, error_message = '', response_json = '', updated_at = NOW(6)
WHERE dedupe_key = ?
  AND (status = ? OR (status = ? AND updated_at <= DATE_SUB(NOW(6), INTERVAL ? MICROSECOND)))`,
		model.EffectStatusRunning, dedupeKey, model.EffectStatusPending, model.EffectStatusRunning, staleWindow.Microseconds())
	if res.Error != nil {
		return nil, false, fmt.Errorf("claim effect %s: %w", dedupeKey, res.Error)
	}
	if res.RowsAffected == 0 {
		return nil, false, nil
	}
	var claimed model.Effect
	if err := r.db.WithContext(ctx).Where("dedupe_key = ?", dedupeKey).First(&claimed).Error; err != nil {
		return nil, false, fmt.Errorf("read claimed effect %s: %w", dedupeKey, err)
	}
	return &claimed, true, nil
}

func (r *EffectRepository) claimSQLite(ctx context.Context, dedupeKey string, staleWindow time.Duration) (*model.Effect, bool, error) {
	now := time.Now()
	var claimed model.Effect
	won := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.Effect
		err := tx.Where("dedupe_key = ?", dedupeKey).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read effect %s: %w", dedupeKey, err)
		}
		claimable := existing.Status == model.EffectStatusPending ||
			(existing.Status == model.EffectStatusRunning && existing.UpdatedAt.Before(now.Add(-staleWindow)))
		if !claimable {
			return nil
		}
		res := tx.Model(&model.Effect{}).
			Where("dedupe_key = ? AND (status = ? OR status = ?)", dedupeKey, model.EffectStatusPending, model.EffectStatusRunning).
			Updates(map[string]any{
				"status":        model.EffectStatusRunning,
				"attempt_count": gorm.Expr("attempt_count + 1"),
				"error_message": "",
				"response_json": "",
				"updated_at":    now,
			})
		if res.Error != nil {
			return fmt.Errorf("claim effect %s: %w", dedupeKey, res.Error)
		}
		if res.RowsAffected == 0 {
			return nil
		}
		won = true
		return tx.Where("dedupe_key = ?", dedupeKey).First(&claimed).Error
	})
	if err != nil {
		return nil, false, err
	}
	if !won {
		return nil, false, nil
	}
	return &claimed, true, nil
}

// Complete 以 attempt 代次栅栏写入终态：执行权在副作用执行期间被另一实例
// 接管后，旧持有方的回写影响 0 行且不覆盖新持有方的结果。
func (r *EffectRepository) Complete(ctx context.Context, dedupeKey string, attempt int, status model.EffectStatus, responseJSON, errorMessage string) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.Effect{}).
		Where("dedupe_key = ? AND status = ? AND attempt_count = ?", dedupeKey, model.EffectStatusRunning, attempt).
		Updates(map[string]any{
			"status":        status,
			"response_json": responseJSON,
			"error_message": errorMessage,
			"updated_at":    r.effectUpdatedAtExpr(),
		})
	if res.Error != nil {
		return false, fmt.Errorf("complete effect %s: %w", dedupeKey, res.Error)
	}
	return res.RowsAffected == 1, nil
}

// FailStaleRunning 条件终态回写：仅当效果仍停留 running 且新鲜度早于窗口时
// 置 failed（恢复路径的原子接管——恰好一个实例落终态，败者 0 行退出）。
// attempt 计数不变：回收是终止不是新的执行尝试。到期判定在 MySQL 上由数据
// 库时钟完成（与 Claim 的接管判定同一时钟源），SQLite 上使用本机时钟。
func (r *EffectRepository) FailStaleRunning(ctx context.Context, dedupeKey string, staleWindow time.Duration, errorMessage string) (bool, error) {
	if r.db.Dialector.Name() == "mysql" {
		res := r.db.WithContext(ctx).Model(&model.Effect{}).
			Where("dedupe_key = ? AND status = ? AND updated_at <= DATE_SUB(NOW(6), INTERVAL ? MICROSECOND)",
				dedupeKey, model.EffectStatusRunning, staleWindow.Microseconds()).
			Updates(map[string]any{
				"status":        model.EffectStatusFailed,
				"error_message": errorMessage,
				"updated_at":    gorm.Expr("NOW(6)"),
			})
		if res.Error != nil {
			return false, fmt.Errorf("fail stale effect %s: %w", dedupeKey, res.Error)
		}
		return res.RowsAffected == 1, nil
	}
	cutoff := time.Now().Add(-staleWindow)
	var applied bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.Effect
		err := tx.Where("dedupe_key = ?", dedupeKey).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read effect %s: %w", dedupeKey, err)
		}
		if existing.Status != model.EffectStatusRunning || !existing.UpdatedAt.Before(cutoff) {
			return nil
		}
		res := tx.Model(&model.Effect{}).
			Where("dedupe_key = ? AND status = ? AND attempt_count = ?", dedupeKey, model.EffectStatusRunning, existing.AttemptCount).
			Updates(map[string]any{
				"status":        model.EffectStatusFailed,
				"error_message": errorMessage,
				"updated_at":    time.Now(),
			})
		if res.Error != nil {
			return fmt.Errorf("fail stale effect %s: %w", dedupeKey, res.Error)
		}
		applied = res.RowsAffected == 1
		return nil
	})
	if err != nil {
		return false, err
	}
	return applied, nil
}

// SkipIfPending 条件终态回写：仅当效果仍在 pending 时置 skipped（超龄作废）。
// 双实例同时作废同一条时恰好一个生效；已被他方抢占执行的效果不受影响。
// attempt 计数不变：作废发生在任何执行尝试之前。
func (r *EffectRepository) SkipIfPending(ctx context.Context, dedupeKey string, errorMessage string) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.Effect{}).
		Where("dedupe_key = ? AND status = ?", dedupeKey, model.EffectStatusPending).
		Updates(map[string]any{
			"status":        model.EffectStatusSkipped,
			"error_message": errorMessage,
			"updated_at":    r.effectUpdatedAtExpr(),
		})
	if res.Error != nil {
		return false, fmt.Errorf("skip effect %s: %w", dedupeKey, res.Error)
	}
	return res.RowsAffected == 1, nil
}

// RequeueIfRunning 把仍由本代次持有的 running 效果退回 pending（attempt 计数
// 保留）：执行器在批次中途被停用时使用，等待渠道配置后自然重放。
func (r *EffectRepository) RequeueIfRunning(ctx context.Context, dedupeKey string, attempt int) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.Effect{}).
		Where("dedupe_key = ? AND status = ? AND attempt_count = ?", dedupeKey, model.EffectStatusRunning, attempt).
		Updates(map[string]any{
			"status":     model.EffectStatusPending,
			"updated_at": r.effectUpdatedAtExpr(),
		})
	if res.Error != nil {
		return false, fmt.Errorf("requeue effect %s: %w", dedupeKey, res.Error)
	}
	return res.RowsAffected == 1, nil
}
