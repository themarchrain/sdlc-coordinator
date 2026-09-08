package handlers

import (
	"net/http"

	"sdlc-coordinator/internal/controlplane"

	"github.com/gin-gonic/gin"
)

// AdminRouteAccess 是一条 /api/admin 路由对三角色的放行位。
type AdminRouteAccess struct {
	SuperAdmin bool
	// SuperAdminSupportOnly：super_admin 仅在支持访问（带审计）
	// 中放行；tenant_admin/member 列不受影响。
	SuperAdminSupportOnly bool
	TenantAdmin           bool
	Member                bool
}

// AdminRoutePolicy 按 gin FullPath 查找矩阵放行位；未登记则 found=false，门禁 fail-closed。
type AdminRoutePolicy interface {
	Lookup(method, path string) (AdminRouteAccess, bool)
}

// SetAdminRoutePolicy 注入 /api/admin 矩阵；由 router 装配。
func (handler *ControlPlaneHandler) SetAdminRoutePolicy(policy AdminRoutePolicy) {
	handler.adminPolicy = policy
}

// RequireAdminMatrix 在已通过 RequirePortalAuth 的请求上执行矩阵 RBAC：
// 未登录 401；无组织作用域 403 tenant_scope_required；角色不足 403；
// super_admin 不要求组织作用域。
func (handler *ControlPlaneHandler) RequireAdminMatrix(c *gin.Context) {
	info := handler.sessionInfo(c)
	if info == nil {
		writeAdminError(c, http.StatusUnauthorized, "unauthenticated", "请先登录")
		c.Abort()
		return
	}
	if handler.adminPolicy == nil {
		writeAdminError(c, http.StatusForbidden, "forbidden", "没有执行该操作的权限")
		c.Abort()
		return
	}
	access, found := handler.adminPolicy.Lookup(c.Request.Method, c.FullPath())
	if !found {
		writeAdminError(c, http.StatusForbidden, "forbidden", "没有执行该操作的权限")
		c.Abort()
		return
	}
	allowed, code, message := decideAdminMatrix(info, access)
	if !allowed {
		writeAdminError(c, http.StatusForbidden, code, message)
		c.Abort()
		return
	}
	if info.EffectiveTenant != nil {
		c.Set(cpTenantKey, info.EffectiveTenant)
		c.Set(cpRoleKey, info.EffectiveRole)
		c.Set(cpSupportKey, info.SupportMode)
	}
	c.Next()
}

func decideAdminMatrix(info *controlplane.SessionInfo, access AdminRouteAccess) (bool, string, string) {
	if info.SuperAdminMode {
		if access.SuperAdmin {
			return true, "", ""
		}
		// 业务面收窄为 support_only——仅在带审计的支持访问中放行。
		if access.SuperAdminSupportOnly && info.SupportMode {
			return true, "", ""
		}
		return false, "forbidden", "没有执行该操作的权限"
	}
	if info.EffectiveTenant == nil {
		return false, "tenant_scope_required", "请先选择组织"
	}
	if info.EffectiveTenant.Status != controlplane.TenantStatusActive {
		return false, "tenant_disabled", "该组织已被停用"
	}
	switch info.EffectiveRole {
	case controlplane.RoleTenantAdmin:
		if access.TenantAdmin {
			return true, "", ""
		}
	case controlplane.RoleMember:
		if access.Member {
			return true, "", ""
		}
	}
	return false, "forbidden", "没有执行该操作的权限"
}
