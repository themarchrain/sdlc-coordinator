import { describe, expect, it } from 'vitest'

import type { PortalSessionPayload } from '../api/portal'
import { workspaceLandingPath, workspaceRoleFromSession } from './workspaceSession'

const memberSession: PortalSessionPayload = {
  user: { id: 1, username: 'member', role: 'user', must_change_password: false },
  scope: { kind: 'tenant', role: 'member', tenant: { id: 2, slug: 'acme', display_name: 'Acme', status: 'active' } },
}

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
