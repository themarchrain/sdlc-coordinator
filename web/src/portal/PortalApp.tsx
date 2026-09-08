import { Navigate, Route, Routes, Outlet, useLocation } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { Spin } from 'antd'

import { portalApi, type PortalSessionPayload } from '../api/portal'
import { PortalChangePasswordPage } from './PortalChangePasswordPage'
import { PortalLoginPage } from './PortalLoginPage'
import { PortalRegisterPage } from './PortalRegisterPage'
import { SelectTenantPage } from './SelectTenantPage'

type GateState =
  | { phase: 'loading' }
  | { phase: 'unauthenticated' }
  | { phase: 'ready'; session: PortalSessionPayload }

// PortalSessionGate 拉取会话并按状态分流：强制改密账号先改密；
// 超管（未支持访问）跳独立后台；多组织未选择先选组织；已绑组织进管理台工作区。
function PortalSessionGate() {
  const location = useLocation()
  const [state, setState] = useState<GateState>({ phase: 'loading' })

  useEffect(() => {
    let cancelled = false
    portalApi.session()
      .then((session) => { if (!cancelled) setState({ phase: 'ready', session }) })
      .catch(() => { if (!cancelled) setState({ phase: 'unauthenticated' }) })
    return () => { cancelled = true }
  }, [])

  if (state.phase === 'loading') {
    return <main className="flex min-h-screen items-center justify-center"><Spin /></main>
  }
  if (state.phase === 'unauthenticated') {
    return <Navigate to="/portal/login" replace />
  }
  const { session } = state
  const path = location.pathname
  if (session.user.must_change_password && path !== '/portal/change-password') {
    return <Navigate to="/portal/change-password" replace />
  }
  if (session.scope.kind === 'super_admin' && !session.support?.active) {
    return <Navigate to="/super-admin/tenants" replace />
  }
  if (session.scope.kind === 'unscoped' && path !== '/portal/select-tenant') {
    return <Navigate to="/portal/select-tenant" replace />
  }
  if (session.scope.kind === 'tenant' && path === '/portal/select-tenant') {
    return <Navigate to="/" replace />
  }
  return (
    <>
      {session.support?.active && (
        <div className="bg-[var(--colorWarningBg,#fffbe6)] px-4 py-2 text-center text-sm">
          支持访问进行中（理由：{session.support.reason}），全部动作已记入审计。
        </div>
      )}
      <Outlet context={session} />
    </>
  )
}

// PortalApp 是新认证体系的门户页面区，挂载于 /admin/portal/*。
export function PortalApp() {
  return (
    <Routes>
      <Route path="login" element={<PortalLoginPage />} />
      <Route path="register" element={<PortalRegisterPage />} />
      <Route element={<PortalSessionGate />}>
        <Route path="select-tenant" element={<SelectTenantPage />} />
        <Route path="change-password" element={<PortalChangePasswordPage />} />
      </Route>
      <Route path="*" element={<Navigate to="login" replace />} />
    </Routes>
  )
}
