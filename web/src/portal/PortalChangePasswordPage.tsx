import { Button, Card, Form, Input, message } from 'antd'
import { useNavigate } from 'react-router-dom'
import { useState } from 'react'

import { portalApi } from '../api/portal'

interface ChangePasswordValues {
  currentPassword: string
  newPassword: string
  confirmPassword: string
}

// PortalChangePasswordPage 承接首登强制改密与主动改密；控制面在改密成功后
// 吊销全部会话，因此完成后回到登录页。
export function PortalChangePasswordPage() {
  const navigate = useNavigate()
  const [submitting, setSubmitting] = useState(false)

  const submit = async (values: ChangePasswordValues) => {
    setSubmitting(true)
    try {
      await portalApi.changePassword(values.currentPassword, values.newPassword)
      message.success('密码已修改，请重新登录')
      navigate('/portal/login', { replace: true })
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '修改密码失败')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <main className="flex min-h-screen items-center justify-center bg-[var(--colorBgLayout,#f5f5f5)] px-4">
      <Card className="w-full max-w-md" title="修改密码">
        <p className="mb-4 text-sm text-[var(--colorTextSecondary,#666)]">
          出于安全要求，首次登录或密码被重置后需要先设置新密码。
        </p>
        <Form<ChangePasswordValues> layout="vertical" requiredMark={false} onFinish={(values) => void submit(values)}>
          <Form.Item label="当前密码" name="currentPassword" rules={[{ required: true, message: '请输入当前密码' }]}>
            <Input.Password autoComplete="current-password" placeholder="请输入当前密码" />
          </Form.Item>
          <Form.Item
            label="新密码"
            name="newPassword"
            rules={[
              { required: true, message: '请输入新密码' },
              { min: 8, message: '密码至少 8 位' },
            ]}
          >
            <Input.Password autoComplete="new-password" placeholder="请输入新密码（至少 8 位）" />
          </Form.Item>
          <Form.Item
            label="确认新密码"
            name="confirmPassword"
            dependencies={['newPassword']}
            rules={[
              { required: true, message: '请再次输入新密码' },
              ({ getFieldValue }) => ({
                validator(_, value) {
                  if (!value || getFieldValue('newPassword') === value) return Promise.resolve()
                  return Promise.reject(new Error('两次输入的新密码不一致'))
                },
              }),
            ]}
          >
            <Input.Password autoComplete="new-password" placeholder="请再次输入新密码" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={submitting}>修改密码</Button>
        </Form>
      </Card>
    </main>
  )
}
