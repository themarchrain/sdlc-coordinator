import { Card, Input, Table, Tag } from 'antd'
import { useCallback, useEffect, useState } from 'react'

import { portalApi, type PortalAuditItem, type PortalAuditPage } from '../api/portal'

const actionOptions = [
  { value: '', label: '全部动作' },
  { value: 'support_access.entered', label: '支持访问进入' },
  { value: 'support_access.exited', label: '支持访问退出' },
  { value: 'tenant.created', label: '组织创建' },
  { value: 'tenant.updated', label: '组织更新' },
  { value: 'tenant.disabled', label: '组织停用' },
  { value: 'tenant.restored', label: '组织恢复' },
  { value: 'tenant_admin.provisioned', label: '管理员开通' },
  { value: 'password.reset', label: '密码重置' },
  { value: 'member.added', label: '成员加入' },
  { value: 'member.removed', label: '成员移除' },
  { value: 'invite.issued', label: '邀请码签发' },
  { value: 'invite.revoked', label: '邀请码吊销' },
  { value: 'runner_assignments.updated', label: 'Runner 指派更新' },
]

function formatDateTime(value: string): string {
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

// AuditLogPage 是超管后台的审计查看页（全量保留、分页浏览）。
export function AuditLogPage() {
  const [page, setPage] = useState<PortalAuditPage | null>(null)
  const [current, setCurrent] = useState(1)
  const [action, setAction] = useState('')
  const [tenantId, setTenantId] = useState('')

  const load = useCallback(async (targetPage: number, targetAction: string, targetTenant: string) => {
    const payload = await portalApi.superListAuditLog({
      action: targetAction || undefined,
      tenantId: targetTenant ? Number(targetTenant) : undefined,
      page: targetPage,
      pageSize: 20,
    })
    setPage(payload)
  }, [])

  useEffect(() => {
    load(current, action, tenantId).catch(() => {
      // 错误已由 request 层抛出；这里保持页面静默（列表为空）
      setPage({ items: [], total: 0, page: current, page_size: 20 })
    })
  }, [load, current, action, tenantId])

  return (
    <Card title="审计日志">
      <div className="mb-3 flex gap-2">
        <Input
          placeholder="按组织 ID 过滤"
          style={{ width: 160 }}
          value={tenantId}
          onChange={(event) => {
            setCurrent(1)
            setTenantId(event.target.value.replace(/[^0-9]/g, ''))
          }}
        />
        <select
          aria-label="审计动作过滤"
          value={action}
          onChange={(event) => {
            setCurrent(1)
            setAction(event.target.value)
          }}
          className="rounded border px-2 py-1"
        >
          {actionOptions.map((option) => (
            <option key={option.value} value={option.value}>{option.label}</option>
          ))}
        </select>
      </div>
      <Table<PortalAuditItem>
        rowKey="id"
        size="small"
        dataSource={page?.items ?? []}
        pagination={{
          current: page?.page ?? 1,
          pageSize: page?.page_size ?? 20,
          total: page?.total ?? 0,
          onChange: (next) => setCurrent(next),
          showSizeChanger: false,
        }}
        columns={[
          { title: '时间', dataIndex: 'created_at', width: 170, render: formatDateTime },
          { title: '操作者', dataIndex: 'actor_username', width: 120 },
          { title: '来源 IP', dataIndex: 'source_ip', width: 140, render: (value: string | undefined) => value || '—' },
          { title: '组织', dataIndex: 'tenant_id', width: 80, render: (value: number | null) => value ?? '—' },
          {
            title: '动作',
            dataIndex: 'action',
            width: 200,
            render: (value: string) => <Tag>{value}</Tag>,
          },
          { title: '详情', dataIndex: 'detail' },
        ]}
      />
    </Card>
  )
}
