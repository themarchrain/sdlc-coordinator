package handlers

import (
	"net/http"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/tenantdb"

	"github.com/gin-gonic/gin"
)

const tenantStoreGinKey = "tenant_store"

func CurrentPortalSession(c *gin.Context) *controlplane.SessionInfo {
	value, exists := c.Get(cpSessionKey)
	if !exists {
		return nil
	}
	info, _ := value.(*controlplane.SessionInfo)
	return info
}

func TenantStoreFrom(c *gin.Context) (*tenantdb.Store, bool) {
	value, exists := c.Get(tenantStoreGinKey)
	if !exists {
		return nil, false
	}
	store, _ := value.(*tenantdb.Store)
	return store, store != nil && store.DB != nil
}

func setTenantStore(c *gin.Context, store *tenantdb.Store) {
	c.Set(tenantStoreGinKey, store)
}

// BindTenantStore 在门户矩阵通过后把租户库句柄写入 gin 上下文。
// 无组织作用域的超管使用 Main()（与主库同一句柄）。
func BindTenantStore(resolver tenantdb.Resolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		if resolver == nil {
			c.Next()
			return
		}
		info := CurrentPortalSession(c)
		var store *tenantdb.Store
		var err error
		if info != nil && info.EffectiveTenant != nil {
			store, err = resolver.Handle(c.Request.Context(), info.EffectiveTenant.ID)
		} else {
			store = resolver.Main()
		}
		if err != nil || store == nil || store.DB == nil {
			writeAdminError(c, http.StatusInternalServerError, "internal_error", "租户数据面暂不可用")
			c.Abort()
			return
		}
		setTenantStore(c, store)
		c.Next()
	}
}
