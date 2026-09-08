import { LockOutlined } from '@ant-design/icons'
import { Alert, Button, Form, Input, message } from 'antd'
import { Link, useNavigate } from 'react-router-dom'
import { useState } from 'react'

import { workspaceLandingPath } from '../app/workspaceSession'
import { portalApi } from '../api/portal'

interface LoginValues {
  username: string
  password: string
}

export function navigatePathForScope(scope: { kind: string }, mustChangePassword: boolean, supportActive = false): string {
  return workspaceLandingPath({
    user: { id: 0, username: '', role: '', must_change_password: mustChangePassword },
    scope: { kind: scope.kind as 'super_admin' | 'tenant' | 'unscoped' },
    support: supportActive ? { active: true, reason: '' } : undefined,
  })
}

export function PortalLoginPage() {
  const navigate = useNavigate()
  const [submitting, setSubmitting] = useState(false)

  const submit = async (values: LoginValues) => {
    setSubmitting(true)
    try {
      const session = await portalApi.login(values.username, values.password)
      navigate(navigatePathForScope(session.scope, session.user.must_change_password), { replace: true })
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '登录失败，请稍后重试')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <main className="flex min-h-screen items-center justify-center bg-[var(--colorBgLayout,#f5f5f5)] px-4">
      <section className="w-full max-w-md rounded-lg bg-[var(--colorBgContainer,#fff)] p-8 shadow-sm">
        <div className="mb-6">
          <span className="text-xs tracking-widest text-[var(--colorTextTertiary,#999)]">多组织门户</span>
          <h1 className="mt-1 text-2xl font-semibold">登录工作台</h1>
          <p className="mt-1 text-sm text-[var(--colorTextSecondary,#666)]">使用组织分配的账号继续。</p>
        </div>
        <Form<LoginValues> layout="vertical" requiredMark={false} onFinish={(values) => void submit(values)}>
          <Form.Item label="用户名" name="username" rules={[{ required: true, message: '请输入用户名' }]}>
            <Input autoFocus autoComplete="username" placeholder="请输入用户名" />
          </Form.Item>
          <Form.Item label="密码" name="password" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password prefix={<LockOutlined />} autoComplete="current-password" placeholder="请输入密码" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={submitting}>登录</Button>
        </Form>
        <div className="mt-4 flex items-center justify-between text-sm">
          <Link to="/portal/register">注册新账号</Link>
        </div>
        <Alert className="mt-4" type="info" showIcon message="注册需要组织管理员签发的邀请码。" />
      </section>
    </main>
  )
}
