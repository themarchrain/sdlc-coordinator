import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import { portalApi } from '../api/portal'
import { App } from './App'

afterEach(() => vi.restoreAllMocks())

// 组织管理内嵌工作台：按 App.tsx 的生产挂载形态渲染完整 App，
// 断言组织页在 AppShell 主区内可用，菜单直达 /org；portal 区不再承载该页。
describe('workbench org workspace', () => {
  it('hosts org management inside the app shell at /org for tenant admins', async () => {
    vi.spyOn(portalApi, 'session').mockResolvedValue({
      user: { id: 2, username: 'ta-acme', role: 'user', must_change_password: false },
      scope: { kind: 'tenant', role: 'tenant_admin' },
    })
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    vi.spyOn(adminApi, 'listRepositories').mockResolvedValue({ items: [] })
    vi.spyOn(portalApi, 'tenantProfile').mockResolvedValue({
      tenant: { id: 2, slug: 'acme', display_name: 'Acme 组织', status: 'active' },
      role: 'tenant_admin',
      support: false,
    })
    vi.spyOn(portalApi, 'listMembers').mockResolvedValue({ members: [] })
    vi.spyOn(portalApi, 'listInviteCodes').mockResolvedValue({ invite_codes: [] })

    const { container } = render(<MemoryRouter initialEntries={['/org']}><App /></MemoryRouter>)

    expect(await screen.findByText('组织信息')).toBeInTheDocument()
    expect(container.querySelector('.workspace-canvas')).toContainElement(screen.getByText('组织信息'))
    expect(screen.getByText('Coordinator')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /组织管理/ })).toHaveAttribute('href', '/org')
    expect(screen.getByRole('link', { name: /组织管理/ })).toHaveAttribute('aria-current', 'page')
  })
})
