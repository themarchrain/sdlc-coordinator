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
import { CodexObservabilityPage } from '../pages/CodexObservabilityPage'

export function App() {
  return (
    <Routes>
      <Route path="login" element={<LoginPage />} />
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
	          <Route path="agents/codex" element={<CodexObservabilityPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
