import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { portalApi, type PortalSessionPayload, type PortalTenantSummary } from '../api/portal'
import { TenantsPage } from './TenantsPage'
import { TenantDetailPage } from './TenantDetailPage'
import { AuditLogPage } from './AuditLogPage'
import { SuperAdminApp } from './SuperAdminApp'

afterEach(() => vi.restoreAllMocks())

const tenants: PortalTenantSummary[] = [
  { id: 1, slug: 'acme', display_name: 'Acme 组织', status: 'active', member_count: 5, created_at: '2026-09-01T00:00:00Z' },
  { id: 2, slug: 'beta', display_name: 'Beta 组织', status: 'disabled', member_count: 3, created_at: '2026-09-02T00:00:00Z' },
]

describe('TenantsPage', () => {
  it('renders the tenant table and lifecycle actions with 组织 wording', async () => {
    vi.spyOn(portalApi, 'superListTenants').mockResolvedValue({ tenants })
    render(
      <MemoryRouter initialEntries={['/super-admin/tenants']}>
        <Routes>
          <Route path="/super-admin/tenants" element={<TenantsPage />} />
        </Routes>
      </MemoryRouter>,
    )
    expect(await screen.findByText('Acme 组织')).toBeInTheDocument()
    expect(screen.getByText('Beta 组织')).toBeInTheDocument()
    expect(document.body.textContent).toContain('已停用')
    expect(document.body.textContent).not.toContain('租户')
    expect(document.body.textContent).not.toContain('公司')
    expect(document.body.textContent).not.toContain('企业')
  })

  it('creates a tenant via the dialog', async () => {
    const user = userEvent.setup()
    vi.spyOn(portalApi, 'superListTenants').mockResolvedValue({ tenants })
    const created = vi.spyOn(portalApi, 'superCreateTenant').mockResolvedValue({
      tenant: { id: 3, slug: 'gamma', display_name: 'Gamma 组织', status: 'active' },
    })
    render(
      <MemoryRouter initialEntries={['/super-admin/tenants']}>
        <Routes>
          <Route path="/super-admin/tenants" element={<TenantsPage />} />
        </Routes>
      </MemoryRouter>,
    )
    await user.click(await screen.findByRole('button', { name: /新建组织/ }))
    await user.type(await screen.findByLabelText('组织标识'), 'gamma')
    await user.type(screen.getByLabelText('组织名称'), 'Gamma 组织')
    await user.click(screen.getByRole('button', { name: /创\s*建/ }))
    await waitFor(() => expect(created).toHaveBeenCalledWith('gamma', 'Gamma 组织'))
  })
})

describe('TenantDetailPage', () => {
  it('provisions a tenant admin and reveals the one-time initial password', async () => {
    const user = userEvent.setup()
    vi.spyOn(portalApi, 'superListTenantMembers').mockResolvedValue({ members: [] })
    vi.spyOn(portalApi, 'superListInviteCodes').mockResolvedValue({ invite_codes: [] })
    vi.spyOn(portalApi, 'superListRunnerAssignments').mockResolvedValue({ runner_assignments: [] })
    const provision = vi.spyOn(portalApi, 'superProvisionAdmin').mockResolvedValue({
      user_id: 9, username: 'new-admin', initial_password: 'INITIAL-PASS-01',
    })

    render(
      <MemoryRouter initialEntries={['/super-admin/tenants/1']}>
        <Routes>
          <Route path="/super-admin/tenants/:id" element={<TenantDetailPage />} />
        </Routes>
      </MemoryRouter>,
    )
    await user.type(await screen.findByPlaceholderText('用户名'), 'new-admin')
    await user.click(screen.getByRole('button', { name: /开通组织管理员/ }))
    await waitFor(() => expect(provision).toHaveBeenCalledWith(1, 'new-admin'))
    expect(await screen.findByText(/INITIAL-PASS-01/)).toBeInTheDocument()
  })
})

describe('AuditLogPage', () => {
  it('loads audit entries with filters', async () => {
    const listed = vi.spyOn(portalApi, 'superListAuditLog').mockResolvedValue({
      items: [
        {
          id: 1, actor_user_id: 9, actor_username: 'root', tenant_id: 1,
          action: 'support_access.entered', detail: 'reason=排障', created_at: '2026-09-04T00:00:00Z',
        },
      ],
      total: 1, page: 1, page_size: 20,
    })
    render(
      <MemoryRouter initialEntries={['/super-admin/audit-log']}>
        <Routes>
          <Route path="/super-admin/audit-log" element={<AuditLogPage />} />
        </Routes>
      </MemoryRouter>,
    )
    expect(await screen.findByText('support_access.entered')).toBeInTheDocument()
    expect(screen.getByText(/排障/)).toBeInTheDocument()
    expect(listed).toHaveBeenCalled()
  })
})

describe('SuperAdminApp shell', () => {
  it('exposes a dedicated menu area separate from tenant pages', async () => {
    vi.spyOn(portalApi, 'session').mockResolvedValue({
      user: { id: 9, username: 'root', role: 'super_admin', must_change_password: false },
      scope: { kind: 'super_admin', role: 'super_admin' },
    })
    vi.spyOn(portalApi, 'superListTenants').mockResolvedValue({ tenants })
    render(
      <MemoryRouter initialEntries={['/super-admin']}>
        <SuperAdminApp />
      </MemoryRouter>,
    )
    const orgEntries = await screen.findAllByText('组织管理')
    expect(orgEntries.length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText('审计日志')).toBeInTheDocument()
    expect(screen.getByRole('menu')).toBeInTheDocument()
  })
})

describe('SuperAdminApp mounting shapes', () => {
  const superSession: PortalSessionPayload = {
    user: { id: 9, username: 'root', role: 'super_admin', must_change_password: false },
    scope: { kind: 'super_admin', role: 'super_admin' },
  }

  it('renders the shell and tenant rows through the production route mounting', async () => {
    // 生产形态锁定（App.tsx 真实挂法）：必须带 super-admin/* 父路由；
    // 不带父路由的直接渲染形态匹配不到叶子路由，单独覆盖下一条。
    vi.spyOn(portalApi, 'session').mockResolvedValue(superSession)
    vi.spyOn(portalApi, 'superListTenants').mockResolvedValue({ tenants })
    render(
      <MemoryRouter initialEntries={['/super-admin/tenants']}>
        <Routes>
          <Route path="super-admin/*" element={<SuperAdminApp />} />
        </Routes>
      </MemoryRouter>,
    )
    expect(await screen.findByTestId('super-admin-shell')).toBeInTheDocument()
    expect(await screen.findByText('Acme 组织')).toBeInTheDocument()
    expect(screen.getByText('Beta 组织')).toBeInTheDocument()
  })

  it('stays usable when mounted directly without the super-admin/* parent route', async () => {
    // 直接渲染形态（无父路由）：重构前叶子路由匹配不到、主区空白（红）；
    // 重构后兜底导航把无前缀 URL 规范到叶子路由，两种挂载形态都可用。
    vi.spyOn(portalApi, 'session').mockResolvedValue(superSession)
    vi.spyOn(portalApi, 'superListTenants').mockResolvedValue({ tenants })
    render(
      <MemoryRouter initialEntries={['/super-admin/tenants']}>
        <SuperAdminApp />
      </MemoryRouter>,
    )
    expect(await screen.findByTestId('super-admin-shell')).toBeInTheDocument()
    expect(await screen.findByText('Acme 组织')).toBeInTheDocument()
  })
})

describe('SuperAdminSessionGate', () => {
  function LocationProbe() {
    const location = useLocation()
    return <span data-testid="location">{location.pathname}</span>
  }

  it('redirects unauthenticated visitors to the portal login instead of firing protected APIs', async () => {
    vi.spyOn(portalApi, 'session').mockRejectedValue(new Error('401'))
    const superListTenants = vi.spyOn(portalApi, 'superListTenants').mockResolvedValue({ tenants })
    render(
      <MemoryRouter initialEntries={['/super-admin/tenants']}>
        <Routes>
          <Route path="/super-admin/*" element={<SuperAdminApp />} />
          <Route path="/portal/login" element={<LocationProbe />} />
        </Routes>
      </MemoryRouter>,
    )
    expect(await screen.findByTestId('location')).toHaveTextContent('/portal/login')
    expect(superListTenants).not.toHaveBeenCalled()
  })

  it('redirects non-super-admin sessions to the portal login', async () => {
    vi.spyOn(portalApi, 'session').mockResolvedValue({
      user: { id: 3, username: 'plain', role: 'user', must_change_password: false },
      scope: { kind: 'tenant', role: 'member' },
    })
    render(
      <MemoryRouter initialEntries={['/super-admin/tenants']}>
        <Routes>
          <Route path="/super-admin/*" element={<SuperAdminApp />} />
          <Route path="/portal/login" element={<LocationProbe />} />
        </Routes>
      </MemoryRouter>,
    )
    expect(await screen.findByTestId('location')).toHaveTextContent('/portal/login')
  })
})

describe('portal gate for super admins', () => {
  it('redirects super admin without support access away from tenant pages', async () => {
    vi.spyOn(portalApi, 'session').mockResolvedValue({
      user: { id: 9, username: 'root', role: 'super_admin', must_change_password: false },
      scope: { kind: 'super_admin', role: 'super_admin' },
    })
    function LocationProbe() {
      const location = useLocation()
      return <span data-testid="location">{location.pathname}</span>
    }
    const { PortalApp } = await import('../portal/PortalApp')
    render(
      <MemoryRouter initialEntries={['/portal/org']}>
        <Routes>
          <Route path="/portal/*" element={<PortalApp />} />
          <Route path="/super-admin/tenants" element={<LocationProbe />} />
        </Routes>
      </MemoryRouter>,
    )
    expect(await screen.findByTestId('location')).toHaveTextContent('/super-admin/tenants')
  })

  it('keeps super admin with active support access on the org page', async () => {
    vi.spyOn(portalApi, 'session').mockResolvedValue({
      user: { id: 9, username: 'root', role: 'super_admin', must_change_password: false },
      scope: { kind: 'super_admin', role: 'super_admin' },
      support: { active: true, reason: '排障', entered_at: '2026-09-04T00:00:00Z' },
    })
    vi.spyOn(portalApi, 'tenantProfile').mockResolvedValue({
      tenant: { id: 1, slug: 'acme', display_name: 'Acme 组织', status: 'active' },
      role: 'tenant_admin',
      support: true,
    })
    vi.spyOn(portalApi, 'listMembers').mockResolvedValue({ members: [] })
    vi.spyOn(portalApi, 'listInviteCodes').mockResolvedValue({ invite_codes: [] })
    const { PortalApp } = await import('../portal/PortalApp')
    render(
      <MemoryRouter initialEntries={['/portal/org']}>
        <Routes>
          <Route path="/portal/*" element={<PortalApp />} />
          <Route path="/super-admin/tenants" element={<div>super home</div>} />
        </Routes>
      </MemoryRouter>,
    )
    expect(await screen.findByText('Acme 组织')).toBeInTheDocument()
  })
})
