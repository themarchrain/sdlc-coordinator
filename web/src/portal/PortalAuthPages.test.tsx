import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { portalApi, type PortalSessionPayload } from '../api/portal'
import { PortalRegisterPage } from './PortalRegisterPage'
import { SelectTenantPage } from './SelectTenantPage'
import { PortalChangePasswordPage } from './PortalChangePasswordPage'

afterEach(() => vi.restoreAllMocks())

function LocationProbe() {
  const location = useLocation()
  return <span data-testid="location">{location.pathname}</span>
}

const registered: PortalSessionPayload = {
  user: { id: 4, username: 'newcomer', role: 'user', must_change_password: false },
  scope: { kind: 'tenant', role: 'member', tenant: { id: 2, slug: 'acme', display_name: 'Acme 组织', status: 'active' } },
}

describe('PortalRegisterPage', () => {
  it('registers with username, password and invite code', async () => {
    const user = userEvent.setup()
    vi.spyOn(portalApi, 'register').mockResolvedValue(registered)
    render(
      <MemoryRouter initialEntries={['/portal/register']}>
        <Routes>
          <Route path="/portal/register" element={<PortalRegisterPage />} />
          <Route path="/" element={<LocationProbe />} />
        </Routes>
      </MemoryRouter>,
    )
    await user.type(screen.getByLabelText('用户名'), 'newcomer')
    await user.type(screen.getByLabelText('密码'), 'password-001')
    await user.type(screen.getByLabelText('邀请码'), 'ABCD-EFGH-JKLM')
    await user.click(screen.getByRole('button', { name: '注册并加入组织' }))
    expect(portalApi.register).toHaveBeenCalledWith('newcomer', 'password-001', 'ABCD-EFGH-JKLM')
    expect(await screen.findByTestId('location')).toHaveTextContent('/')
  })
})

describe('SelectTenantPage', () => {
  it('lists joined tenants and binds the chosen one', async () => {
    const user = userEvent.setup()
    vi.spyOn(portalApi, 'session').mockResolvedValue({
      user: { id: 5, username: 'multi', role: 'user', must_change_password: false },
      scope: { kind: 'unscoped' },
      tenants: [
        { id: 1, slug: 'alpha', display_name: 'Alpha 组织', role: 'member' },
        { id: 2, slug: 'beta', display_name: 'Beta 组织', role: 'tenant_admin' },
      ],
    })
    vi.spyOn(portalApi, 'selectTenant').mockResolvedValue({
      user: { id: 5, username: 'multi', role: 'user', must_change_password: false },
      scope: { kind: 'tenant', role: 'tenant_admin', tenant: { id: 2, slug: 'beta', display_name: 'Beta 组织', status: 'active' } },
    })
    render(
      <MemoryRouter initialEntries={['/portal/select-tenant']}>
        <Routes>
          <Route path="/portal/select-tenant" element={<SelectTenantPage />} />
          <Route path="/" element={<LocationProbe />} />
        </Routes>
      </MemoryRouter>,
    )
    expect(await screen.findByText('Beta 组织')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /Beta 组织/ }))
    expect(portalApi.selectTenant).toHaveBeenCalledWith(2)
    expect(await screen.findByTestId('location')).toHaveTextContent('/')
  })
})

describe('PortalChangePasswordPage', () => {
  it('changes password and returns to login', async () => {
    const user = userEvent.setup()
    vi.spyOn(portalApi, 'changePassword').mockResolvedValue({ ok: true })
    render(
      <MemoryRouter initialEntries={['/portal/change-password']}>
        <Routes>
          <Route path="/portal/change-password" element={<PortalChangePasswordPage />} />
          <Route path="/portal/login" element={<LocationProbe />} />
        </Routes>
      </MemoryRouter>,
    )
    await user.type(screen.getByLabelText('当前密码'), 'initial-pass-01')
    await user.type(screen.getByLabelText('新密码'), 'changed-pass-01')
    await user.type(screen.getByLabelText('确认新密码'), 'changed-pass-01')
    await user.click(screen.getByRole('button', { name: '修改密码' }))
    expect(portalApi.changePassword).toHaveBeenCalledWith('initial-pass-01', 'changed-pass-01')
    expect(await screen.findByTestId('location')).toHaveTextContent('/portal/login')
  })
})
