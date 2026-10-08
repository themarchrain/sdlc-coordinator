import { KeyOutlined, LogoutOutlined, PlusOutlined, SwapOutlined } from '@ant-design/icons'
import { Avatar, Button, Dropdown, Form, Input, message, Modal, Spin } from 'antd'
import type { MenuProps } from 'antd'
import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { PortalApiError, portalApi } from '../api/portal'
import {
  clearSavedAccounts,
  listSavedAccounts,
  otherSavedAccounts,
  removeSavedAccount,
  saveCurrentAccount,
  type SavedAccount,
} from './accountRegistry'
import { useWorkspaceSession } from './workspaceSession'

// SidebarAccountMenu 是管理台外壳的统一账号入口：头像 + 用户名 + 当前身份，
// 菜单提供修改密码、切换账号（用户中心）与退出登录。会话数据只来自
// WorkspaceSessionProvider 注入的可信会话；HttpOnly cookie 由服务端写入与
// 清除，前端只在切换时把注册表内的 token 交给服务端校验后由服务端改写。
type AccountDialog = 'none' | 'logout' | 'switch' | 'password'

const MIN_PASSWORD_LENGTH = 8

export function SidebarAccountMenu({ collapsed = false }: { collapsed?: boolean }) {
  const { session, role, superAdmin } = useWorkspaceSession()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [loggingOut, setLoggingOut] = useState(false)
  const [dialog, setDialog] = useState<AccountDialog>('none')
  const [switchingTo, setSwitchingTo] = useState<number | null>(null)
  const [changeForm] = Form.useForm<{ current: string; next: string; confirm: string }>()
  const [loginForm] = Form.useForm<{ username: string; password: string }>()
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

  const gotoLogin = () => {
    navigate('/portal/login', { replace: true })
  }

  const handleLogout = async () => {
    if (loggingOutRef.current) return
    loggingOutRef.current = true
    setLoggingOut(true)
    try {
      await portalApi.logout()
      if (session?.user.id != null) removeSavedAccount(session.user.id)
      setDialog('none')
      gotoLogin()
    } catch (reason) {
      loggingOutRef.current = false
      setLoggingOut(false)
      if (reason instanceof PortalApiError && reason.status === 401) {
        // 会话已失效或已撤销：按已退出处理，不把用户困在后台。
        setDialog('none')
        gotoLogin()
        return
      }
      // 静态文案：不回显 cookie、token、堆栈或数据库细节。弹窗保持打开供重试。
      message.error('退出登录失败，请稍后重试')
    }
  }

  // handleRevokeAll 退出所有已保存账号：批量撤销服务端会话并清空注册表。
  const handleRevokeAll = async () => {
    if (loggingOutRef.current) return
    loggingOutRef.current = true
    setLoggingOut(true)
    try {
      const tokens = listSavedAccounts().map((item) => item.token)
      await portalApi.revokeSavedSessions(tokens)
      clearSavedAccounts()
      setDialog('none')
      gotoLogin()
    } catch (reason) {
      loggingOutRef.current = false
      setLoggingOut(false)
      if (reason instanceof PortalApiError && reason.status === 401) {
        clearSavedAccounts()
        setDialog('none')
        gotoLogin()
        return
      }
      message.error('退出全部账号失败，请稍后重试')
    }
  }

  // handleSwitch 把会话切换到注册表内的另一账号：服务端校验 token 有效后
  // 改写会话 cookie；整页重载让全部会话上下文按新账号重新装载。
  const handleSwitch = async (account: SavedAccount) => {
    if (switchingTo !== null) return
    setSwitchingTo(account.userId)
    try {
      await portalApi.switchAccount(account.token)
      window.location.assign('/admin/')
    } catch (reason) {
      setSwitchingTo(null)
      if (reason instanceof PortalApiError && reason.status === 401) {
        // 该账号的已保存会话已失效（过期/被他处撤销）：剔除后让用户重试。
        removeSavedAccount(account.userId)
        message.warning('该账号的登录状态已失效，已从列表移除')
        return
      }
      message.error('切换账号失败，请稍后重试')
    }
  }

  // handleChangePassword 走既有改密接口；服务端成功即吊销该用户全部会话
  // （包括注册表内同账号的旧 token），因此清空注册表并回到登录页。
  const handleChangePassword = async () => {
    try {
      const values = await changeForm.validateFields()
      setSwitchingTo(-1)
      await portalApi.changePassword(values.current, values.next)
      clearSavedAccounts()
      message.success('密码已修改，请使用新密码重新登录')
      gotoLogin()
    } catch (reason) {
      if (reason && typeof reason === 'object' && 'errorFields' in reason) {
        return // 表单校验错误由输入框内文案呈现。
      }
      if (reason instanceof PortalApiError) {
        if (reason.status === 401) {
          message.error('登录状态已失效，请重新登录')
          clearSavedAccounts()
          gotoLogin()
          return
        }
        message.error(reason.message || '修改密码失败，请稍后重试')
        return
      }
      message.error('修改密码失败，请稍后重试')
    } finally {
      setSwitchingTo(null)
    }
  }

  // handleLoginAnother 在弹窗内直接登录另一个账号：当前会话的 token 已登记在
  // 注册表里（登录即登记），登录成功后 cookie 换写为新账号、整页重载，原账号
  // 留在列表中可随时切回。401 = 用户名或密码错误，在表单内呈现。
  const handleLoginAnother = async (rawUsername: string, password: string) => {
    if (switchingTo !== null) return
    setSwitchingTo(-2)
    try {
      const result = await portalApi.login(rawUsername.trim(), password)
      if (result.session_token) {
        saveCurrentAccount(result.user.id, result.user.username, result.session_token)
      }
      window.location.assign('/admin/')
    } catch (reason) {
      setSwitchingTo(null)
      if (reason instanceof PortalApiError) {
        if (reason.status === 401) {
          loginForm.setFields([{ name: 'password', errors: ['用户名或密码错误'] }])
          return
        }
        message.error(reason.message || '登录失败，请稍后重试')
        return
      }
      message.error('登录失败，请稍后重试')
    }
  }

  const closeDialog = () => {
    // 切换/退出请求进行中不关弹窗：防中途取消造成状态悬空。
    if (switchingTo !== null && switchingTo !== -1) return
    if (loggingOutRef.current) return
    setDialog('none')
    changeForm.resetFields()
    loginForm.resetFields()
  }

  const savedCount = listSavedAccounts().length
  const others: SavedAccount[] = dialog === 'switch' ? otherSavedAccounts(session?.user.id ?? null) : []
  const hasOtherAccounts = otherSavedAccounts(session?.user.id ?? null).length > 0

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
      key: 'password',
      icon: <KeyOutlined />,
      label: '修改密码',
    },
    {
      key: 'switch',
      icon: <SwapOutlined />,
      label: '切换账号',
    },
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
            if (key === 'logout') {
              setDialog('logout')
            } else if (key === 'switch') {
              loginForm.resetFields()
              setDialog('switch')
            } else if (key === 'password') {
              changeForm.resetFields()
              setDialog('password')
            }
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
          {/* 头像底/字走 primary/on-primary 配对：antd 默认灰底白字在多数浅色主题下不可读 */}
          <Avatar
            size={collapsed ? 24 : 28}
            className="sidebar-account__avatar"
            style={{ backgroundColor: 'var(--primary)', color: 'var(--on-primary)' }}
          >
            {username.trim().charAt(0).toUpperCase()}
          </Avatar>
          <span className="sidebar-account__meta">
            <strong>{username}</strong>
            <span>{orgName ? `${identity} · ${orgName}` : identity}</span>
          </span>
        </button>
      </Dropdown>

      <Modal
        title="退出登录"
        open={dialog === 'logout'}
        onCancel={closeDialog}
        footer={null}
        width={420}
        destroyOnClose
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          <span>选择退出范围：</span>
          <button
            type="button"
            data-testid="logout-current"
            disabled={loggingOut}
            className="sidebar-account__logout-option"
            onClick={() => void handleLogout()}
          >
            退出登录当前账号
          </button>
          <button
            type="button"
            data-testid="logout-all"
            disabled={loggingOut || savedCount === 0 || !hasOtherAccounts}
            className="sidebar-account__logout-option"
            onClick={() => void handleRevokeAll()}
          >
            {savedCount > 0 ? `退出所有已保存的账号（${savedCount} 个）` : '没有其他已保存账号'}
          </button>
        </div>
      </Modal>

      <Modal
        title="切换账号"
        open={dialog === 'switch'}
        onCancel={closeDialog}
        footer={null}
        width={420}
        destroyOnClose
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          {others.length === 0 ? (
            <span style={{ color: 'var(--mute)', fontSize: 12 }}>还没有其他已保存的登录账号。</span>
          ) : (
            others.map((account) => (
              <button
                key={account.userId}
                type="button"
                data-testid={`switch-target-${account.username}`}
                className="sidebar-account__switch-option"
                disabled={switchingTo !== null}
                onClick={() => void handleSwitch(account)}
              >
                {switchingTo === account.userId ? <Spin size="small" /> : <Avatar size={24}>{account.username.charAt(0).toUpperCase()}</Avatar>}
                <span style={{ fontWeight: 600 }}>{account.username}</span>
                <span style={{ color: 'var(--mute)', fontSize: 11 }}>切换后需重新加载页面</span>
              </button>
            ))
          )}
          <div className="sidebar-account__login-inline">
            <span style={{ color: 'var(--mute)', fontSize: 12 }}>登录其他账号（当前登录状态保持不变）</span>
            <Form form={loginForm} layout="vertical" requiredMark={false} onFinish={(values) => void handleLoginAnother(values.username, values.password)}>
              <Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
                <Input placeholder="用户名" autoComplete="username" />
              </Form.Item>
              <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
                <Input.Password placeholder="密码" autoComplete="current-password" />
              </Form.Item>
              <Button
                type="primary"
                block
                data-testid="switch-login-submit"
                htmlType="submit"
                loading={switchingTo === -2}
              >
                登录并加入列表
              </Button>
            </Form>
          </div>
        </div>
      </Modal>

      <Modal
        title="修改密码"
        open={dialog === 'password'}
        onCancel={closeDialog}
        okText="确认修改"
        okButtonProps={{ loading: switchingTo === -1 }}
        onOk={() => void handleChangePassword()}
        destroyOnClose
        width={420}
      >
        <Form form={changeForm} layout="vertical" requiredMark={false}>
          <Form.Item
            name="current"
            label="当前密码"
            rules={[{ required: true, message: '请输入当前密码' }]}
          >
            <Input.Password autoComplete="current-password" />
          </Form.Item>
          <Form.Item
            name="next"
            label="新密码"
            rules={[
              { required: true, message: '请输入新密码' },
              { min: MIN_PASSWORD_LENGTH, message: `新密码至少 ${MIN_PASSWORD_LENGTH} 个字符` },
            ]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Form.Item
            name="confirm"
            label="确认新密码"
            dependencies={['next']}
            rules={[
              { required: true, message: '请再次输入新密码' },
              ({ getFieldValue }) => ({
                validator(_, value) {
                  if (!value || getFieldValue('next') === value) return Promise.resolve()
                  return Promise.reject(new Error('两次输入的新密码不一致'))
                },
              }),
            ]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

