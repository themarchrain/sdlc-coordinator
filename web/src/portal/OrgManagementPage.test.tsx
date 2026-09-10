import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { portalApi, type PortalInviteCodeRecord, type PortalMember } from '../api/portal'
import { OrgManagementPage } from './OrgManagementPage'

afterEach(() => vi.restoreAllMocks())

const members: PortalMember[] = [
  { user_id: 1, username: 'org-admin', role: 'tenant_admin', created_at: '2026-09-01T00:00:00Z' },
  { user_id: 2, username: 'plain-member', role: 'member', created_at: '2026-09-02T00:00:00Z' },
]

const inviteCodes: PortalInviteCodeRecord[] = [
  {
    id: 11, role: 'member', expires_at: '2026-12-01T00:00:00Z', max_uses: 10, used_count: 2,
    revoked_at: null, created_by: 1, created_at: '2026-09-03T00:00:00Z',
  },
]

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/portal/org']}>
      <Routes>
        <Route path="/portal/org" element={<OrgManagementPage />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('OrgManagementPage', () => {
  it('loads profile, members and invite codes for tenant admins', async () => {
    vi.spyOn(portalApi, 'tenantProfile').mockResolvedValue({
      tenant: { id: 2, slug: 'acme', display_name: 'Acme 组织', status: 'active' },
      role: 'tenant_admin',
      support: false,
    })
    vi.spyOn(portalApi, 'listMembers').mockResolvedValue({ members })
    vi.spyOn(portalApi, 'listInviteCodes').mockResolvedValue({ invite_codes: inviteCodes })

    renderPage()
    expect(await screen.findByText('组织信息')).toBeInTheDocument()
    expect(screen.getByText('org-admin')).toBeInTheDocument()
    expect(screen.getByText('plain-member')).toBeInTheDocument()
    expect(await screen.findByText('邀请码管理')).toBeInTheDocument()
    expect(document.body.textContent).toContain('已使用 2 / 10')
  })

  it('creates an invite code and shows the one-time plaintext once', async () => {
    const user = userEvent.setup()
    vi.spyOn(portalApi, 'tenantProfile').mockResolvedValue({
      tenant: { id: 2, slug: 'acme', display_name: 'Acme 组织', status: 'active' },
      role: 'tenant_admin',
      support: false,
    })
    vi.spyOn(portalApi, 'listMembers').mockResolvedValue({ members: [] })
    vi.spyOn(portalApi, 'listInviteCodes').mockResolvedValue({ invite_codes: [] })
    const createdCode = 'ABCD-EFGH-JKLM'
    vi.spyOn(portalApi, 'createInviteCode').mockResolvedValue({
      code: createdCode,
      record: { ...inviteCodes[0], id: 12 },
    })

    renderPage()
    await waitFor(() => expect(screen.getByRole('button', { name: '生成邀请码' })).toBeEnabled())
    await user.click(screen.getByRole('button', { name: '生成邀请码' }))
    expect(portalApi.createInviteCode).toHaveBeenCalledWith(undefined, undefined)
    expect(await screen.findByText(createdCode)).toBeInTheDocument()
  })

  it('revokes an invite code from the table action', async () => {
    const user = userEvent.setup()
    vi.spyOn(portalApi, 'tenantProfile').mockResolvedValue({
      tenant: { id: 2, slug: 'acme', display_name: 'Acme 组织', status: 'active' },
      role: 'tenant_admin',
      support: false,
    })
    vi.spyOn(portalApi, 'listMembers').mockResolvedValue({ members: [] })
    vi.spyOn(portalApi, 'listInviteCodes').mockResolvedValue({ invite_codes: inviteCodes })
    const revoke = vi.spyOn(portalApi, 'revokeInviteCode').mockResolvedValue({ ok: true })

    renderPage()
    await screen.findByText('组织信息')
    const revokeButtons = await screen.findAllByRole('button', { name: /吊\s*销/ })
    await user.click(revokeButtons[0])
    // Popconfirm 二次确认（测试环境未配 zhCN，按钮文案为 OK）
    await user.click(await screen.findByRole('button', { name: 'OK' }))
    expect(revoke).toHaveBeenCalledWith(11)
  })

  it('blocks non-admin members from management actions', async () => {
    vi.spyOn(portalApi, 'listMembers').mockResolvedValue({ members: [] })
    vi.spyOn(portalApi, 'tenantProfile').mockResolvedValue({
      tenant: { id: 2, slug: 'acme', display_name: 'Acme 组织', status: 'active' },
      role: 'member',
      support: false,
    })
    renderPage()
    expect(await screen.findByText('需要组织管理员权限')).toBeInTheDocument()
    expect(portalApi.listMembers).not.toHaveBeenCalled()
  })
})
