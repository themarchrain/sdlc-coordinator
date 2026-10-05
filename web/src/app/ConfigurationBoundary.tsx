import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router-dom'

import { adminApi } from '../api/client'
import type { ConfigurationStatus } from '../api/types'
import { ErrorState, LoadingState } from '../components/PageState'
import {
  ConfigurationStatusProvider,
  type ConfigurationStatusSyncOptions,
} from './configurationStatus'
import { useWorkspaceSession } from './workspaceSession'

type TrackedConfigurationStatus = { status: ConfigurationStatus; fromRestart: boolean }

export function ConfigurationBoundary({ children }: { children: ReactNode }) {
  const location = useLocation()
  const { superAdmin } = useWorkspaceSession()
  const [tracked, setTracked] = useState<TrackedConfigurationStatus>()
  const [error, setError] = useState<unknown>()

  useEffect(() => {
    let active = true
    adminApi.configurationStatus()
      .then((value) => { if (active) setTracked({ status: value, fromRestart: false }) })
      .catch((reason) => { if (active) setError(reason) })
    return () => { active = false }
  }, [])

  // 重启完成的落地标记只消费一次：离开 /setup 后即失效，之后直接
  // 访问 /setup 仍按既有规则送系统配置页。
  useEffect(() => {
    if (location.pathname !== '/setup' && tracked?.fromRestart) {
      setTracked({ status: tracked.status, fromRestart: false })
    }
  }, [location.pathname, tracked])

  const sync = useCallback((value: ConfigurationStatus, options?: ConfigurationStatusSyncOptions) => {
    setTracked({ status: value, fromRestart: options?.fromRestart === true })
  }, [])

  const statusValue = useMemo(() => ({ status: tracked?.status, sync }), [tracked, sync])

  if (error) return <ErrorState error={error} />
  if (!tracked) return <LoadingState />

  const status = tracked.status
  const bootstrapMode = status.mode === 'setup_pending' || status.mode === 'configuration_error'
  if (superAdmin && bootstrapMode && location.pathname !== '/setup' && location.pathname !== '/people') {
    return <Navigate to="/setup" replace />
  }
  if (status.mode === 'ready' && location.pathname === '/setup') {
    // 重启完成时边界接管落地（送首页），页面不再自行导航——react-router
    // 的 navigate 走 transition，会被这里的紧急重定向打断，二者赛跑。
    return <Navigate to={superAdmin && !tracked.fromRestart ? '/settings' : '/'} replace />
  }
  return <ConfigurationStatusProvider value={statusValue}>{children}</ConfigurationStatusProvider>
}
