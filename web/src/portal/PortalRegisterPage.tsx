import { Button, Form, Input, message } from 'antd'
import { Link, useNavigate } from 'react-router-dom'
import { useState } from 'react'

import { portalApi } from '../api/portal'
import { saveCurrentAccount } from '../app/accountRegistry'
import { navigatePathForScope } from './PortalLoginPage'

interface RegisterValues {
  username: string
  password: string
  inviteCode: string
}

export function PortalRegisterPage() {
  const navigate = useNavigate()
  const [submitting, setSubmitting] = useState(false)

  const submit = async (values: RegisterValues) => {
    setSubmitting(true)
    try {
      const session = await portalApi.register(values.username, values.password, values.inviteCode)
      if (session.session_token) {
        saveCurrentAccount(session.user.id, session.user.username)
      }
      navigate(navigatePathForScope(session.scope, session.user.must_change_password), { replace: true })
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '注册失败，请稍后重试')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <main className="flex min-h-screen items-center justify-center bg-[var(--colorBgLayout,#f5f5f5)] px-4">
      <section className="w-full max-w-md rounded-lg bg-[var(--colorBgContainer,#fff)] p-8 shadow-sm">
        <div className="mb-6">
          <span className="text-xs tracking-widest text-[var(--colorTextTertiary,#999)]">多租户门户</span>
          <h1 className="mt-1 text-2xl font-semibold">注册新账号</h1>
          <p className="mt-1 text-sm text-[var(--colorTextSecondary,#666)]">注册需要组织管理员签发的邀请码。</p>
        </div>
        <Form<RegisterValues> layout="vertical" requiredMark={false} onFinish={(values) => void submit(values)}>
          <Form.Item
            label="用户名"
            name="username"
            rules={[
              { required: true, message: '请输入用户名' },
              { pattern: /^[a-z0-9_-]{3,32}$/, message: '3-32 位小写字母、数字、下划线或连字符' },
            ]}
          >
            <Input autoFocus autoComplete="username" placeholder="请输入用户名" />
          </Form.Item>
          <Form.Item
            label="密码"
            name="password"
            rules={[
              { required: true, message: '请输入密码' },
              { min: 8, message: '密码至少 8 位' },
            ]}
          >
            <Input.Password autoComplete="new-password" placeholder="请输入密码（至少 8 位）" />
          </Form.Item>
          <Form.Item label="邀请码" name="inviteCode" rules={[{ required: true, message: '请输入邀请码' }]}>
            <Input placeholder="形如 ABCD-EFGH-JKLM" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={submitting}>注册并加入组织</Button>
        </Form>
        <div className="mt-4 text-sm">
          <Link to="/portal/login">返回登录</Link>
        </div>
      </section>
    </main>
  )
}
