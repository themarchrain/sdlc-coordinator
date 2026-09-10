import { Navigate, Route, Routes } from 'react-router-dom'
import type { ReactNode } from 'react'

import { ConnectorsPage } from '../pages/ConnectorsPage'
import { ConfigurationPage } from '../pages/ConfigurationPage'
import { DataExplorerPage } from '../pages/DataExplorerPage'
import { MemberHomePage } from '../pages/MemberHomePage'
import { OperationsPage } from '../pages/OperationsPage'
import { OverviewPage } from '../pages/OverviewPage'
import { OrgManagementPage } from '../portal/OrgManagementPage'
import { OrgConfigurationPage } from '../pages/OrgConfigurationPage'
import { PeoplePage } from '../pages/PeoplePage'
import { RepositoriesPage } from '../pages/RepositoriesPage'
import { RequirementsPage } from '../pages/RequirementsPage'
import { TasksPage } from '../pages/TasksPage'
import { AppShell } from './AppShell'
import { AuthenticationBoundary } from './AuthenticationBoundary'
import { ConfigurationBoundary } from './ConfigurationBoundary'
import { LoginPage } from '../pages/LoginPage'
import { AgentObservabilityPage } from '../pages/AgentObservabilityPage'
import { PortalApp } from '../portal/PortalApp'
import { SuperAdminApp } from '../superadmin/SuperAdminApp'
import { useWorkspaceSession, type WorkspaceRole } from './workspaceSession'

const rank: Record<WorkspaceRole, number> = { member: 0, tenant_admin: 1, super_admin: 2 }

function RoleGate({ min, children }: { min: WorkspaceRole; children: ReactNode }) {
  const { role, superAdmin } = useWorkspaceSession()
  const current = superAdmin ? 2 : rank[role]
  if (current < rank[min]) return <Navigate to="/" replace />
  return children
}

// SupportOnlyGate：业务页面对应的 API 已收窄为 support_only——
// 无支持访问的超管直接访问业务路由时引导回运行总览，不再渲染必然 403 的页面；
// 支持访问中（role 降为 tenant_admin）行为不变。
function SupportOnlyGate({ children }: { children: ReactNode }) {
  const { superAdmin, session } = useWorkspaceSession()
  if (superAdmin && !session?.support?.active) return <Navigate to="/" replace />
  return children
}

function HomeRoute() {
  const { role } = useWorkspaceSession()
  if (role === 'member') return <MemberHomePage />
  return <OverviewPage />
}

export function App() {
  return (
    <Routes>
      <Route path="login" element={<LoginPage />} />
      <Route path="portal/*" element={<PortalApp />} />
      <Route path="super-admin/*" element={<SuperAdminApp />} />
      <Route path="*" element={<AuthenticationBoundary><AppShell><ConfigurationBoundary><AdminRoutes /></ConfigurationBoundary></AppShell></AuthenticationBoundary>} />
    </Routes>
  )
}

function AdminRoutes() {
  return (
    <Routes>
          <Route index element={<HomeRoute />} />
          <Route path="setup" element={<RoleGate min="super_admin"><ConfigurationPage /></RoleGate>} />
          <Route path="settings" element={<RoleGate min="super_admin"><ConfigurationPage /></RoleGate>} />
          <Route path="people" element={<RoleGate min="tenant_admin"><SupportOnlyGate><PeoplePage /></SupportOnlyGate></RoleGate>} />
          {/* 组织管理内嵌工作台：菜单直达 /org；portal 区只留无壳页。 */}
          <Route path="org" element={<RoleGate min="tenant_admin"><OrgManagementPage /></RoleGate>} />
          {/* 组织段运行配置（代码仓库默认值+通知）：组织管理员向导，保存热生效。 */}
          <Route path="org/settings" element={<RoleGate min="tenant_admin"><OrgConfigurationPage /></RoleGate>} />
          <Route path="requirements" element={<SupportOnlyGate><RequirementsPage /></SupportOnlyGate>} />
          <Route path="repositories" element={<RoleGate min="tenant_admin"><SupportOnlyGate><RepositoriesPage /></SupportOnlyGate></RoleGate>} />
          <Route path="tasks" element={<SupportOnlyGate><TasksPage /></SupportOnlyGate>} />
          <Route path="explorer" element={<SupportOnlyGate><DataExplorerPage /></SupportOnlyGate>} />
          <Route path="connectors" element={<RoleGate min="tenant_admin"><ConnectorsPage /></RoleGate>} />
          <Route path="operations" element={<RoleGate min="tenant_admin"><OperationsPage /></RoleGate>} />
          <Route path="agents" element={<AgentObservabilityPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
