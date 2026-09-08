import { Menu } from 'antd'
import { AuditOutlined, ClusterOutlined } from '@ant-design/icons'
import { Navigate, Outlet, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
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
    return <main className="flex min-h-screen items-center justify-center"><Spin /></main>
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

// SuperAdminShell 是超管独立后台页面区骨架：独立菜单区，与组织内
// 界面（AppShell）完全分离；入口 /admin/super-admin/*。菜单用横向 antd Menu +
// 普通头部布局，不引入 Sider 响应式（jsdom 下 matchMedia 交互不稳）。叶子页面
// 由 <Outlet /> 渲染（Outlet 化形态）。
function SuperAdminShell() {
  const navigate = useNavigate()
  const location = useLocation()

  const selectedKey = location.pathname.startsWith('/super-admin/audit-log')
    ? 'audit-log'
    : 'tenants'

  return (
    <div className="flex min-h-screen flex-col bg-[var(--colorBgLayout,#f5f5f5)]" data-testid="super-admin-shell">
      <header className="flex items-center justify-between border-b border-[var(--colorBorderSecondary,#eee)] bg-[var(--colorBgContainer,#fff)] px-4 py-2">
        <div className="select-none text-sm font-semibold tracking-widest text-[var(--colorTextSecondary,#666)]">
          超管后台
        </div>
        <Menu
          mode="horizontal"
          selectedKeys={[selectedKey]}
          onClick={(info) => navigate(`/super-admin/${info.key}`)}
          items={[
            { key: 'tenants', icon: <ClusterOutlined />, label: '组织管理' },
            { key: 'audit-log', icon: <AuditOutlined />, label: '审计日志' },
          ]}
          style={{ flex: 1, minWidth: 280, borderBottom: 'none' }}
        />
      </header>
      <main className="flex-1 p-4">
        <Outlet />
      </main>
    </div>
  )
}

// EntryNormalize 是 index/兜底共用的规范化导航：未知入口按 URL 前缀送回组织
// 列表——带 /super-admin 前缀的入口在生产挂法下由父路由吸收、在直接渲染挂法
// 下由下方双前缀叶子吸收；无前缀入口走根下叶子。落点与当前路径相同时不再
// 导航（防重定向循环）。
function EntryNormalize() {
  const location = useLocation()
  const path = location.pathname.replace(/\/+$/, '') || '/'
  const target = path.startsWith('/super-admin') ? '/super-admin/tenants' : '/tenants'
  if (path === target) {
    return null
  }
  return <Navigate to={target} replace />
}

// 单一 layout 路由树：门闸（pathless）→ Shell（pathless，main 内 Outlet）→
// 叶子子路由（Outlet 化形态）。叶子按两种挂载形态双前缀注册：生产挂法
// （App.tsx 的 super-admin/* 父路由）命中第一组；直接渲染 <SuperAdminApp/>
// 时 base 为根，URL 自带的 /super-admin 前缀由第二组吸收——不依赖双层
// Routes 的 splat base 累积语义（回归防护：直接渲染时早期实现叶子匹配
// 不到、主区空白）。
export function SuperAdminApp() {
  return (
    <Routes>
      <Route element={<SuperAdminSessionGate />}>
        <Route element={<SuperAdminShell />}>
          <Route path="tenants" element={<TenantsPage />} />
          <Route path="tenants/:id" element={<TenantDetailPage />} />
          <Route path="audit-log" element={<AuditLogPage />} />
          <Route path="super-admin/tenants" element={<TenantsPage />} />
          <Route path="super-admin/tenants/:id" element={<TenantDetailPage />} />
          <Route path="super-admin/audit-log" element={<AuditLogPage />} />
          <Route index element={<EntryNormalize />} />
          <Route path="*" element={<EntryNormalize />} />
        </Route>
      </Route>
    </Routes>
  )
}
