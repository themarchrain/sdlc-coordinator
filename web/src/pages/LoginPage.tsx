import { LockOutlined } from '@ant-design/icons'
import { Alert, Button, Form, Input, message } from 'antd'
import { useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'

import { adminApi } from '../api/client'

interface LoginValues {
  username: string
  password: string
}

function safeReturnTo(value: string | null): string {
  if (value && value.startsWith('/') && !value.startsWith('//')) return value
  return '/'
}

export function LoginPage() {
  const navigate = useNavigate()
  const location = useLocation()
  const [submitting, setSubmitting] = useState(false)

  const submit = async (values: LoginValues) => {
    setSubmitting(true)
    try {
      await adminApi.login(values)
      navigate(safeReturnTo(new URLSearchParams(location.search).get('return_to')), { replace: true })
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '登录失败，请稍后重试')
    } finally {
      setSubmitting(false)
    }
  }

  return <main className="login-page">
    <section className="login-page__shell">
      <aside className="login-page__showcase">
        <div>
          <div className="login-page__brand">
            <div className="login-page__signal" aria-hidden="true"><span /><span /><span /></div>
            <div>
              <strong>Coordinator 控制面</strong>
              <span>COORDINATOR · CONTROL PLANE</span>
            </div>
          </div>
          <div className="login-page__showcase-copy">
            <span>研发自动化工作台</span>
            <h2>任务、执行与反馈，<br />保持同一份状态。</h2>
            <p>在一个管理入口查看任务运行、Agent 执行和状态源同步记录。</p>
          </div>
          <div className="login-page__preview" aria-hidden="true">
            <div className="login-page__preview-header"><i /><span>当前运行快照</span><b /></div>
            <div className="login-page__preview-row"><span>需求同步</span><em>已接管</em></div>
            <div className="login-page__preview-row"><span>开发任务</span><strong /></div>
            <div className="login-page__preview-row"><span>MR 审查</span><em>等待确认</em></div>
          </div>
        </div>
        <small className="login-page__showcase-foot">状态由 Coordinator 后端统一推进</small>
      </aside>
      <section className="login-page__card" aria-label="管理台登录">
        <div className="login-page__card-heading">
          <span>管理员访问</span>
          <h1>登录管理台</h1>
          <p>使用已配置的管理员账号继续。</p>
        </div>
        <Alert className="login-page__notice" type="info" showIcon message="首次启动默认账号：admin / admin123" />
        <Form<LoginValues> layout="vertical" requiredMark={false} onFinish={(values) => void submit(values)}>
          <Form.Item label="账号" name="username" rules={[{ required: true, message: '请输入账号' }]}>
            <Input autoFocus autoComplete="username" placeholder="请输入账号" />
          </Form.Item>
          <Form.Item label="密码" name="password" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password prefix={<LockOutlined />} autoComplete="current-password" placeholder="请输入密码" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={submitting}>登录管理台</Button>
        </Form>
      </section>
    </section>
  </main>
}
