import type { ReactNode } from 'react'
import { useState } from 'react'
import {
  ApiOutlined,
  AppstoreOutlined,
  BarChartOutlined,
  BulbOutlined,
  ControlOutlined,
  DatabaseOutlined,
  FolderOutlined,
  LineChartOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  ProfileOutlined,
  RobotOutlined,
  SettingOutlined,
  TeamOutlined,
} from '@ant-design/icons'
import { NavLink, useSearchParams } from 'react-router-dom'
import { PageHeadingHostContext } from '../components/PageHeading'
import { RepositorySwitcher } from '../components/RepositorySwitcher'

const workspaces = [
  { to: '/', label: '运行总览', icon: <AppstoreOutlined />, end: true },
  { to: '/requirements', label: '需求管理', icon: <BulbOutlined /> },
  { to: '/repositories', label: '仓库管理', icon: <FolderOutlined /> },
  { to: '/tasks', label: '任务查询', icon: <ProfileOutlined /> },
  { to: '/explorer', label: '数据查询', icon: <DatabaseOutlined /> },
]

const adminWorkspaces = [
  { to: '/people', label: '人员资产', icon: <TeamOutlined /> },
  { to: '/settings', label: '系统配置', icon: <SettingOutlined /> },
  { to: '/operations', label: '安全操作', icon: <ControlOutlined /> },
  { to: '/connectors', label: '连接器实例', icon: <ApiOutlined /> },
]

const agentWorkspaces = [
  { to: '/agents/codex', label: 'Codex 运行', icon: <RobotOutlined /> },
]

function withRepository(path: string, repositoryID: string | null) {
  if (!repositoryID) return path
  return `${path}?repository_id=${encodeURIComponent(repositoryID)}`
}

export function AppShell({ children }: { children: ReactNode }) {
  const [collapsed, setCollapsed] = useState(false)
  const [pageHeadingHost, setPageHeadingHost] = useState<HTMLDivElement | null>(null)
  const [searchParams] = useSearchParams()
  const repositoryID = searchParams.get('repository_id')

  return (
    <div className={`control-room${collapsed ? ' control-room--collapsed' : ''}`}>
      <aside className="control-sidebar">
        <header className="brand-block">
          <div className="brand-mark" aria-hidden="true"><span /><span /><span /></div>
          <div><strong>Coordinator</strong><span>Control room</span></div>
        </header>

        <nav className="workspace-nav" aria-label="管理台工作区">
          <div className="nav-caption">工作区</div>
          {workspaces.map((item) => (
            <NavLink key={item.to} to={withRepository(item.to, repositoryID)} end={item.end} title={item.label}>
              {item.icon}<span className="nav-label">{item.label}</span>
            </NavLink>
          ))}
          <div className="nav-group-title">Agent 观测</div>
          {agentWorkspaces.map((item) => (
            <NavLink key={item.to} to={withRepository(item.to, repositoryID)} title={item.label}>
              {item.icon}<span className="nav-label">{item.label}</span>
            </NavLink>
          ))}
          <div className="nav-group-title">管理与配置</div>
          {adminWorkspaces.map((item) => (
            <NavLink key={item.to} to={withRepository(item.to, repositoryID)} title={item.label}>
              {item.icon}<span className="nav-label">{item.label}</span>
            </NavLink>
          ))}
        </nav>

        <button
          type="button"
          className="collapse-toggle"
          aria-label={collapsed ? '展开侧边栏' : '折叠侧边栏'}
          onClick={() => setCollapsed((value) => !value)}
        >
          {collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
        </button>
      </aside>

      <section className="control-stage">
        <header className="context-bar">
          <div className="context-instance"><span className="context-dot" /> 单实例控制面</div>
          <div className="context-heading-slot" ref={setPageHeadingHost} />
          <RepositorySwitcher />
        </header>
        <PageHeadingHostContext.Provider value={pageHeadingHost}>
          <main className="workspace-canvas">{children}</main>
        </PageHeadingHostContext.Provider>
      </section>
    </div>
  )
}
