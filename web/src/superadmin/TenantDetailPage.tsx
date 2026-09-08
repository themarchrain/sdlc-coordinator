import { Button, Card, Col, Form, Input, Modal, Popconfirm, Row, Space, Table, Tag, Typography, message } from 'antd'
import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import {
  portalApi,
  type PortalInviteCodeRecord,
  type PortalMember,
  type PortalRunnerAssignment,
} from '../api/portal'

function formatDateTime(value: string | null | undefined): string {
  if (!value) return '—'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

// TenantDetailPage 是超管视角的组织详情：管理员开通与密码重置、成员与邀请码
// 查看、runner 静态指派（Phase 1 形态）。
export function TenantDetailPage() {
  const tenantId = Number(useParams<'id'>().id)
  const navigate = useNavigate()
  const [members, setMembers] = useState<PortalMember[]>([])
  const [inviteCodes, setInviteCodes] = useState<PortalInviteCodeRecord[]>([])
  const [assignments, setAssignments] = useState<PortalRunnerAssignment[]>([])
  const [provisioning, setProvisioning] = useState(false)
  const [provisioned, setProvisioned] = useState<{ username: string; initialPassword: string } | null>(null)
  const [runnerDraft, setRunnerDraft] = useState('')
  const [username, setUsername] = useState('')

  const reload = useCallback(async () => {
    const [memberPayload, invitePayload, runnerPayload] = await Promise.all([
      portalApi.superListTenantMembers(tenantId),
      portalApi.superListInviteCodes(tenantId),
      portalApi.superListRunnerAssignments(tenantId),
    ])
    setMembers(memberPayload.members)
    setInviteCodes(invitePayload.invite_codes)
    setAssignments(runnerPayload.runner_assignments)
    setRunnerDraft(runnerPayload.runner_assignments.map((item) => item.endpoint_url).join('\n'))
  }, [tenantId])

  useEffect(() => {
    reload().catch((reason: unknown) => {
      message.error(reason instanceof Error ? reason.message : '加载组织详情失败')
    })
  }, [reload])

  const provision = async () => {
    if (!username.trim()) {
      message.warning('请输入用户名')
      return
    }
    setProvisioning(true)
    try {
      const payload = await portalApi.superProvisionAdmin(tenantId, username.trim())
      setProvisioned({ username: payload.username, initialPassword: payload.initial_password ?? '' })
      setUsername('')
      await reload()
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '开通失败')
    } finally {
      setProvisioning(false)
    }
  }

  const resetAdminPassword = async (userId: number) => {
    try {
      const payload = await portalApi.superResetAdminPassword(tenantId, userId)
      Modal.info({
        title: '管理员密码已重置',
        content: <Typography.Paragraph copyable code>{payload.initial_password}</Typography.Paragraph>,
      })
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '重置失败')
    }
  }

  const revokeInvite = async (codeId: number) => {
    try {
      await portalApi.superRevokeInviteCode(tenantId, codeId)
      message.success('邀请码已吊销')
      await reload()
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '吊销失败')
    }
  }

  const saveRunners = async () => {
    const endpoints = runnerDraft.split('\n').map((line) => line.trim()).filter(Boolean)
    try {
      await portalApi.superReplaceRunnerAssignments(tenantId, endpoints)
      message.success('runner 指派已更新')
      await reload()
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '更新失败')
    }
  }

  return (
    <Row gutter={[12, 12]}>
      <Col span={24}>
        <Button onClick={() => navigate('/super-admin/tenants')}>返回组织列表</Button>
      </Col>
      <Col span={24}>
        <Card title="开通组织管理员">
          <Space>
            <Input
              placeholder="用户名"
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              style={{ width: 240 }}
            />
            <Button type="primary" loading={provisioning} onClick={() => void provision()}>开通组织管理员</Button>
          </Space>
          <p className="mt-2 text-sm text-[var(--colorTextSecondary,#666)]">
            新用户名将获得一次性初始密码并强制改密；已有账号则直接授予组织管理员角色。
          </p>
        </Card>
      </Col>
      <Col span={24}>
        <Card title="成员列表">
          <Table<PortalMember>
            rowKey="user_id"
            size="small"
            dataSource={members}
            pagination={false}
            columns={[
              { title: '用户名', dataIndex: 'username' },
              {
                title: '角色',
                dataIndex: 'role',
                render: (role: string) => (
                  <Tag color={role === 'tenant_admin' ? 'blue' : 'default'}>
                    {role === 'tenant_admin' ? '组织管理员' : '成员'}
                  </Tag>
                ),
              },
              { title: '加入时间', dataIndex: 'created_at', render: formatDateTime },
              {
                title: '操作',
                key: 'actions',
                render: (_, record) =>
                  record.role === 'tenant_admin' ? (
                    <Popconfirm title="将生成一次性初始密码并吊销其全部会话，确认重置？" onConfirm={() => void resetAdminPassword(record.user_id)}>
                      <Button size="small">重置密码</Button>
                    </Popconfirm>
                  ) : null,
              },
            ]}
          />
        </Card>
      </Col>
      <Col span={24}>
        <Card title="邀请码">
          <Table<PortalInviteCodeRecord>
            rowKey="id"
            size="small"
            dataSource={inviteCodes}
            pagination={false}
            columns={[
              { title: '编号', dataIndex: 'id' },
              {
                title: '状态',
                key: 'state',
                render: (_, record) => {
                  if (record.revoked_at) return <Tag color="red">已吊销</Tag>
                  if (new Date(record.expires_at).getTime() < Date.now()) return <Tag color="orange">已过期</Tag>
                  if (record.used_count >= record.max_uses) return <Tag color="orange">已用尽</Tag>
                  return <Tag color="green">可用</Tag>
                },
              },
              { title: '使用情况', key: 'usage', render: (_, record) => `已使用 ${record.used_count} / ${record.max_uses}` },
              { title: '过期时间', dataIndex: 'expires_at', render: formatDateTime },
              {
                title: '操作',
                key: 'actions',
                render: (_, record) =>
                  record.revoked_at ? null : (
                    <Popconfirm title="确认吊销该邀请码？" onConfirm={() => void revokeInvite(record.id)}>
                      <Button size="small" danger>吊销</Button>
                    </Popconfirm>
                  ),
              },
            ]}
          />
        </Card>
      </Col>
      <Col span={24}>
        <Card title="Runner 指派（静态池）" extra={<Button onClick={() => void saveRunners()}>保存</Button>}>
          <p className="mb-2 text-sm text-[var(--colorTextSecondary,#666)]">每行一个 runner 端点（http/https），保存即整表替换。</p>
          <Input.TextArea rows={4} value={runnerDraft} onChange={(event) => setRunnerDraft(event.target.value)} />
        </Card>
      </Col>

      <Modal open={provisioned !== null} title="组织管理员已开通" footer={null} onCancel={() => setProvisioned(null)}>
        {provisioned && (
          <div>
            <p>账号 {provisioned.username} 已开通。一次性初始密码（请立即转交本人）：</p>
            <Typography.Paragraph copyable code style={{ fontSize: 18 }}>{provisioned.initialPassword}</Typography.Paragraph>
            {provisioned.initialPassword === '' && <p>该账号为已有用户，密码保持不变。</p>}
          </div>
        )}
      </Modal>
    </Row>
  )
}
