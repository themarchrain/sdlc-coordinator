// portal.ts 是新认证体系（多租户控制面 M1）的前端 API client：会话走
// HttpOnly cookie（credentials: same-origin），与存量 adminApi（单管理员
// 会话）互不干涉。

export interface PortalUser {
  id: number
  username: string
  role: string
  must_change_password: boolean
}

export interface PortalTenant {
  id: number
  slug: string
  display_name: string
  status: string
  created_at?: string
}

export interface PortalScope {
  kind: 'super_admin' | 'tenant' | 'unscoped'
  role?: string | null
  tenant?: PortalTenant | null
}

export interface PortalTenantOption {
  id: number
  slug: string
  display_name: string
  role: string
}

export interface PortalSupportState {
  active: boolean
  reason: string
  entered_at?: string | null
}

export interface PortalSessionPayload {
  user: PortalUser
  scope: PortalScope
  tenants?: PortalTenantOption[]
  support?: PortalSupportState
}

export interface PortalMember {
  user_id: number
  username: string
  role: string
  created_at: string
}

export interface PortalInviteCodeRecord {
  id: number
  role: string
  expires_at: string
  max_uses: number
  used_count: number
  revoked_at: string | null
  created_by: number
  created_at: string
}

export interface PortalAuditItem {
  id: number
  actor_user_id: number | null
  actor_username: string
  tenant_id: number | null
  action: string
  detail: string
  created_at: string
}

export interface PortalAuditPage {
  items: PortalAuditItem[]
  total: number
  page: number
  page_size: number
}

export interface PortalTenantSummary extends PortalTenant {
  member_count: number
}

export interface PortalRunnerAssignment {
  id: number
  endpoint_url: string
  created_at: string
}

export class PortalApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
  ) {
    super(message)
    this.name = 'PortalApiError'
  }
}

interface PortalErrorEnvelope {
  error?: { code?: string; message?: string }
}

async function portalRequest<T>(method: string, path: string, body?: unknown): Promise<T> {
  const init: RequestInit = {
    method,
    credentials: 'same-origin',
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  }
  const response = await fetch(path, init)
  if (!response.ok) {
    let code = 'portal_error'
    let message = '请求失败，请稍后重试'
    try {
      const envelope = (await response.json()) as PortalErrorEnvelope
      if (envelope.error?.code) code = envelope.error.code
      if (envelope.error?.message) message = envelope.error.message
    } catch {
      // 非 JSON 错误体：保留默认文案
    }
    throw new PortalApiError(response.status, code, message)
  }
  return (await response.json()) as T
}

export const portalApi = {
  login(username: string, password: string): Promise<PortalSessionPayload> {
    return portalRequest('POST', '/api/portal/auth/login', { username, password })
  },
  register(username: string, password: string, inviteCode: string): Promise<PortalSessionPayload> {
    return portalRequest('POST', '/api/portal/auth/register', { username, password, invite_code: inviteCode })
  },
  logout(): Promise<{ ok: boolean }> {
    return portalRequest('POST', '/api/portal/auth/logout')
  },
  session(): Promise<PortalSessionPayload> {
    return portalRequest('GET', '/api/portal/auth/session')
  },
  selectTenant(tenantId: number): Promise<PortalSessionPayload> {
    return portalRequest('POST', '/api/portal/auth/select-tenant', { tenant_id: tenantId })
  },
  changePassword(currentPassword: string, newPassword: string): Promise<{ ok: boolean }> {
    return portalRequest('POST', '/api/portal/auth/change-password', {
      current_password: currentPassword,
      new_password: newPassword,
    })
  },
  tenantProfile(): Promise<{ tenant: PortalTenant; role: string; support: boolean }> {
    return portalRequest('GET', '/api/portal/tenant/profile')
  },
  listMembers(): Promise<{ members: PortalMember[] }> {
    return portalRequest('GET', '/api/portal/tenant/members')
  },
  removeMember(userId: number): Promise<{ ok: boolean }> {
    return portalRequest('DELETE', `/api/portal/tenant/members/${userId}`)
  },
  resetMemberPassword(userId: number): Promise<{ initial_password: string }> {
    return portalRequest('POST', `/api/portal/tenant/members/${userId}/reset-password`)
  },
  listInviteCodes(): Promise<{ invite_codes: PortalInviteCodeRecord[] }> {
    return portalRequest('GET', '/api/portal/tenant/invite-codes')
  },
  createInviteCode(expiresInHours?: number, maxUses?: number): Promise<{ code: string; record: PortalInviteCodeRecord }> {
    return portalRequest('POST', '/api/portal/tenant/invite-codes', {
      expires_in_hours: expiresInHours,
      max_uses: maxUses,
    })
  },
  revokeInviteCode(codeId: number): Promise<{ ok: boolean }> {
    return portalRequest('DELETE', `/api/portal/tenant/invite-codes/${codeId}`)
  },
  superListTenants(): Promise<{ tenants: PortalTenantSummary[] }> {
    return portalRequest('GET', '/api/super-admin/tenants')
  },
  superCreateTenant(slug: string, displayName: string): Promise<{ tenant: PortalTenant }> {
    return portalRequest('POST', '/api/super-admin/tenants', { slug, display_name: displayName })
  },
  superUpdateTenant(tenantId: number, displayName: string): Promise<{ tenant: PortalTenant }> {
    return portalRequest('PATCH', `/api/super-admin/tenants/${tenantId}`, { display_name: displayName })
  },
  superDisableTenant(tenantId: number): Promise<{ tenant: PortalTenant }> {
    return portalRequest('POST', `/api/super-admin/tenants/${tenantId}/disable`)
  },
  superRestoreTenant(tenantId: number): Promise<{ tenant: PortalTenant }> {
    return portalRequest('POST', `/api/super-admin/tenants/${tenantId}/restore`)
  },
  superListTenantMembers(tenantId: number): Promise<{ members: PortalMember[] }> {
    return portalRequest('GET', `/api/super-admin/tenants/${tenantId}/members`)
  },
  superProvisionAdmin(tenantId: number, username: string): Promise<{ user_id: number; username: string; initial_password?: string }> {
    return portalRequest('POST', `/api/super-admin/tenants/${tenantId}/admins`, { username })
  },
  superResetAdminPassword(tenantId: number, userId: number): Promise<{ initial_password: string }> {
    return portalRequest('POST', `/api/super-admin/tenants/${tenantId}/admins/${userId}/reset-password`)
  },
  superListInviteCodes(tenantId: number): Promise<{ invite_codes: PortalInviteCodeRecord[] }> {
    return portalRequest('GET', `/api/super-admin/tenants/${tenantId}/invite-codes`)
  },
  superRevokeInviteCode(tenantId: number, codeId: number): Promise<{ ok: boolean }> {
    return portalRequest('DELETE', `/api/super-admin/tenants/${tenantId}/invite-codes/${codeId}`)
  },
  superListRunnerAssignments(tenantId: number): Promise<{ runner_assignments: PortalRunnerAssignment[] }> {
    return portalRequest('GET', `/api/super-admin/tenants/${tenantId}/runner-assignments`)
  },
  superReplaceRunnerAssignments(tenantId: number, endpoints: string[]): Promise<{ ok: boolean }> {
    return portalRequest('PUT', `/api/super-admin/tenants/${tenantId}/runner-assignments`, { endpoints })
  },
  superEnterSupportAccess(tenantId: number, reason: string): Promise<{ ok: boolean }> {
    return portalRequest('POST', `/api/super-admin/tenants/${tenantId}/support-access`, { reason })
  },
  superExitSupportAccess(): Promise<{ ok: boolean }> {
    return portalRequest('DELETE', '/api/super-admin/support-access')
  },
  superListAuditLog(params: { tenantId?: number; action?: string; page?: number; pageSize?: number }): Promise<PortalAuditPage> {
    const query = new URLSearchParams()
    if (params.tenantId !== undefined) query.set('tenant_id', String(params.tenantId))
    if (params.action) query.set('action', params.action)
    if (params.page !== undefined) query.set('page', String(params.page))
    if (params.pageSize !== undefined) query.set('page_size', String(params.pageSize))
    const suffix = query.toString() ? `?${query.toString()}` : ''
    return portalRequest('GET', `/api/super-admin/audit-log${suffix}`)
  },
}
