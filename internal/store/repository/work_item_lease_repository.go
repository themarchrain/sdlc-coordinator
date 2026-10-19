package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

type WorkItemLeaseRepository struct {
	db *gorm.DB
}

func NewWorkItemLeaseRepository(db *gorm.DB) *WorkItemLeaseRepository {
	return &WorkItemLeaseRepository{db: db}
}

const mysqlAcquireSQL = `
INSERT INTO work_item_leases (scope, owner, generation, operation_id, lease_until, acquired_at, updated_at)
VALUES (?, ?, 1, ?, DATE_ADD(NOW(6), INTERVAL ? MICROSECOND), NOW(6), NOW(6)) AS new
ON DUPLICATE KEY UPDATE
  owner        = IF(work_item_leases.lease_until <= NOW(6) OR work_item_leases.owner = new.owner, new.owner, work_item_leases.owner),
  generation   = IF(work_item_leases.lease_until <= NOW(6) OR work_item_leases.owner = new.owner, work_item_leases.generation + 1, work_item_leases.generation),
  operation_id = IF(work_item_leases.lease_until <= NOW(6) OR work_item_leases.owner = new.owner, new.operation_id, work_item_leases.operation_id),
  acquired_at  = IF(work_item_leases.lease_until <= NOW(6) OR work_item_leases.owner = new.owner, new.acquired_at, work_item_leases.acquired_at),
  lease_until  = IF(work_item_leases.lease_until <= NOW(6) OR work_item_leases.owner = new.owner, new.lease_until, work_item_leases.lease_until),
  updated_at   = NOW(6)`

// Acquire 尝试取得工作项执行权；won=false 表示当前由其他 owner 有效持有。
// 同 owner 重入视为续期并递增代次（旧代次的在途写自然失效）。
func (r *WorkItemLeaseRepository) Acquire(ctx context.Context, scope, owner, operationID string, ttl time.Duration) (model.WorkItemLease, bool, error) {
	if r.db.Dialector.Name() == "mysql" {
		return r.acquireMySQL(ctx, scope, owner, operationID, ttl)
	}
	return r.acquireSQLite(ctx, scope, owner, operationID, ttl)
}

func (r *WorkItemLeaseRepository) acquireMySQL(ctx context.Context, scope, owner, operationID string, ttl time.Duration) (model.WorkItemLease, bool, error) {
	if err := r.db.WithContext(ctx).Exec(mysqlAcquireSQL, scope, owner, operationID, ttl.Microseconds()).Error; err != nil {
		return model.WorkItemLease{}, false, fmt.Errorf("acquire lease %s: %w", scope, err)
	}
	var row model.WorkItemLease
	if err := r.db.WithContext(ctx).Where("scope = ?", scope).First(&row).Error; err != nil {
		return model.WorkItemLease{}, false, fmt.Errorf("read back lease %s: %w", scope, err)
	}
	return row, row.Owner == owner, nil
}

func (r *WorkItemLeaseRepository) acquireSQLite(ctx context.Context, scope, owner, operationID string, ttl time.Duration) (model.WorkItemLease, bool, error) {
	now := time.Now().UTC()
	until := now.Add(ttl)
	var result model.WorkItemLease
	won := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.WorkItemLease
		err := tx.Where("scope = ?", scope).First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			result = model.WorkItemLease{
				Scope: scope, Owner: owner, Generation: 1, OperationID: operationID,
				LeaseUntil: until, AcquiredAt: now, UpdatedAt: now,
			}
			if createErr := tx.Create(&result).Error; createErr != nil {
				return fmt.Errorf("create lease %s: %w", scope, createErr)
			}
			won = true
			return nil
		case err != nil:
			return fmt.Errorf("read lease %s: %w", scope, err)
		}
		if existing.LeaseUntil.After(now) && existing.Owner != owner {
			result = existing
			return nil
		}
		existing.Owner = owner
		existing.Generation++
		existing.OperationID = operationID
		existing.LeaseUntil = until
		existing.AcquiredAt = now
		existing.UpdatedAt = now
		if saveErr := tx.Model(&model.WorkItemLease{}).Where("scope = ?", scope).Updates(map[string]any{
			"owner": existing.Owner, "generation": existing.Generation, "operation_id": existing.OperationID,
			"lease_until": existing.LeaseUntil, "acquired_at": existing.AcquiredAt, "updated_at": existing.UpdatedAt,
		}).Error; saveErr != nil {
			return fmt.Errorf("take over lease %s: %w", scope, saveErr)
		}
		result = existing
		won = true
		return nil
	})
	return result, won, err
}

// Renew 以 owner+generation 条件续租；返回是否续租成功（被接管后的旧代次
// 恒失败）。
func (r *WorkItemLeaseRepository) Renew(ctx context.Context, scope, owner string, generation uint64, ttl time.Duration) (bool, error) {
	query := r.db.WithContext(ctx).Model(&model.WorkItemLease{}).
		Where("scope = ? AND owner = ? AND generation = ?", scope, owner, generation)
	var result *gorm.DB
	if r.db.Dialector.Name() == "mysql" {
		result = query.Updates(map[string]any{
			"lease_until": gorm.Expr("DATE_ADD(NOW(6), INTERVAL ? MICROSECOND)", ttl.Microseconds()),
			"updated_at":  gorm.Expr("NOW(6)"),
		})
	} else {
		now := time.Now().UTC()
		result = query.Updates(map[string]any{
			"lease_until": now.Add(ttl), "updated_at": now,
		})
	}
	if result.Error != nil {
		return false, fmt.Errorf("renew lease %s: %w", scope, result.Error)
	}
	return result.RowsAffected == 1, nil
}

// Release 条件删除（完成/显式归还）；被接管后的旧代次删除失败为常态，
// 不构成错误。
func (r *WorkItemLeaseRepository) Release(ctx context.Context, scope, owner string, generation uint64) (bool, error) {
	result := r.db.WithContext(ctx).
		Where("scope = ? AND owner = ? AND generation = ?", scope, owner, generation).
		Delete(&model.WorkItemLease{})
	if result.Error != nil {
		return false, fmt.Errorf("release lease %s: %w", scope, result.Error)
	}
	return result.RowsAffected == 1, nil
}

// Verify 核验调用方仍有效持有执行权（owner+generation 匹配且未过期）。
// 副作用写出前调用：false 表示陈旧代次，必须放弃写入。
func (r *WorkItemLeaseRepository) Verify(ctx context.Context, scope, owner string, generation uint64) (bool, error) {
	query := r.db.WithContext(ctx).Model(&model.WorkItemLease{}).
		Where("scope = ? AND owner = ? AND generation = ?", scope, owner, generation)
	if r.db.Dialector.Name() == "mysql" {
		query = query.Where("lease_until > NOW(6)")
	} else {
		query = query.Where("lease_until > ?", time.Now().UTC())
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("verify lease %s: %w", scope, err)
	}
	return count == 1, nil
}

// Get 读取当前租约（可观测/诊断；不存在返回 (nil, nil)）。
func (r *WorkItemLeaseRepository) Get(ctx context.Context, scope string) (*model.WorkItemLease, error) {
	var row model.WorkItemLease
	err := r.db.WithContext(ctx).Where("scope = ?", scope).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get lease %s: %w", scope, err)
	}
	return &row, nil
}
