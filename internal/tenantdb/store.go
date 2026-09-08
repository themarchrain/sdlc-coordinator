package tenantdb

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// Store 是租户作用域业务库句柄。业务代码只通过本类型碰库，
// 禁止在全局连接上 WHERE tenant_id。
type Store struct {
	DB *gorm.DB
}

// Resolver 按租户解析句柄。当前实现把所有租户指到主库，并缓存租户 id。
type Resolver interface {
	Handle(ctx context.Context, tenantID uint) (*Store, error)
	Invalidate(tenantID uint)
	Main() *Store
}

var ErrInvalidTenantID = errors.New("invalid tenant id")

const DefaultCacheSize = 64
