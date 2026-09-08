package controlplane

import (
	"context"
	"time"
)

// SessionInfo 是 HTTP 层会话视图：与 ValidateSession 不同，它对「无作用域」
// 与「组织已禁用」不直接报错，而是携带完整状态交由中间件分级裁决
// （登出/改密等基础动作在组织禁用时仍须可达）。
type SessionInfo struct {
	User             *User
	Scope            SessionScope
	EffectiveTenant  *Tenant
	EffectiveRole    string
	SuperAdminMode   bool
	SupportMode      bool
	SupportReason    string
	SupportEnteredAt *time.Time
	Tenants          []TenantOption
}

// SessionInfo 按 token 汇总当前会话状态（校验链与 ValidateSession 一致：
// token → 用户存在 → memberships 回查 → 组织状态）。
func (service *Service) SessionInfo(ctx context.Context, token string) (*SessionInfo, error) {
	session, err := service.activeSession(ctx, token)
	if err != nil {
		return nil, err
	}
	user, err := service.repository.GetUser(ctx, session.UserID)
	if err != nil {
		return nil, ErrSessionRevoked
	}
	info := &SessionInfo{User: user}
	if user.Role == UserRoleSuperAdmin || session.Role == RoleSuperAdmin {
		info.SuperAdminMode = true
		info.Scope = SessionScope{Kind: ScopeSuperAdmin, Role: RoleSuperAdmin}
		if session.SupportTenantID != nil {
			tenant, err := service.repository.GetTenant(ctx, *session.SupportTenantID)
			if err != nil {
				return nil, ErrSessionRevoked
			}
			info.SupportMode = true
			info.SupportReason = session.SupportReason
			info.SupportEnteredAt = session.SupportEnteredAt
			info.EffectiveTenant = tenant
			info.EffectiveRole = RoleTenantAdmin
			info.Scope.Tenant = tenant
		}
		return info, nil
	}
	if session.ActiveTenantID == nil {
		info.Scope = SessionScope{Kind: ScopeUnscoped}
		memberships, err := service.repository.ListMembershipsByUser(ctx, user.ID)
		if err != nil {
			return nil, err
		}
		options, err := service.tenantOptionsFor(ctx, memberships)
		if err != nil {
			return nil, err
		}
		info.Tenants = options
		return info, nil
	}
	membership, err := service.repository.GetMembership(ctx, user.ID, *session.ActiveTenantID)
	if err != nil {
		return nil, ErrSessionRevoked
	}
	if membership.Role != session.Role {
		return nil, ErrSessionRevoked
	}
	tenant, err := service.repository.GetTenant(ctx, *session.ActiveTenantID)
	if err != nil {
		return nil, ErrSessionRevoked
	}
	info.Scope = SessionScope{Kind: ScopeTenant, Tenant: tenant, Role: membership.Role}
	info.EffectiveTenant = tenant
	info.EffectiveRole = membership.Role
	return info, nil
}

// MarkMustChangePassword 设置/清除强制改密标记（管理员开通路径在创建时内置；
// 该方法供运维修正与测试装配）。
func (service *Service) MarkMustChangePassword(ctx context.Context, userID uint, mustChange bool) error {
	user, err := service.repository.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	user.MustChangePassword = mustChange
	return service.repository.SaveUser(ctx, user)
}
