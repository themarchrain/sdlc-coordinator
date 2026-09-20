import { Navigate, Outlet, Route, Routes, useLocation } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { Spin } from 'antd'

import { portalApi, type PortalSessionPayload } from '../api/portal'
import { AuditLogPage } from './AuditLogPage'
import { TenantDetailPage } from './TenantDetailPage'
import { TenantsPage } from './TenantsPage'

type GateState =
  | { phase: 'loading' }
  | { phase: 'redirect' }
  | { phase: 'ready'; session: PortalSessionPayload }

// SuperAdminSessionGate 与门户 PortalSessionGate 对称：进入超管
// 页面区前先确认会话；未登录或非超管会话一律送回门户登录页，不让受保护 API
// 的 401/403 冒充页面状态。支持访问中的超管仍可留在超管后台排障。
function SuperAdminSessionGate() {
  const [state, setState] = useState<GateState>({ phase: 'loading' })

  useEffect(() => {
    let cancelled = false
    portalApi.session()
      .then((session) => { if (!cancelled) setState({ phase: 'ready', session }) })
      .catch(() => { if (!cancelled) setState({ phase: 'redirect' }) })
    return () => { cancelled = true }
  }, [])

  if (state.phase === 'loading') {
    return <div className="flex items-center justify-center py-16"><Spin /></div>
  }
  if (state.phase === 'redirect') {
    return <Navigate to="/portal/login" replace />
  }
  const { session } = state
  if (session.scope.kind !== 'super_admin') {
    return <Navigate to="/portal/login" replace />
  }
  if (session.user.must_change_password) {
    return <Navigate to="/portal/change-password" replace />
  }
  return <Outlet context={session} />
}

// EntryNormalize 是 index/兜底共用的规范化导航：未知入口一律送回组织列表
// （绝对路径，防相对导航在嵌套挂法下循环）。落点与当前路径相同时不再导航。
function EntryNormalize() {
  const location = useLocation()
  const path = location.pathname.replace(/\/+$/, '') || '/'
  if (path === '/super-admin/tenants') {
    return null
  }
  return <Navigate to="/super-admin/tenants" replace />
}

// 超管子应用不再自带独立壳：页面渲染在 /admin 主壳（AppShell）内，本组件
// 只保留「会话门闸 + 叶子路由」。叶子按两种挂载形态双前缀注册：生产挂法
// （AdminRoutes 内的 super-admin/* 父路由）命中第一组；直接渲染 <SuperAdminApp/>
// 时 base 为根，URL 自带的 /super-admin 前缀由第二组吸收——不依赖双层
// Routes 的 splat base 累积语义（回归防护：直接渲染时早期实现叶子匹配
// 不到、主区空白）。
export function SuperAdminApp() {
  return (
    <Routes>
      <Route element={<SuperAdminSessionGate />}>
        <Route path="tenants" element={<TenantsPage />} />
        <Route path="tenants/:id" element={<TenantDetailPage />} />
        <Route path="audit-log" element={<AuditLogPage />} />
        <Route path="super-admin/tenants" element={<TenantsPage />} />
        <Route path="super-admin/tenants/:id" element={<TenantDetailPage />} />
        <Route path="super-admin/audit-log" element={<AuditLogPage />} />
        <Route index element={<EntryNormalize />} />
        <Route path="*" element={<EntryNormalize />} />
      </Route>
    </Routes>
  )
}
