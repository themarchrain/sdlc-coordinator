import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import { portalApi, type PortalTenantSummary } from '../api/portal'
import { App } from '../app/App'
import { SuperAdminOverviewPage } from './SuperAdminOverviewPage'

afterEach(() => vi.restoreAllMocks())

const tenants: PortalTenantSummary[] = [
  { id: 101, slug: 'acme', display_name: 'Acme 组织', status: 'active', member_count: 5, created_at: '2026-09-01T00:00:00Z' },
  { id: 102, slug: 'beta', display_name: 'Beta 组织', status: 'disabled', member_count: 3, created_at: '2026-09-02T00:00:00Z' },
]

describe('SuperAdminOverviewPage', () => {
  it('locks the tenant and member totals with the tenant table', async () => {
    vi.spyOn(portalApi, 'superListTenants').mockResolvedValue({ tenants })

    render(<MemoryRouter><SuperAdminOverviewPage /></MemoryRouter>)

    expect(await screen.findByText('Acme 组织')).toBeInTheDocument()
    expect(screen.getByText('Beta 组织')).toBeInTheDocument()
    expect(screen.getByText('组织总数')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
    expect(screen.getByText('成员总数')).toBeInTheDocument()
    expect(screen.getByText('8')).toBeInTheDocument()
    expect(screen.getByText('acme')).toBeInTheDocument()
    expect(screen.getByText('已停用')).toBeInTheDocument()
  })

  it('renders the empty state when no tenants exist', async () => {
    vi.spyOn(portalApi, 'superListTenants').mockResolvedValue({ tenants: [] })

    render(<MemoryRouter><SuperAdminOverviewPage /></MemoryRouter>)

    expect(await screen.findByText('还没有组织')).toBeInTheDocument()
    // 两张统计卡在空态下都归零。
    expect(screen.getAllByText('0')).toHaveLength(2)
  })

  it('renders the error state when listing fails', async () => {
    vi.spyOn(portalApi, 'superListTenants').mockRejectedValue(new Error('boom'))

    render(<MemoryRouter><SuperAdminOverviewPage /></MemoryRouter>)

    expect(await screen.findByText('数据加载失败')).toBeInTheDocument()
    expect(screen.getByText('boom')).toBeInTheDocument()
  })

  it('shows the loading skeleton before the payload arrives', () => {
    vi.spyOn(portalApi, 'superListTenants').mockReturnValue(new Promise(() => undefined))

    render(<MemoryRouter><SuperAdminOverviewPage /></MemoryRouter>)

    expect(screen.getByLabelText('正在加载')).toBeInTheDocument()
  })
})

// 首页按身份分流：无支持访问的超管落在系统总览，而不是组织级运行总览。
describe('super admin home route', () => {
  it('renders the super admin overview at / inside the app shell', async () => {
    vi.spyOn(portalApi, 'session').mockResolvedValue({
      user: { id: 9, username: 'root', role: 'super_admin', must_change_password: false },
      scope: { kind: 'super_admin', role: 'super_admin' },
    })
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    vi.spyOn(adminApi, 'listRepositories').mockResolvedValue({ items: [] })
    vi.spyOn(portalApi, 'superListTenants').mockResolvedValue({ tenants })

    render(<MemoryRouter initialEntries={['/']}><App /></MemoryRouter>)

    expect(await screen.findByText('组织总数')).toBeInTheDocument()
    expect(await screen.findByRole('link', { name: /系统总览/ })).toHaveAttribute('aria-current', 'page')
    expect(screen.queryByText('最近失败')).not.toBeInTheDocument()
  })
})
