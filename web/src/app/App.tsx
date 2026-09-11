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
import { SuperAdminOverviewPage } from '../superadmin/SuperAdminOverviewPage'
import { useWorkspaceSession, type WorkspaceRole } from './workspaceSession'

const rank: Record<WorkspaceRole, number> = { member: 0, tenant_admin: 1, super_admin: 2 }

function RoleGate({ min, children }: { min: WorkspaceRole; children: ReactNode }) {
  const { role, superAdmin } = useWorkspaceSession()
  const current = superAdmin ? 2 : rank[role]
  if (current < rank[min]) return <Navigate to="/" replace />
  return children
}

// SupportOnlyGate：组织级业务页面（需求/仓库/任务/数据/人员/安全操作/
// 连接器/Agent 观测）对无支持访问的超管整体关闭——直达 URL 一律引导回
// 首页，不再渲染必然 403 或越权可见的页面；支持访问中（role 降为
// tenant_admin）与组织管理员不受影响。
function SupportOnlyGate({ children }: { children: ReactNode }) {
  const { superAdmin, session } = useWorkspaceSession()
  if (superAdmin && !session?.support?.active) return <Navigate to="/" replace />
  return children
}

// 首页按身份分流：超管看系统总览（组织规模），成员看工作台，
// 组织管理员（含支持访问中的超管）看组织级运行总览。
function HomeRoute() {
  const { role } = useWorkspaceSession()
  if (role === 'super_admin') return <SuperAdminOverviewPage />
  if (role === 'member') return <MemberHomePage />
  return <OverviewPage />
}

export function App() {
  return (
    <Routes>
      <Route path="login" element={<LoginPage />} />
      <Route path="portal/*" element={<PortalApp />} />
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
          <Route path="connectors" element={<RoleGate min="tenant_admin"><SupportOnlyGate><ConnectorsPage /></SupportOnlyGate></RoleGate>} />
          <Route path="operations" element={<RoleGate min="tenant_admin"><SupportOnlyGate><OperationsPage /></SupportOnlyGate></RoleGate>} />
          <Route path="agents" element={<SupportOnlyGate><AgentObservabilityPage /></SupportOnlyGate>} />
          {/* 超管子应用整合进主壳：页面渲染在 AppShell 内，子应用自带会话门闸。 */}
          <Route path="super-admin/*" element={<SuperAdminApp />} />
          <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
