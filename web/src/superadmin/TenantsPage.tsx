import { Button, Card, Form, Input, Modal, Popconfirm, Space, Table, Tag, message } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { portalApi, type PortalTenantSummary } from '../api/portal'

// TenantsPage 是超管后台的组织列表：生命周期管理（禁用/恢复，写审计）与
// 带审计支持访问入口。
export function TenantsPage() {
  const navigate = useNavigate()
  const [tenants, setTenants] = useState<PortalTenantSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [creating, setCreating] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [supportTenant, setSupportTenant] = useState<PortalTenantSummary | null>(null)
  const [createForm] = Form.useForm<{ slug: string; displayName: string }>()
  const [supportForm] = Form.useForm<{ reason: string }>()

  const reload = useCallback(async () => {
    const payload = await portalApi.superListTenants()
    setTenants(payload.tenants)
    setLoading(false)
  }, [])

  useEffect(() => {
    reload().catch((reason: unknown) => {
      message.error(reason instanceof Error ? reason.message : '加载组织列表失败')
      setLoading(false)
    })
  }, [reload])

  const create = async (values: { slug: string; displayName: string }) => {
    try {
      await portalApi.superCreateTenant(values.slug, values.displayName)
      message.success('组织已创建')
      setCreateOpen(false)
      createForm.resetFields()
      await reload()
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '创建组织失败')
    }
  }

  const toggleStatus = async (tenant: PortalTenantSummary) => {
    try {
      if (tenant.status === 'active') {
        await portalApi.superDisableTenant(tenant.id)
        message.success('组织已停用，其成员会话即时失效')
      } else {
        await portalApi.superRestoreTenant(tenant.id)
        message.success('组织已恢复')
      }
      await reload()
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '操作失败')
    }
  }

  const enterSupport = async (values: { reason: string }) => {
    if (!supportTenant) return
    try {
      await portalApi.superEnterSupportAccess(supportTenant.id, values.reason)
      message.success('已进入支持访问，所有动作已记入审计')
      setSupportTenant(null)
      supportForm.resetFields()
      navigate('/org')
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '进入支持访问失败')
    }
  }

  return (
    <Card
      title="组织管理"
      extra={<Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>新建组织</Button>}
    >
      <Table<PortalTenantSummary>
        rowKey="id"
        size="small"
        loading={loading}
        dataSource={tenants}
        pagination={false}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 60 },
          { title: '组织标识', dataIndex: 'slug' },
          { title: '组织名称', dataIndex: 'display_name' },
          {
            title: '状态',
            dataIndex: 'status',
            render: (status: string) => (
              <Tag color={status === 'active' ? 'green' : 'red'}>{status === 'active' ? '正常' : '已停用'}</Tag>
            ),
          },
          { title: '成员数', dataIndex: 'member_count', width: 90 },
          {
            title: '操作',
            key: 'actions',
            render: (_, record) => (
              <Space>
                <Button size="small" onClick={() => navigate(`/super-admin/tenants/${record.id}`)}>详情</Button>
                {record.status === 'active' ? (
                  <Popconfirm title="停用后该组织全部成员会话即时失效，确认停用？" onConfirm={() => void toggleStatus(record)}>
                    <Button size="small" danger>停用</Button>
                  </Popconfirm>
                ) : (
                  <Button size="small" onClick={() => void toggleStatus(record)}>恢复</Button>
                )}
                <Button size="small" type="primary" ghost onClick={() => setSupportTenant(record)}>支持访问</Button>
              </Space>
            ),
          },
        ]}
      />

      <Modal
        title="新建组织"
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={() => createForm.submit()}
        okText="创建"
        cancelText="取消"
      >
        <Form form={createForm} layout="vertical" onFinish={(values) => void create(values)}>
          <Form.Item
            label="组织标识"
            name="slug"
            rules={[
              { required: true, message: '请输入组织标识' },
              { pattern: /^[a-z0-9]([a-z0-9-]{1,30}[a-z0-9])$/, message: '3-32 位小写字母、数字与连字符，首尾不能是连字符' },
            ]}
            extra="用于 URL 与库定位，创建后不可修改。"
          >
            <Input placeholder="例如 acme" />
          </Form.Item>
          <Form.Item label="组织名称" name="displayName" rules={[{ required: true, message: '请输入组织名称' }]}>
            <Input placeholder="例如 Acme 组织" />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={`进入「${supportTenant?.display_name ?? ''}」支持访问`}
        open={supportTenant !== null}
        onCancel={() => setSupportTenant(null)}
        onOk={() => supportForm.submit()}
        okText="进入"
        cancelText="取消"
      >
        <p className="mb-2 text-sm text-[var(--colorTextSecondary,#666)]">
          进入后将临时获得该组织上下文，进出动作与理由会写入审计日志。
        </p>
        <Form form={supportForm} layout="vertical" onFinish={(values) => void enterSupport(values)}>
          <Form.Item
            label="支持访问理由"
            name="reason"
            rules={[
              { required: true, message: '请填写支持访问理由' },
              { max: 500, message: '理由不超过 500 字' },
            ]}
          >
            <Input.TextArea rows={3} placeholder="请描述本次排障或支持的事由" />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  )
}
