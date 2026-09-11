import type { ReactNode } from 'react'
import { useState } from 'react'
import {
  ApiOutlined,
  AppstoreOutlined,
  AuditOutlined,
  BankOutlined,
  BulbOutlined,
  ControlOutlined,
  DatabaseOutlined,
  FolderOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  ProfileOutlined,
  RobotOutlined,
  SettingOutlined,
  TeamOutlined,
} from '@ant-design/icons'
import { NavLink, useSearchParams } from 'react-router-dom'
import { AppearanceDrawerButton } from '../components/AppearanceDrawer'
import { PageHeadingHostContext } from '../components/PageHeading'
import { RepositorySwitcher } from '../components/RepositorySwitcher'
import { useWorkspaceSession } from './workspaceSession'

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
  { to: '/agents', label: 'Agent 运行', icon: <RobotOutlined /> },
]

// 超管是独立身份、不属于任何组织：工作区只留系统总览，业务视图整体退出；
// 管理组换成系统配置 + 超管子应用两个入口。
const superAdminWorkspaces = [
  { to: '/', label: '系统总览', icon: <AppstoreOutlined />, end: true },
]

const superAdminAdminWorkspaces = [
  { to: '/settings', label: '系统配置', icon: <SettingOutlined /> },
  { to: '/super-admin/tenants', label: '组织管理', icon: <BankOutlined /> },
  { to: '/super-admin/audit-log', label: '审计日志', icon: <AuditOutlined /> },
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
  const { role, superAdmin } = useWorkspaceSession()
  const member = role === 'member' && !superAdmin
  // 超管无租户身份：菜单整体收敛为系统级三件事（系统总览 / 系统配置 /
  // 组织管理+审计日志），Agent 观测组对超管隐藏；支持访问中的超管 role 视作
  // tenant_admin，走组织侧完整菜单，不受此收敛影响。
  const superAdminSolo = role === 'super_admin'
  const workspaceItems = member
    ? [
        { to: '/', label: '工作台', icon: <AppstoreOutlined />, end: true },
        { to: '/requirements', label: '需求管理', icon: <BulbOutlined /> },
        { to: '/tasks', label: '任务查询', icon: <ProfileOutlined /> },
        { to: '/explorer', label: '数据查询', icon: <DatabaseOutlined /> },
      ]
    : superAdminSolo
      ? superAdminWorkspaces
      : workspaces
  const adminItems = member
    ? []
    : superAdminSolo
      ? superAdminAdminWorkspaces
      : [
          // 支持访问中的超管（superAdmin 标记仍在）可继续直达系统配置排障。
          ...adminWorkspaces.filter((item) => item.to !== '/settings' || superAdmin),
          ...(role === 'tenant_admin' ? [{ to: '/org', label: '组织管理', icon: <BankOutlined /> }] : []),
        ]
  const agentItems = superAdminSolo ? [] : agentWorkspaces

  return (
    <div className={`control-room${collapsed ? ' control-room--collapsed' : ''}`}>
      <aside className="control-sidebar">
        <header className="brand-block">
          <div className="brand-mark" aria-hidden="true"><span /><span /><span /></div>
          <div><strong>Coordinator</strong><span>Control room</span></div>
        </header>

        <nav className="workspace-nav" aria-label="管理台工作区">
          <div className="nav-caption">工作区</div>
          {workspaceItems.map((item) => (
            <NavLink key={item.to} to={withRepository(item.to, repositoryID)} end={'end' in item ? item.end : false} title={item.label}>
              {item.icon}<span className="nav-label">{item.label}</span>
            </NavLink>
          ))}
          {agentItems.length > 0 ? (
            <>
              <div className="nav-group-title">Agent 观测</div>
              {agentItems.map((item) => (
                <NavLink key={item.to} to={withRepository(item.to, repositoryID)} title={item.label}>
                  {item.icon}<span className="nav-label">{item.label}</span>
                </NavLink>
              ))}
            </>
          ) : null}
          {adminItems.length > 0 ? (
            <>
              <div className="nav-group-title">管理与配置</div>
              {adminItems.map((item) => (
                <NavLink key={item.to} to={withRepository(item.to, repositoryID)} title={item.label}>
                  {item.icon}<span className="nav-label">{item.label}</span>
                </NavLink>
              ))}
            </>
          ) : null}
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
          {/* 外观主题入口：本地浏览器偏好，对所有登录角色开放（含无 /settings 路由的成员）。 */}
          <AppearanceDrawerButton />
          <RepositorySwitcher />
        </header>
        <PageHeadingHostContext.Provider value={pageHeadingHost}>
          <main className="workspace-canvas">{children}</main>
        </PageHeadingHostContext.Provider>
      </section>
    </div>
  )
}
