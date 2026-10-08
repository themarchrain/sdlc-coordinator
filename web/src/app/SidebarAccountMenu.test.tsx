import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation, useNavigationType } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { PortalApiError, portalApi, type PortalSessionPayload } from '../api/portal'
import { listSavedAccounts } from './accountRegistry'
import { SidebarAccountMenu } from './SidebarAccountMenu'
import { WorkspaceSessionProvider, workspaceRoleFromSession, type WorkspaceSessionValue } from './workspaceSession'
import '../styles/index.css'

afterEach(() => {
  vi.restoreAllMocks()
  window.localStorage.clear()
})

const superUser = { id: 1, username: 'root', role: 'super_admin', must_change_password: false }
const acmeTenant = { id: 7, slug: 'acme', display_name: 'Acme 组织', status: 'active' }

const superSession: PortalSessionPayload = {
  user: superUser,
  scope: { kind: 'super_admin', role: 'super_admin', tenant: null },
}
const supportSession: PortalSessionPayload = {
  user: superUser,
  scope: { kind: 'super_admin', role: 'super_admin', tenant: acmeTenant },
  support: { active: true, reason: '排查任务失败', entered_at: null },
}
const tenantAdminSession: PortalSessionPayload = {
  user: { id: 2, username: 'ops-admin', role: 'tenant_admin', must_change_password: false },
  scope: { kind: 'tenant', role: 'tenant_admin', tenant: acmeTenant },
}
const memberSession: PortalSessionPayload = {
  user: { id: 3, username: 'chen', role: 'member', must_change_password: false },
  scope: { kind: 'tenant', role: 'member', tenant: acmeTenant },
}

function workspaceValue(session: PortalSessionPayload | null): WorkspaceSessionValue {
  return session ? workspaceRoleFromSession(session) : { session: null, role: 'member', superAdmin: false }
}

function LoginLanding() {
  const location = useLocation()
  const navigationType = useNavigationType()
  return <div data-testid="login-landing">{`${location.pathname}|${navigationType}`}</div>
}

function renderAccountMenu(session: PortalSessionPayload | null, collapsed = false) {
  return render(
    <MemoryRouter initialEntries={['/']}>
      <Routes>
        <Route
          path="/"
          element={
            <WorkspaceSessionProvider value={workspaceValue(session)}>
              <div className={collapsed ? 'control-room control-room--collapsed' : 'control-room'}>
                <SidebarAccountMenu collapsed={collapsed} />
              </div>
            </WorkspaceSessionProvider>
          }
        />
        <Route path="/portal/login" element={<LoginLanding />} />
      </Routes>
    </MemoryRouter>,
  )
}

function openAccountMenu(session: PortalSessionPayload) {
  renderAccountMenu(session)
  const trigger = screen.getByRole('button', { name: `账号菜单，${session.user.username}` })
  fireEvent.click(trigger)
  return trigger
}

describe('SidebarAccountMenu', () => {
  it('shows the username and identity for every shell role', () => {
    const cases: Array<[PortalSessionPayload, string]> = [
      [superSession, '系统管理员'],
      [tenantAdminSession, '组织管理员 · Acme 组织'],
      [memberSession, '成员 · Acme 组织'],
    ]
    for (const [session, identityLine] of cases) {
      const { unmount } = renderAccountMenu(session)
      expect(screen.getByRole('button', { name: `账号菜单，${session.user.username}` })).toBeInTheDocument()
      expect(screen.getByText(session.user.username)).toBeInTheDocument()
      expect(screen.getByText(identityLine)).toBeInTheDocument()
      unmount()
    }
  })

  it('marks support access explicitly instead of showing a plain tenant admin', () => {
    renderAccountMenu(supportSession)
    expect(screen.getByRole('button', { name: '账号菜单，root' })).toBeInTheDocument()
    expect(screen.getByText(/系统管理员 · 支持访问 · Acme 组织/)).toBeInTheDocument()
    // 不能误显示成普通组织管理员
    expect(screen.queryByText('组织管理员 · Acme 组织')).not.toBeInTheDocument()
  })

  it('falls back to a placeholder when the shell renders without session details', () => {
    renderAccountMenu(null)
    expect(screen.getByRole('button', { name: '账号菜单，账号' })).toBeInTheDocument()
  })

  it('opens the menu with account info and logout on top of the sidebar', async () => {
    openAccountMenu(tenantAdminSession)
    const menu = await screen.findByRole('menu')
    expect(menu).toBeInTheDocument()
    expect(screen.getByTestId('account-menu-panel')).toHaveTextContent('ops-admin')
    expect(screen.getByTestId('account-menu-panel')).toHaveTextContent('组织管理员')
    expect(screen.getByRole('menuitem', { name: /退出登录/ })).toBeInTheDocument()
    // 菜单挂在 body 上，不被侧栏边界或导航容器裁剪
    expect(menu.closest('.control-sidebar')).toBeNull()
    expect(menu.closest('body')).not.toBeNull()
  })

  it('opens the logout menu only after clicking the account entry', async () => {
    const user = userEvent.setup()
    renderAccountMenu(memberSession)
    const trigger = screen.getByRole('button', { name: '账号菜单，chen' })
    await user.hover(trigger)
    await act(async () => { await new Promise((resolve) => setTimeout(resolve, 250)) })
    expect(screen.queryByRole('menuitem', { name: /退出登录/ })).not.toBeInTheDocument()

    await user.click(trigger)
    expect(await screen.findByRole('menuitem', { name: /退出登录/ })).toBeInTheDocument()
  })

  it('opens with Enter and closes with Escape returning focus to the trigger', async () => {
    const user = userEvent.setup()
    renderAccountMenu(memberSession)
    const trigger = screen.getByRole('button', { name: '账号菜单，chen' })
    trigger.focus()
    await user.keyboard('{Enter}')
    expect(await screen.findByRole('menu')).toBeInTheDocument()

    fireEvent.keyDown(document, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
    expect(trigger).toHaveFocus()
  })

  it('keeps a working accessible entry when the sidebar is collapsed', async () => {
    renderAccountMenu(superSession, true)
    const trigger = screen.getByRole('button', { name: '账号菜单，root' })
    // 折叠态隐藏用户名/身份文字，但可访问名称仍在
    expect(trigger).toHaveTextContent('root')
    expect(getComputedStyle(trigger.querySelector('.sidebar-account__meta') as HTMLElement).display).toBe('none')

    fireEvent.click(trigger)
    expect(await screen.findByRole('menuitem', { name: /退出登录/ })).toBeInTheDocument()
  })

  it('logs out through a confirm dialog and replaces onto the login route', async () => {
    const logout = vi.spyOn(portalApi, 'logout').mockResolvedValue({ ok: true })
    openAccountMenu(tenantAdminSession)
    fireEvent.click(await screen.findByRole('menuitem', { name: /退出登录/ }))

    // 必须先经确认弹窗：点当前账号选项前不得调用登出 API
    expect(await screen.findByTestId('logout-current')).toBeInTheDocument()
    expect(logout).not.toHaveBeenCalled()

    fireEvent.click(screen.getByTestId('logout-current'))
    expect(await screen.findByTestId('login-landing')).toHaveTextContent('/portal/login|REPLACE')
    expect(logout).toHaveBeenCalledTimes(1)
  })

  it('treats a 401 as already logged out and goes to the login route', async () => {
    vi.spyOn(portalApi, 'logout').mockRejectedValue(new PortalApiError(401, 'unauthenticated', '请先登录'))
    openAccountMenu(memberSession)
    fireEvent.click(await screen.findByRole('menuitem', { name: /退出登录/ }))
    fireEvent.click(await screen.findByTestId('logout-current'))

    expect(await screen.findByTestId('login-landing')).toHaveTextContent('/portal/login')
  })

  it('shows an error and allows a retry when the server fails to revoke', async () => {
    const logout = vi
      .spyOn(portalApi, 'logout')
      .mockRejectedValueOnce(new PortalApiError(500, 'internal_error', '服务暂不可用'))
      .mockResolvedValueOnce({ ok: true })
    openAccountMenu(memberSession)
    fireEvent.click(await screen.findByRole('menuitem', { name: /退出登录/ }))
    fireEvent.click(await screen.findByTestId('logout-current'))

    // 静态文案，不回显 token/堆栈等细节；弹窗保持打开供原位重试
    expect(await screen.findByText('退出登录失败，请稍后重试')).toBeInTheDocument()
    expect(screen.queryByTestId('login-landing')).not.toBeInTheDocument()

    fireEvent.click(screen.getByTestId('logout-current'))
    expect(await screen.findByTestId('login-landing')).toHaveTextContent('/portal/login')
    expect(logout).toHaveBeenCalledTimes(2)
  })

  it('prevents duplicate submits while a logout request is in flight', async () => {
    const logout = vi.spyOn(portalApi, 'logout').mockImplementation(() => new Promise(() => {}))
    openAccountMenu(superSession)
    fireEvent.click(await screen.findByRole('menuitem', { name: /退出登录/ }))
    fireEvent.click(await screen.findByTestId('logout-current'))

    // 请求进行中：当前账号选项禁用，重复点击不追加请求
    await waitFor(() => expect(screen.getByTestId('logout-current')).toBeDisabled())
    fireEvent.click(screen.getByTestId('logout-current'))
    expect(logout).toHaveBeenCalledTimes(1)
  })

  it('shows all three account entries with logout last', async () => {
    // 预置一个其他账号，让切换入口出现
    window.localStorage.setItem('coordinator_portal_accounts', JSON.stringify([
      { token: 'token-other', userId: 99, username: 'alice', savedAt: '2026-10-08T00:00:00.000Z' },
    ]))
    openAccountMenu(tenantAdminSession)
    await screen.findByRole('menu')
    const labels = Array.from(document.querySelectorAll('[role="menuitem"]'))
      .filter((el) => el.getAttribute('aria-disabled') !== 'true')
      .map((el) => el.textContent)
    expect(labels).toEqual(['修改密码', '切换账号', '退出登录'])
  })

  it('hides the switch entry when no other account is saved', async () => {
    openAccountMenu(tenantAdminSession)
    await screen.findByRole('menu')
    expect(screen.queryByRole('menuitem', { name: /切换账号/ })).not.toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /修改密码/ })).toBeInTheDocument()
  })

  it('revokes all saved sessions through the logout-all option', async () => {
    window.localStorage.setItem('coordinator_portal_accounts', JSON.stringify([
      { token: 'token-other', userId: 99, username: 'alice', savedAt: '2026-10-08T00:00:00.000Z' },
    ]))
    const logout = vi.spyOn(portalApi, 'logout').mockResolvedValue({ ok: true })
    const revoke = vi.spyOn(portalApi, 'revokeSavedSessions').mockResolvedValue({ ok: true })
    openAccountMenu(tenantAdminSession)
    fireEvent.click(await screen.findByRole('menuitem', { name: /退出登录/ }))

    fireEvent.click(await screen.findByTestId('logout-all'))
    expect(await screen.findByTestId('login-landing')).toBeInTheDocument()
    expect(revoke).toHaveBeenCalledWith(['token-other'])
    expect(logout).not.toHaveBeenCalled()
    expect(listSavedAccounts()).toHaveLength(0)
  })

  it('switches to a saved account via the switch dialog', async () => {
    window.localStorage.setItem('coordinator_portal_accounts', JSON.stringify([
      { token: 'token-other', userId: 99, username: 'alice', savedAt: '2026-10-08T00:00:00.000Z' },
    ]))
    const switchAccount = vi.spyOn(portalApi, 'switchAccount').mockResolvedValue({ ok: true })
    const assign = vi.fn()
    vi.stubGlobal('location', { assign })
    openAccountMenu(tenantAdminSession)
    fireEvent.click(await screen.findByRole('menuitem', { name: /切换账号/ }))

    fireEvent.click(await screen.findByTestId('switch-target-alice'))
    await waitFor(() => expect(switchAccount).toHaveBeenCalledWith('token-other'))
    await waitFor(() => expect(assign).toHaveBeenCalled())
    vi.unstubAllGlobals()
  })

  it('removes a stale saved account when its switch is rejected as expired', async () => {
    window.localStorage.setItem('coordinator_portal_accounts', JSON.stringify([
      { token: 'token-stale', userId: 99, username: 'alice', savedAt: '2026-10-08T00:00:00.000Z' },
    ]))
    vi.spyOn(portalApi, 'switchAccount').mockRejectedValue(new PortalApiError(401, 'invalid_token', '会话无效'))
    const assign = vi.fn()
    vi.stubGlobal('location', { href: '', assign })
    openAccountMenu(tenantAdminSession)
    fireEvent.click(await screen.findByRole('menuitem', { name: /切换账号/ }))

    fireEvent.click(await screen.findByTestId('switch-target-alice'))
    await waitFor(() => expect(listSavedAccounts()).toHaveLength(0))
    expect(assign).not.toHaveBeenCalled()
    vi.unstubAllGlobals()
  })

  it('opens the change-password dialog and submits a valid new password', async () => {
    const change = vi.spyOn(portalApi, 'changePassword').mockResolvedValue({ ok: true })
    openAccountMenu(tenantAdminSession)
    fireEvent.click(await screen.findByRole('menuitem', { name: /修改密码/ }))

    await screen.findByText('当前密码')
    fireEvent.change(screen.getByLabelText('当前密码'), { target: { value: 'old-password-1' } })
    fireEvent.change(screen.getByLabelText('新密码'), { target: { value: 'new-password-1' } })
    fireEvent.change(screen.getByLabelText('确认新密码'), { target: { value: 'new-password-1' } })
    fireEvent.click(screen.getByRole('button', { name: '确认修改' }))

    await waitFor(() => expect(change).toHaveBeenCalledWith('old-password-1', 'new-password-1'))
    expect(await screen.findByTestId('login-landing')).toBeInTheDocument()
    expect(listSavedAccounts()).toHaveLength(0)
  })

  it('rejects mismatched password confirmation inside the dialog', async () => {
    const change = vi.spyOn(portalApi, 'changePassword').mockResolvedValue({ ok: true })
    openAccountMenu(tenantAdminSession)
    fireEvent.click(await screen.findByRole('menuitem', { name: /修改密码/ }))

    await screen.findByText('当前密码')
    fireEvent.change(screen.getByLabelText('当前密码'), { target: { value: 'old-password-1' } })
    fireEvent.change(screen.getByLabelText('新密码'), { target: { value: 'new-password-1' } })
    fireEvent.change(screen.getByLabelText('确认新密码'), { target: { value: 'different-pass' } })
    fireEvent.click(screen.getByRole('button', { name: '确认修改' }))

    expect(await screen.findByText('两次输入的新密码不一致')).toBeInTheDocument()
    expect(change).not.toHaveBeenCalled()
  })
})
