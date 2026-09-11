import { Table, Tag } from 'antd'
import { useEffect, useMemo, useState } from 'react'

import { portalApi, type PortalTenantSummary } from '../api/portal'
import { PageHeading } from '../components/PageHeading'
import { EmptyState, ErrorState, LoadingState } from '../components/PageState'

function formatDateTime(value: string | null | undefined): string {
  if (!value) return '—'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

// SuperAdminOverviewPage 是超管侧的系统总览首页：组织规模两张统计卡 +
// 全量组织列表。超管不属于任何组织，这里只消费系统级数据，不触碰组织业务视图。
export function SuperAdminOverviewPage() {
  const [tenants, setTenants] = useState<PortalTenantSummary[] | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<unknown>()
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(undefined)
    portalApi.superListTenants()
      .then((payload) => { if (!cancelled) setTenants(payload.tenants) })
      .catch((reason) => { if (!cancelled) setError(reason) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [reloadKey])

  const memberTotal = useMemo(
    () => (tenants ?? []).reduce((sum, item) => sum + item.member_count, 0),
    [tenants],
  )

  return (
    <div className="org-page">
      <PageHeading eyebrow="超管工作台" title="系统总览" />
      {loading ? <LoadingState /> : error ? (
        <ErrorState error={error} onRetry={() => setReloadKey((key) => key + 1)} />
      ) : (
        <>
          <section className="surface-panel">
            <div className="grid grid-cols-2 gap-6">
              <div>
                <div className="text-sm text-[var(--colorTextSecondary,#666)]">组织总数</div>
                <div className="mt-1 text-3xl font-semibold">{tenants?.length ?? 0}</div>
              </div>
              <div>
                <div className="text-sm text-[var(--colorTextSecondary,#666)]">成员总数</div>
                <div className="mt-1 text-3xl font-semibold">{memberTotal}</div>
              </div>
            </div>
          </section>
          {tenants && tenants.length > 0 ? (
            <section className="surface-panel">
              <h3>组织列表</h3>
              <Table<PortalTenantSummary>
                rowKey="id"
                size="small"
                dataSource={tenants}
                pagination={false}
                columns={[
                  { title: '组织名称', dataIndex: 'display_name' },
                  { title: '组织标识', dataIndex: 'slug' },
                  { title: '成员数', dataIndex: 'member_count', width: 90 },
                  {
                    title: '状态',
                    dataIndex: 'status',
                    width: 100,
                    render: (status: string) => (
                      <Tag color={status === 'active' ? 'green' : 'red'}>{status === 'active' ? '正常' : '已停用'}</Tag>
                    ),
                  },
                  { title: '创建时间', dataIndex: 'created_at', render: formatDateTime },
                ]}
              />
            </section>
          ) : (
            <section className="surface-panel"><EmptyState description="还没有组织" /></section>
          )}
        </>
      )}
    </div>
  )
}
