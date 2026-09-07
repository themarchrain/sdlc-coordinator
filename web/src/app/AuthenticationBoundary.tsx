import { useEffect, useState, type ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router-dom'

import { AdminApiError, adminApi } from '../api/client'
import { ErrorState, LoadingState } from '../components/PageState'

export function AuthenticationBoundary({ children }: { children: ReactNode }) {
  const location = useLocation()
  const [authenticated, setAuthenticated] = useState<boolean>()
  const [error, setError] = useState<unknown>()

  useEffect(() => {
    let active = true
    adminApi.session()
      .then(() => { if (active) setAuthenticated(true) })
      .catch((reason) => {
        if (!active) return
        if (reason instanceof AdminApiError && reason.status === 401) {
          setAuthenticated(false)
          return
        }
        setError(reason)
      })
    return () => { active = false }
  }, [])

  if (error) return <ErrorState error={error} />
  if (authenticated === undefined) return <LoadingState />
  if (!authenticated) {
    const returnTo = `${location.pathname}${location.search}`
    return <Navigate to={`/login?return_to=${encodeURIComponent(returnTo)}`} replace />
  }
  return children
}
