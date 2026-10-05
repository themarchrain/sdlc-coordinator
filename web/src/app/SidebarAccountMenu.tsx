import { LogoutOutlined } from '@ant-design/icons'
import { Avatar, Dropdown, message } from 'antd'
import type { MenuProps } from 'antd'
import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { PortalApiError, portalApi } from '../api/portal'
import { useWorkspaceSession } from './workspaceSession'

// SidebarAccountMenu 是管理台外壳的统一账号入口：头像 + 用户名 + 当前身份，
// 菜单向上展开并提供退出登录。会话数据只来自 WorkspaceSessionProvider 注入的
// 可信会话；退出走现有门户 API，HttpOnly cookie 由服务端清除，前端不读 cookie。
export function SidebarAccountMenu({ collapsed = false }: { collapsed?: boolean }) {
  const { session, role, superAdmin } = useWorkspaceSession()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [loggingOut, setLoggingOut] = useState(false)
  const loggingOutRef = useRef(false)
  const triggerRef = useRef<HTMLButtonElement>(null)

  const username = session?.user.username ?? '账号'
  const supportActive = session?.support?.active === true
  const orgName = session?.scope.tenant?.display_name ?? null
  let identity = '成员'
  if (supportActive) {
    identity = '系统管理员 · 支持访问'
  } else if (superAdmin) {
    identity = '系统管理员'
  } else if (role === 'tenant_admin') {
    identity = '组织管理员'
  }

  // Escape 关闭菜单并把焦点还给触发按钮；antd 自身的 Esc 处理不保证焦点归还。
  useEffect(() => {
    if (!open) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      setOpen(false)
      triggerRef.current?.focus()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [open])

  const handleOpenChange = (next: boolean) => {
    // 退出请求进行中不收起菜单：保住进行中状态与失败后的原位重试。
    if (!next && loggingOutRef.current) return
    setOpen(next)
  }

  const handleLogout = async () => {
    if (loggingOutRef.current) return
    loggingOutRef.current = true
    setLoggingOut(true)
    try {
      await portalApi.logout()
      navigate('/portal/login', { replace: true })
    } catch (reason) {
      if (reason instanceof PortalApiError && reason.status === 401) {
        // 会话已失效或已撤销：按已退出处理，不把用户困在后台。
        navigate('/portal/login', { replace: true })
        return
      }
      // 静态文案：不回显 cookie、token、堆栈或数据库细节。
      message.error('退出登录失败，请稍后重试')
      loggingOutRef.current = false
      setLoggingOut(false)
    }
  }

  const items: MenuProps['items'] = [
    {
      key: 'account',
      disabled: true,
      label: (
        <div className="sidebar-account__panel" data-testid="account-menu-panel">
          <strong>{username}</strong>
          <span>{identity}</span>
          {orgName ? <span>{orgName}</span> : null}
        </div>
      ),
    },
    { type: 'divider' },
    {
      key: 'logout',
      danger: true,
      disabled: loggingOut,
      icon: <LogoutOutlined />,
      label: loggingOut ? '退出中…' : '退出登录',
    },
  ]

  return (
    <div className="sidebar-account">
      <Dropdown
        trigger={['click']}
        placement="topLeft"
        open={open}
        onOpenChange={handleOpenChange}
        getPopupContainer={() => document.body}
        menu={{
          items,
          onClick: ({ key }) => {
            if (key === 'logout') void handleLogout()
          },
        }}
      >
        <button
          ref={triggerRef}
          type="button"
          className="sidebar-account__trigger"
          aria-label={`账号菜单，${username}`}
          aria-haspopup="menu"
          aria-expanded={open}
          onClick={() => setOpen(true)}
        >
          <Avatar size={collapsed ? 24 : 28} className="sidebar-account__avatar">
            {username.trim().charAt(0).toUpperCase()}
          </Avatar>
          <span className="sidebar-account__meta">
            <strong>{username}</strong>
            <span>{orgName ? `${identity} · ${orgName}` : identity}</span>
          </span>
        </button>
      </Dropdown>
    </div>
  )
}
