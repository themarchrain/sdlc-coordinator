import { useEffect, useState, type ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router-dom'

import { PortalApiError, portalApi } from '../api/portal'
import { ErrorState, LoadingState } from '../components/PageState'
import { WorkspaceSessionProvider, workspaceRoleFromSession } from './workspaceSession'

export function AuthenticationBoundary({ children }: { children: ReactNode }) {
  const location = useLocation()
  const [session, setSession] = useState<ReturnType<typeof workspaceRoleFromSession> | 'loading' | 'unauthenticated' | { error: unknown }>('loading')

  useEffect(() => {
    let active = true
    portalApi.session()
      .then((payload) => {
        if (active) setSession(workspaceRoleFromSession(payload))
      })
      .catch((reason) => {
        if (!active) return
        if (reason instanceof PortalApiError && reason.status === 401) {
          setSession('unauthenticated')
          return
        }
        setSession({ error: reason })
      })
    return () => { active = false }
  }, [])

  if (session === 'loading') return <LoadingState />
  if (session === 'unauthenticated') {
    const returnTo = `${location.pathname}${location.search}`
    return <Navigate to={`/portal/login?return_to=${encodeURIComponent(returnTo)}`} replace />
  }
  if ('error' in session) return <ErrorState error={session.error} />
  if (session.session?.user.must_change_password) {
    return <Navigate to="/portal/change-password" replace />
  }
  if (session.session?.scope.kind === 'unscoped') {
    return <Navigate to="/portal/select-tenant" replace />
  }
  return <WorkspaceSessionProvider value={session}>{children}</WorkspaceSessionProvider>
}
