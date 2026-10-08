import { Button, Card, Spin, Tag, message } from 'antd'
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { portalApi, type PortalSessionPayload, type PortalTenantOption } from '../api/portal'
import { saveCurrentAccount } from '../app/accountRegistry'

// SelectTenantPage 是一人多组织登录后的组织选择页；选择即
// 换发会话作用域。
export function SelectTenantPage() {
  const navigate = useNavigate()
  const [session, setSession] = useState<PortalSessionPayload | null>(null)
  const [loading, setLoading] = useState(true)
  const [selecting, setSelecting] = useState<number | null>(null)

  useEffect(() => {
    let cancelled = false
    portalApi.session()
      .then((payload) => {
        if (cancelled) return
        setSession(payload)
        setLoading(false)
      })
      .catch(() => {
        if (cancelled) return
        navigate('/portal/login', { replace: true })
      })
    return () => { cancelled = true }
  }, [navigate])

  const choose = async (option: PortalTenantOption) => {
    setSelecting(option.id)
    try {
      const session = await portalApi.selectTenant(option.id)
      if (session.session_token) {
        saveCurrentAccount(session.user.id, session.user.username, session.session_token)
      }
      navigate('/', { replace: true })
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '选择组织失败')
      setSelecting(null)
    }
  }

  if (loading) {
    return <main className="flex min-h-screen items-center justify-center"><Spin /></main>
  }
  const tenants = session?.tenants ?? []
  return (
    <main className="flex min-h-screen items-center justify-center bg-[var(--colorBgLayout,#f5f5f5)] px-4">
      <Card className="w-full max-w-lg" title="选择要进入的组织" data-testid="select-tenant-card">
        <p className="mb-4 text-sm text-[var(--colorTextSecondary,#666)]">你的账号加入了多个组织，请选择本次要使用的组织。</p>
        <div className="flex flex-col gap-2">
          {tenants.map((option) => (
            <Button
              key={option.id}
              block
              className="flex items-center justify-between text-left"
              loading={selecting === option.id}
              onClick={() => void choose(option)}
            >
              <span className="font-medium">{option.display_name}</span>
              <Tag color={option.role === 'tenant_admin' ? 'blue' : 'default'}>
                {option.role === 'tenant_admin' ? '组织管理员' : '成员'}
              </Tag>
            </Button>
          ))}
          {tenants.length === 0 && (
            <p className="text-sm text-[var(--colorTextSecondary,#666)]">暂无可进入的组织，请联系组织管理员。</p>
          )}
        </div>
        <div className="mt-4">
          <Button type="link" onClick={() => void portalApi.logout().then(() => navigate('/portal/login'))}>退出登录</Button>
        </div>
      </Card>
    </main>
  )
}
