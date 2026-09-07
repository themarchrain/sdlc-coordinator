import { useEffect, useState, type ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router-dom'

import { adminApi } from '../api/client'
import type { ConfigurationStatus } from '../api/types'
import { ErrorState, LoadingState } from '../components/PageState'

export function ConfigurationBoundary({ children }: { children: ReactNode }) {
  const location = useLocation()
  const [status, setStatus] = useState<ConfigurationStatus>()
  const [error, setError] = useState<unknown>()

  useEffect(() => {
    let active = true
    adminApi.configurationStatus()
      .then((value) => { if (active) setStatus(value) })
      .catch((reason) => { if (active) setError(reason) })
    return () => { active = false }
  }, [])

  if (error) return <ErrorState error={error} />
  if (!status) return <LoadingState />

  const bootstrapMode = status.mode === 'setup_pending' || status.mode === 'configuration_error'
  if (bootstrapMode && location.pathname !== '/setup' && location.pathname !== '/people') return <Navigate to="/setup" replace />
  if (status.mode === 'ready' && location.pathname === '/setup') return <Navigate to="/settings" replace />
  return children
}
