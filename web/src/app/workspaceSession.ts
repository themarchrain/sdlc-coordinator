import { createContext, createElement, useContext, type ReactNode } from 'react'

import type { PortalSessionPayload } from '../api/portal'

export type WorkspaceRole = 'super_admin' | 'tenant_admin' | 'member'

export type WorkspaceSessionValue = {
  session: PortalSessionPayload | null
  role: WorkspaceRole
  superAdmin: boolean
}

const WorkspaceSessionContext = createContext<WorkspaceSessionValue>({
  session: null,
  role: 'super_admin',
  superAdmin: true,
})

export function WorkspaceSessionProvider({
  value,
  children,
}: {
  value: WorkspaceSessionValue
  children: ReactNode
}) {
  return createElement(WorkspaceSessionContext.Provider, { value }, children)
}

export function useWorkspaceSession(): WorkspaceSessionValue {
  return useContext(WorkspaceSessionContext)
}

export function useCanWrite(): boolean {
  const { role, superAdmin } = useWorkspaceSession()
  return superAdmin || role === 'tenant_admin' || role === 'super_admin'
}

export function workspaceRoleFromSession(session: PortalSessionPayload): WorkspaceSessionValue {
  const superAdmin = session.scope.kind === 'super_admin' || session.user.role === 'super_admin'
  if (superAdmin && !session.support?.active) {
    return { session, role: 'super_admin', superAdmin: true }
  }
  if (session.scope.role === 'member' && !session.support?.active) {
    return { session, role: 'member', superAdmin }
  }
  return { session, role: 'tenant_admin', superAdmin }
}

export function workspaceLandingPath(session: PortalSessionPayload): string {
  if (session.user.must_change_password) return '/portal/change-password'
  if (session.scope.kind === 'super_admin' && !session.support?.active) return '/super-admin/tenants'
  if (session.scope.kind === 'unscoped') return '/portal/select-tenant'
  return '/'
}
