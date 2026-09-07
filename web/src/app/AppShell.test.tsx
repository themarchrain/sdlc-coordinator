import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import { AppShell } from './AppShell'
import { PageHeading } from '../components/PageHeading'
import '../styles/index.css'

afterEach(() => vi.restoreAllMocks())

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
        <AppShell><div>任务工作区内容</div></AppShell>
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

  it('groups person directory and system configuration under one admin block', () => {
    mockRepositories()
    render(<MemoryRouter initialEntries={['/settings']}><AppShell><div>配置内容</div></AppShell></MemoryRouter>)

    expect(screen.getByText('管理与配置')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /人员资产/ })).toHaveAttribute('href', '/people')
    expect(screen.getByRole('link', { name: /系统配置/ })).toHaveAttribute('aria-current', 'page')
  })

  it('adds repository management to the workspace and demotes connectors to the admin footer', () => {
    mockRepositories()
    const { container } = render(
      <MemoryRouter initialEntries={['/repositories']}>
        <AppShell><div>仓库内容</div></AppShell>
      </MemoryRouter>,
    )

    const titles = [...container.querySelectorAll('.workspace-nav a')].map((link) => link.getAttribute('title'))
    expect(titles).toEqual([
      '运行总览', '需求管理', '仓库管理', '任务查询', '数据查询',
      'Agent 运行',
      '人员资产', '系统配置', '安全操作', '连接器实例',
    ])
    expect(screen.getByRole('link', { name: /仓库管理/ })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('link', { name: /连接器实例/ })).toHaveAttribute('href', '/connectors')
    expect(screen.queryByText('状态源连接器')).not.toBeInTheDocument()
  })

  it('defaults the top-bar switcher to 全部仓库', async () => {
    mockRepositories()
    render(<MemoryRouter><AppShell><div>内容</div></AppShell></MemoryRouter>)

    expect(await screen.findByText('全部仓库')).toBeInTheDocument()
    expect(screen.getByText('仓库：')).toBeInTheDocument()
  })

  it('exposes the person directory as a workspace', () => {
    mockRepositories()
    render(<MemoryRouter initialEntries={['/people']}><AppShell><div>人员内容</div></AppShell></MemoryRouter>)
    expect(screen.getByRole('link', { name: /人员资产/ })).toHaveAttribute('aria-current', 'page')
  })

  it('collapses to an icon rail and expands back', () => {
    mockRepositories()
    const { container } = render(
      <MemoryRouter>
        <AppShell><div>内容</div></AppShell>
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
        <AppShell><div>任务内容</div></AppShell>
      </MemoryRouter>,
    )

    expect(await screen.findByRole('link', { name: /运行总览/ })).toHaveAttribute('href', '/?repository_id=repo_a')
    expect(screen.getByRole('link', { name: /数据查询/ })).toHaveAttribute('href', '/explorer?repository_id=repo_a')
    expect(screen.getByRole('link', { name: /系统配置/ })).toHaveAttribute('href', '/settings?repository_id=repo_a')
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
})
