package api

import "sdlc-coordinator/internal/http/handlers"

// adminMatrixPolicy 把路由矩阵变成 /api/admin 门禁的查找表。
type adminMatrixPolicy struct{}

func (adminMatrixPolicy) Lookup(method, path string) (handlers.AdminRouteAccess, bool) {
	for _, row := range HTTPRouteMatrix() {
		if row.Class != ClassAdmin {
			continue
		}
		if row.Method == method && row.Path == path {
			return handlers.AdminRouteAccess{
				SuperAdmin:            row.SuperAdmin == AccessAllow,
				SuperAdminSupportOnly: row.SuperAdmin == AccessSupportOnly,
				TenantAdmin:           row.TenantAdmin == AccessAllow,
				Member:                row.Member == AccessAllow,
			}, true
		}
	}
	return handlers.AdminRouteAccess{}, false
}
