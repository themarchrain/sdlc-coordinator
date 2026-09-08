import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { portalApi, type PortalSessionPayload } from '../api/portal'
import { PortalLoginPage } from './PortalLoginPage'

afterEach(() => vi.restoreAllMocks())

function LocationProbe() {
  const location = useLocation()
  return <span data-testid="location">{location.pathname}</span>
}

const tenantSession: PortalSessionPayload = {
  user: { id: 1, username: 'member', role: 'user', must_change_password: false },
  scope: { kind: 'tenant', role: 'member', tenant: { id: 2, slug: 'acme', display_name: 'Acme 组织', status: 'active' } },
}

describe('PortalLoginPage', () => {
  it('renders 组织 wording only (no 租户/公司/企业 labels)', () => {
    render(
      <MemoryRouter initialEntries={['/portal/login']}>
        <Routes>
          <Route path="/portal/login" element={<PortalLoginPage />} />
        </Routes>
      </MemoryRouter>,
    )
    expect(screen.getByRole('button', { name: /登\s*录/ })).toBeInTheDocument()
    expect(screen.getByText('注册新账号')).toBeInTheDocument()
    expect(document.body.textContent).not.toContain('租户')
    expect(document.body.textContent).not.toContain('公司')
    expect(document.body.textContent).not.toContain('企业')
  })

  it('navigates to org page for tenant scoped sessions', async () => {
    const user = userEvent.setup()
    vi.spyOn(portalApi, 'login').mockResolvedValue(tenantSession)
    render(
      <MemoryRouter initialEntries={['/portal/login']}>
        <Routes>
          <Route path="/portal/login" element={<PortalLoginPage />} />
          <Route path="/portal/org" element={<LocationProbe />} />
        </Routes>
      </MemoryRouter>,
    )
    await user.type(screen.getByLabelText('用户名'), 'member')
    await user.type(screen.getByLabelText('密码'), 'password-001')
    await user.click(screen.getByRole('button', { name: /登\s*录/ }))
    expect(portalApi.login).toHaveBeenCalledWith('member', 'password-001')
    expect(await screen.findByTestId('location')).toHaveTextContent('/portal/org')
  })

  it('navigates to super admin area for super admin sessions', async () => {
    const user = userEvent.setup()
    vi.spyOn(portalApi, 'login').mockResolvedValue({
      user: { id: 9, username: 'root', role: 'super_admin', must_change_password: false },
      scope: { kind: 'super_admin', role: 'super_admin' },
    })
    render(
      <MemoryRouter initialEntries={['/portal/login']}>
        <Routes>
          <Route path="/portal/login" element={<PortalLoginPage />} />
          <Route path="/super-admin/tenants" element={<LocationProbe />} />
        </Routes>
      </MemoryRouter>,
    )
    await user.type(screen.getByLabelText('用户名'), 'root')
    await user.type(screen.getByLabelText('密码'), 'password-001')
    await user.click(screen.getByRole('button', { name: /登\s*录/ }))
    expect(await screen.findByTestId('location')).toHaveTextContent('/super-admin/tenants')
  })

  it('navigates to tenant selection for unscoped sessions', async () => {
    const user = userEvent.setup()
    vi.spyOn(portalApi, 'login').mockResolvedValue({
      user: { id: 5, username: 'multi', role: 'user', must_change_password: false },
      scope: { kind: 'unscoped' },
      tenants: [
        { id: 1, slug: 'a', display_name: 'A', role: 'member' },
        { id: 2, slug: 'b', display_name: 'B', role: 'member' },
      ],
    })
    render(
      <MemoryRouter initialEntries={['/portal/login']}>
        <Routes>
          <Route path="/portal/login" element={<PortalLoginPage />} />
          <Route path="/portal/select-tenant" element={<LocationProbe />} />
        </Routes>
      </MemoryRouter>,
    )
    await user.type(screen.getByLabelText('用户名'), 'multi')
    await user.type(screen.getByLabelText('密码'), 'password-001')
    await user.click(screen.getByRole('button', { name: /登\s*录/ }))
    expect(await screen.findByTestId('location')).toHaveTextContent('/portal/select-tenant')
  })
})
