package tenantdb

import (
	"context"
	"errors"

	"sdlc-coordinator/internal/store"

	"gorm.io/gorm"
)

// Store 是租户作用域业务库句柄。业务代码只通过本类型碰库，
// 禁止在全局连接上 WHERE tenant_id。
type Store struct {
	DB     *gorm.DB
	budget *store.ConnectionBudget
}

func (storeHandle *Store) Close() error {
	if storeHandle == nil || storeHandle.DB == nil {
		return nil
	}
	sqlDB, err := storeHandle.DB.DB()
	if err != nil {
		return err
	}
	if storeHandle.budget != nil {
		storeHandle.budget.UnregisterDB(sqlDB)
	}
	return sqlDB.Close()
}

// Resolver 按租户解析句柄。
type Resolver interface {
	Handle(ctx context.Context, tenantID uint) (*Store, error)
	Invalidate(tenantID uint)
	Main() *Store
}

type UnmanagedResolver interface {
	Resolver
	// OpenUnmanaged 打开不参与缓存淘汰的独立句柄，生命周期归调用方。
	OpenUnmanaged(tenantID uint) (*Store, error)
	// EnsureDatabase 确保数据面容器存在（MySQL：库；SQLite：目录）并返回
	// 规范 locator；幂等、可并发（内部串行保护）。不做 schema 迁移。
	EnsureDatabase(ctx context.Context, tenantID uint) (string, error)
	// DiscardFailedProvision 处置开通失败后的半成品：SQLite 清理目录（登记行
	// 由外层事务回滚兜底）；MySQL 只失效句柄缓存，库保留给幂等重试接管。
	DiscardFailedProvision(tenantID uint)
	IsProvisioned(locator string) bool
	// Locator 返回该租户的规范 locator（不开通、不连接）。
	Locator(tenantID uint) string
	Close()
}

var ErrInvalidTenantID = errors.New("invalid tenant id")

const DefaultCacheSize = 64
