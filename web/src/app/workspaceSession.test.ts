import { renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import type { PortalSessionPayload } from '../api/portal'
import { useCanWrite, workspaceLandingPath, workspaceRoleFromSession } from './workspaceSession'

const memberSession: PortalSessionPayload = {
  user: { id: 1, username: 'member', role: 'user', must_change_password: false },
  scope: { kind: 'tenant', role: 'member', tenant: { id: 2, slug: 'acme', display_name: 'Acme', status: 'active' } },
}

describe('default session context', () => {
  it('fails closed to a plain member outside the provider', () => {
    // Provider 外误用不得 fail-open 成超管：默认上下文按普通成员处理。
    const { result } = renderHook(() => useCanWrite())
    expect(result.current).toBe(false)
  })
})

describe('workspaceRoleFromSession', () => {
  it('maps tenant members to member without super-admin privileges', () => {
    expect(workspaceRoleFromSession(memberSession)).toMatchObject({ role: 'member', superAdmin: false })
  })

  it('keeps tenant admins writable', () => {
    const session: PortalSessionPayload = {
      ...memberSession,
      user: { ...memberSession.user, username: 'admin' },
      scope: { ...memberSession.scope, role: 'tenant_admin' },
    }
    expect(workspaceRoleFromSession(session)).toMatchObject({ role: 'tenant_admin', superAdmin: false })
  })
})

describe('workspaceLandingPath', () => {
  it('sends tenant-scoped sessions to the admin workspace root', () => {
    expect(workspaceLandingPath(memberSession)).toBe('/')
  })

  it('keeps super admins on the independent console', () => {
    expect(workspaceLandingPath({
      user: { id: 9, username: 'root', role: 'super_admin', must_change_password: false },
      scope: { kind: 'super_admin', role: 'super_admin' },
    })).toBe('/super-admin/tenants')
  })
})
