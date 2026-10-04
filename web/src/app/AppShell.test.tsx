import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import { AppShell } from './AppShell'
import { PageHeading } from '../components/PageHeading'
import { WorkspaceSessionProvider, type WorkspaceSessionValue } from './workspaceSession'
import '../styles/index.css'

afterEach(() => vi.restoreAllMocks())

const memberWorkspace: WorkspaceSessionValue = { session: null, role: 'member', superAdmin: false }
const tenantAdminWorkspace: WorkspaceSessionValue = { session: null, role: 'tenant_admin', superAdmin: false }
const superAdminWorkspace: WorkspaceSessionValue = { session: null, role: 'super_admin', superAdmin: true }

function mockRepositories() {
  vi.spyOn(adminApi, 'listRepositories').mockResolvedValue({
    items: [{
      repository_id: 'repo_a', display_name: 'Repo A',
provider: 'gitlab',
base_url: 'https://gitlab.example.com', codehost_project_id: '1', path_with_namespace: 'group/a',
      token_configured: true, webhook_secret_configured: true,default_branch: 'main', status: 'enabled',
last_configuration_error: '',
    }],
  })
}

describe('AppShell', () => {
  it('marks the active workspace and keeps the page content visible', () => {
    mockRepositories()
    render(
      <MemoryRouter initialEntries={['/tasks']}>
        <WorkspaceSessionProvider value={tenantAdminWorkspace}>
          <AppShell><div>任务工作区内容</div></AppShell>
        </WorkspaceSessionProvider>
      </MemoryRouter>,
    )

    expect(screen.getByText('Coordinator')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /任务查询/ })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByText('任务工作区内容')).toBeInTheDocument()
    expect(screen.queryByText('状态由 Coordinator 后端统一推进')).not.toBeInTheDocument()
    expect(screen.queryByText('边界明确')).not.toBeInTheDocument()
    expect(screen.queryByText('浏览器仅连接 Coordinator')).not.toBeInTheDocument()
    expect(screen.queryByText('管理台需登录，运行接口保持独立')).not.toBeInTheDocument()
  })

	  it('exposes a provider-agnostic agent observability workspace', () => {
	    render(<MemoryRouter initialEntries={['/agents']}><AppShell><div>Agent 观测内容</div></AppShell></MemoryRouter>)

	    expect(screen.getByText('Agent 观测')).toBeInTheDocument()
	    expect(screen.getByRole('link', { name: /Agent 运行/ })).toHaveAttribute('aria-current', 'page')
	  })

  it('groups person directory and org management under the admin block for tenant admins', () => {
    mockRepositories()
    render(<MemoryRouter initialEntries={['/people']}><WorkspaceSessionProvider value={tenantAdminWorkspace}><AppShell><div>人员内容</div></AppShell></WorkspaceSessionProvider></MemoryRouter>)

    expect(screen.getByText('管理与配置')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /人员资产/ })).toHaveAttribute('href', '/people')
    expect(screen.getByRole('link', { name: /组织管理/ })).toHaveAttribute('href', '/org')
    expect(screen.queryByRole('link', { name: /系统配置/ })).not.toBeInTheDocument()
  })

  it('converges the super admin menu to system-level entries only', () => {
    mockRepositories()
    render(<MemoryRouter initialEntries={['/settings']}><WorkspaceSessionProvider value={superAdminWorkspace}><AppShell><div>超管内容</div></AppShell></WorkspaceSessionProvider></MemoryRouter>)

    expect(screen.getByRole('link', { name: /系统配置/ })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('link', { name: /系统总览/ })).toHaveAttribute('href', '/')
    expect(screen.getByRole('link', { name: /组织管理/ })).toHaveAttribute('href', '/super-admin/tenants')
    expect(screen.getByRole('link', { name: /审计日志/ })).toHaveAttribute('href', '/super-admin/audit-log')
    // 业务视图与 Agent 观测组对超管整体退出；管理组不再挂组织级页面。
    expect(screen.queryByRole('link', { name: /运行总览/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /Agent 运行/ })).not.toBeInTheDocument()
    expect(screen.queryByText('Agent 观测')).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /需求管理/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /仓库管理/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /任务查询/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /数据查询/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /人员资产/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /安全操作/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /连接器实例/ })).not.toBeInTheDocument()
  })

  it('adds repository management to the workspace and demotes connectors to the admin footer', () => {
    mockRepositories()
    const { container } = render(
      <MemoryRouter initialEntries={['/repositories']}>
        <WorkspaceSessionProvider value={tenantAdminWorkspace}>
          <AppShell><div>仓库内容</div></AppShell>
        </WorkspaceSessionProvider>
      </MemoryRouter>,
    )

    const titles = [...container.querySelectorAll('.workspace-nav a')].map((link) => link.getAttribute('title'))
    expect(titles).toEqual([
      '运行总览', '需求管理', '仓库管理', '任务查询', '数据查询',
      'Agent 运行',
      '人员资产', '安全操作', '连接器实例', '组织管理',
    ])
    expect(screen.getByRole('link', { name: /仓库管理/ })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('link', { name: /连接器实例/ })).toHaveAttribute('href', '/connectors')
    expect(screen.queryByText('状态源连接器')).not.toBeInTheDocument()
  })

  it('hides repository management and other admin workspaces from members', () => {
    mockRepositories()
    render(
      <MemoryRouter initialEntries={['/']}>
        <WorkspaceSessionProvider value={memberWorkspace}>
          <AppShell><div>成员内容</div></AppShell>
        </WorkspaceSessionProvider>
      </MemoryRouter>,
    )

    expect(screen.getByRole('link', { name: /工作台/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /需求管理/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /任务查询/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /数据查询/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Agent 运行/ })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /仓库管理/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /人员资产/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /系统配置/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /安全操作/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /连接器实例/ })).not.toBeInTheDocument()
    expect(screen.queryByText('管理与配置')).not.toBeInTheDocument()
  })

  it('keeps repository management for tenant admins and hides system configuration', () => {
    mockRepositories()
    render(
      <MemoryRouter initialEntries={['/repositories']}>
        <WorkspaceSessionProvider value={tenantAdminWorkspace}>
          <AppShell><div>仓库内容</div></AppShell>
        </WorkspaceSessionProvider>
      </MemoryRouter>,
    )

    expect(screen.getByRole('link', { name: /仓库管理/ })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('link', { name: /人员资产/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /组织管理/ })).toHaveAttribute('href', '/org')
    expect(screen.queryByRole('link', { name: /系统配置/ })).not.toBeInTheDocument()
  })

  it('defaults the top-bar switcher to 全部仓库', async () => {
    mockRepositories()
    render(<MemoryRouter><AppShell><div>内容</div></AppShell></MemoryRouter>)

    expect(await screen.findByText('全部仓库')).toBeInTheDocument()
    expect(screen.getByText('仓库：')).toBeInTheDocument()
  })

  it('exposes the person directory as a workspace', () => {
    mockRepositories()
    render(<MemoryRouter initialEntries={['/people']}><WorkspaceSessionProvider value={tenantAdminWorkspace}><AppShell><div>人员内容</div></AppShell></WorkspaceSessionProvider></MemoryRouter>)
    expect(screen.getByRole('link', { name: /人员资产/ })).toHaveAttribute('aria-current', 'page')
  })

  it('collapses to an icon rail and expands back', () => {
    mockRepositories()
    const { container } = render(
      <MemoryRouter>
        <WorkspaceSessionProvider value={tenantAdminWorkspace}>
          <AppShell><div>内容</div></AppShell>
        </WorkspaceSessionProvider>
      </MemoryRouter>,
    )

    expect(container.querySelector('.control-room')).not.toHaveClass('control-room--collapsed')
    expect(screen.getByRole('link', { name: /运行总览/ })).toHaveAttribute('title', '运行总览')

    const toggle = screen.getByRole('button', { name: '折叠侧边栏' })
    expect(container.querySelector('.control-sidebar')?.lastElementChild).toBe(toggle)
    expect(window.getComputedStyle(toggle).marginTop).toBe('auto')
    fireEvent.click(toggle)
    expect(container.querySelector('.control-room')).toHaveClass('control-room--collapsed')
    expect(window.getComputedStyle(toggle).marginTop).toBe('auto')
    expect(screen.getByTitle('运行总览')).toHaveAttribute('href', '/')
	    expect(container.querySelectorAll('.workspace-nav [role="img"]')).toHaveLength(10)
	    expect(container.querySelectorAll('.workspace-nav .nav-label')).toHaveLength(10)

    fireEvent.click(screen.getByRole('button', { name: '展开侧边栏' }))
    expect(container.querySelector('.control-room')).not.toHaveClass('control-room--collapsed')
  })

  it('keeps repository_id on workspace links so switching pages does not clear the selection', async () => {
    mockRepositories()
    render(
      <MemoryRouter initialEntries={['/tasks?repository_id=repo_a']}>
        <WorkspaceSessionProvider value={tenantAdminWorkspace}>
          <AppShell><div>任务内容</div></AppShell>
        </WorkspaceSessionProvider>
      </MemoryRouter>,
    )

    expect(await screen.findByRole('link', { name: /运行总览/ })).toHaveAttribute('href', '/?repository_id=repo_a')
    expect(screen.getByRole('link', { name: /数据查询/ })).toHaveAttribute('href', '/explorer?repository_id=repo_a')
    expect(screen.getByRole('link', { name: /安全操作/ })).toHaveAttribute('href', '/operations?repository_id=repo_a')
  })

  it('hosts the page heading in the compact top bar', async () => {
    mockRepositories()
    const { container } = render(
      <MemoryRouter>
        <AppShell><PageHeading eyebrow="协调任务状态" title="任务轨迹" /></AppShell>
      </MemoryRouter>,
    )

    const eyebrow = await screen.findByText('协调任务状态')
    await waitFor(() => expect(container.querySelector('.context-bar')).toContainElement(eyebrow))
    expect(container.querySelector('.workspace-canvas')).not.toContainElement(eyebrow)
    expect(screen.queryByRole('heading', { name: '任务轨迹' })).not.toBeInTheDocument()
  })

  it('offers the shared shell account menu with logout for every role', async () => {
    mockRepositories()
    render(
      <MemoryRouter initialEntries={['/']}>
        <WorkspaceSessionProvider value={memberWorkspace}>
          <AppShell><div>成员内容</div></AppShell>
        </WorkspaceSessionProvider>
      </MemoryRouter>,
    )

    fireEvent.click(screen.getByRole('button', { name: '账号菜单，账号' }))
    expect(await screen.findByRole('menuitem', { name: /退出登录/ })).toBeInTheDocument()
    // 账号区在折叠按钮上方、侧栏之内，不属于工作区导航链接
    const sidebar = document.querySelector('.control-sidebar')
    const account = document.querySelector('.sidebar-account') as HTMLElement | null
    expect(sidebar).toContainElement(account)
    expect(account?.nextElementSibling).toHaveClass('collapse-toggle')
  })

  it('opens the appearance drawer from the context bar for members too', async () => {
    // 主题入口迁到外壳顶栏后，没有 /settings 路由的成员也能换本地肤。
    mockRepositories()
    const { container } = render(
      <MemoryRouter>
        <WorkspaceSessionProvider value={memberWorkspace}>
          <AppShell><div>成员内容</div></AppShell>
        </WorkspaceSessionProvider>
      </MemoryRouter>,
    )

    const trigger = screen.getByRole('button', { name: '外观主题' })
    expect(container.querySelector('.context-bar')).toContainElement(trigger)
    fireEvent.click(trigger)

    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    expect(screen.getByRole('region', { name: '外观主题' })).toBeInTheDocument()
    expect(screen.getByText(/外观主题：默认/)).toBeInTheDocument()
  })
})
