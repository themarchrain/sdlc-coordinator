import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation, useNavigationType } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { PortalApiError, portalApi, type PortalSessionPayload } from '../api/portal'
import { SidebarAccountMenu } from './SidebarAccountMenu'
import { WorkspaceSessionProvider, workspaceRoleFromSession, type WorkspaceSessionValue } from './workspaceSession'
import '../styles/index.css'

afterEach(() => vi.restoreAllMocks())

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

  it('logs out once through the portal api and replaces onto the login route', async () => {
    const logout = vi.spyOn(portalApi, 'logout').mockResolvedValue({ ok: true })
    openAccountMenu(tenantAdminSession)
    fireEvent.click(await screen.findByRole('menuitem', { name: /退出登录/ }))

    expect(await screen.findByTestId('login-landing')).toHaveTextContent('/portal/login|REPLACE')
    expect(logout).toHaveBeenCalledTimes(1)
  })

  it('treats a 401 as already logged out and goes to the login route', async () => {
    vi.spyOn(portalApi, 'logout').mockRejectedValue(new PortalApiError(401, 'unauthenticated', '请先登录'))
    openAccountMenu(memberSession)
    fireEvent.click(await screen.findByRole('menuitem', { name: /退出登录/ }))

    expect(await screen.findByTestId('login-landing')).toHaveTextContent('/portal/login')
  })

  it('shows an error and allows a retry when the server fails to revoke', async () => {
    const logout = vi
      .spyOn(portalApi, 'logout')
      .mockRejectedValueOnce(new PortalApiError(500, 'internal_error', '服务暂不可用'))
      .mockResolvedValueOnce({ ok: true })
    openAccountMenu(memberSession)
    fireEvent.click(await screen.findByRole('menuitem', { name: /退出登录/ }))

    // 静态文案，不回显 token/堆栈等细节
    expect(await screen.findByText('退出登录失败，请稍后重试')).toBeInTheDocument()
    expect(screen.queryByTestId('login-landing')).not.toBeInTheDocument()

    fireEvent.click(await screen.findByRole('menuitem', { name: /退出登录/ }))
    expect(await screen.findByTestId('login-landing')).toHaveTextContent('/portal/login')
    expect(logout).toHaveBeenCalledTimes(2)
  })

  it('prevents duplicate submits while a logout request is in flight', async () => {
    const logout = vi.spyOn(portalApi, 'logout').mockImplementation(() => new Promise(() => {}))
    openAccountMenu(superSession)
    fireEvent.click(await screen.findByRole('menuitem', { name: /退出登录/ }))

    // 触发后进入进行中状态：菜单不收起，退出项转为禁用
    const item = await screen.findByRole('menuitem', { name: /退出中/ })
    expect(item).toHaveAttribute('aria-disabled', 'true')

    fireEvent.click(item)
    expect(logout).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('menuitem', { name: /退出中/ })).toBeInTheDocument()
  })
})
