import { Navigate, Route, Routes } from 'react-router-dom'

import { ConnectorsPage } from '../pages/ConnectorsPage'
import { ConfigurationPage } from '../pages/ConfigurationPage'
import { DataExplorerPage } from '../pages/DataExplorerPage'
import { OperationsPage } from '../pages/OperationsPage'
import { OverviewPage } from '../pages/OverviewPage'
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

export function App() {
  return (
    <Routes>
      <Route path="login" element={<LoginPage />} />
      {/* 多租户新增页面区：门户（登录/注册/选组织/组织管理）与超管独立后台。
          纯增量，不影响存量 admin 页面与其认证边界。 */}
      <Route path="portal/*" element={<PortalApp />} />
      <Route path="super-admin/*" element={<SuperAdminApp />} />
      <Route path="*" element={<AuthenticationBoundary><AppShell><ConfigurationBoundary><AdminRoutes /></ConfigurationBoundary></AppShell></AuthenticationBoundary>} />
    </Routes>
  )
}

function AdminRoutes() {
  return (
    <Routes>
          <Route index element={<OverviewPage />} />
          <Route path="setup" element={<ConfigurationPage />} />
          <Route path="settings" element={<ConfigurationPage />} />
          <Route path="people" element={<PeoplePage />} />
          <Route path="requirements" element={<RequirementsPage />} />
          <Route path="repositories" element={<RepositoriesPage />} />
          <Route path="tasks" element={<TasksPage />} />
          <Route path="explorer" element={<DataExplorerPage />} />
          <Route path="connectors" element={<ConnectorsPage />} />
          <Route path="operations" element={<OperationsPage />} />
          <Route path="agents" element={<AgentObservabilityPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
