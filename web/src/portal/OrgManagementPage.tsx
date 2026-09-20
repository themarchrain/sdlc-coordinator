import { Button, Descriptions, Form, InputNumber, Modal, Popconfirm, Result, Space, Table, Tag, Typography, message } from 'antd'
import { SettingOutlined } from '@ant-design/icons'
import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { portalApi, type PortalInviteCodeRecord, type PortalMember, type PortalTenant } from '../api/portal'
import { PageHeading } from '../components/PageHeading'

interface Profile {
  tenant: PortalTenant
  role: string
  support: boolean
}

function formatDateTime(value: string | null | undefined): string {
  if (!value) return '—'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

// OrgManagementPage 是组织内「组织管理」工作台：组织信息、成员列表、
// 邀请码管理与组织配置入口，仅组织管理员（tenant_admin）可用；支持访问
// 中的超管同样可见。
export function OrgManagementPage() {
  const navigate = useNavigate()
  const [profile, setProfile] = useState<Profile | null>(null)
  const [members, setMembers] = useState<PortalMember[]>([])
  const [inviteCodes, setInviteCodes] = useState<PortalInviteCodeRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [issuing, setIssuing] = useState(false)
  const [issuedCode, setIssuedCode] = useState<string | null>(null)

  const reload = useCallback(async () => {
    const profilePayload = await portalApi.tenantProfile()
    setProfile(profilePayload)
    if (profilePayload.role === 'tenant_admin' || profilePayload.support) {
      const [memberPayload, invitePayload] = await Promise.all([
        portalApi.listMembers(),
        portalApi.listInviteCodes(),
      ])
      setMembers(memberPayload.members)
      setInviteCodes(invitePayload.invite_codes)
    }
    setLoading(false)
  }, [])

  useEffect(() => {
    reload().catch((reason: unknown) => {
      message.error(reason instanceof Error ? reason.message : '加载组织信息失败')
      setLoading(false)
    })
  }, [reload])

  const issue = async (values?: { expiresInHours?: number; maxUses?: number }) => {
    setIssuing(true)
    try {
      const payload = await portalApi.createInviteCode(values?.expiresInHours, values?.maxUses)
      setIssuedCode(payload.code)
      await portalApi.listInviteCodes().then((next) => setInviteCodes(next.invite_codes))
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '生成邀请码失败')
    } finally {
      setIssuing(false)
    }
  }

  const revoke = async (codeId: number) => {
    try {
      await portalApi.revokeInviteCode(codeId)
      message.success('邀请码已吊销')
      const next = await portalApi.listInviteCodes()
      setInviteCodes(next.invite_codes)
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '吊销失败')
    }
  }

  const removeMember = async (userId: number) => {
    try {
      await portalApi.removeMember(userId)
      message.success('成员已移除，其会话即时失效')
      const next = await portalApi.listMembers()
      setMembers(next.members)
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '移除成员失败')
    }
  }

  const resetPassword = async (userId: number) => {
    try {
      const payload = await portalApi.resetMemberPassword(userId)
      Modal.info({
        title: '密码已重置',
        content: (
          <div>
            <p>请将以下一次性初始密码告知该成员，其下次登录须修改密码：</p>
            <Typography.Paragraph copyable code>{payload.initial_password}</Typography.Paragraph>
          </div>
        ),
      })
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '重置密码失败')
    }
  }

  if (loading) return null
  if (!profile) return null
  const isAdmin = profile.role === 'tenant_admin' || profile.support
  if (!isAdmin) {
    return (
      <Result
        status="403"
        title="需要组织管理员权限"
        subTitle="「组织管理」页面仅对组织管理员开放。"
      />
    )
  }

  return (
    <div className="org-page">
      <PageHeading
        eyebrow="组织管理"
        title={profile.tenant.display_name}
        action={<Button type="primary" icon={<SettingOutlined />} onClick={() => navigate('/org/settings')}>组织配置</Button>}
      />
      <section className="surface-panel">
        <Descriptions title="组织信息" size="small" column={3}>
          <Descriptions.Item label="组织标识">{profile.tenant.slug}</Descriptions.Item>
          <Descriptions.Item label="状态">
            <Tag color={profile.tenant.status === 'active' ? 'green' : 'red'}>
              {profile.tenant.status === 'active' ? '正常' : '已停用'}
            </Tag>
          </Descriptions.Item>
          <Descriptions.Item label="我的角色">
            {profile.support ? '支持访问（超管）' : '组织管理员'}
          </Descriptions.Item>
        </Descriptions>
      </section>

      <section className="surface-panel" data-testid="members-card">
        <h3>成员列表</h3>
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
              render: (_, record) => (
                <Space>
                  {record.role === 'member' && (
                    <Button size="small" onClick={() => void resetPassword(record.user_id)}>重置密码</Button>
                  )}
                  <Popconfirm
                    title="移除后该成员在本组织的会话将立即失效，确认移除？"
                    onConfirm={() => void removeMember(record.user_id)}
                  >
                    <Button size="small" danger>移除</Button>
                  </Popconfirm>
                </Space>
              ),
            },
          ]}
        />
      </section>

      <section className="surface-panel">
        <div className="configuration-inline-title">
          <h3>邀请码管理</h3>
          <Button type="primary" loading={issuing} onClick={() => void issue()}>生成邀请码</Button>
        </div>
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
            {
              title: '使用情况',
              key: 'usage',
              render: (_, record) => `已使用 ${record.used_count} / ${record.max_uses}`,
            },
            { title: '过期时间', dataIndex: 'expires_at', render: formatDateTime },
            { title: '签发时间', dataIndex: 'created_at', render: formatDateTime },
            {
              title: '操作',
              key: 'actions',
              render: (_, record) =>
                record.revoked_at ? null : (
                  <Popconfirm title="确认吊销该邀请码？" onConfirm={() => void revoke(record.id)}>
                    <Button size="small" danger>吊销</Button>
                  </Popconfirm>
                ),
            },
          ]}
        />
        <Form
          layout="inline"
          className="org-invite-form"
          onFinish={(values: { expiresInHours?: number; maxUses?: number }) => void issue(values)}
        >
          <Form.Item name="expiresInHours" label="有效期（小时）" initialValue={72}>
            <InputNumber min={1} max={720} style={{ width: 120 }} />
          </Form.Item>
          <Form.Item name="maxUses" label="次数上限" initialValue={10}>
            <InputNumber min={1} max={1000} style={{ width: 120 }} />
          </Form.Item>
          <Button htmlType="submit" loading={issuing}>按参数生成</Button>
        </Form>
      </section>

      <Modal
        open={issuedCode !== null}
        title="邀请码已生成（仅显示一次）"
        okText="我已保存"
        cancelText={null}
        onOk={() => setIssuedCode(null)}
        onClose={() => setIssuedCode(null)}
      >
        <p>请立即复制并妥善保存，关闭后将无法再次查看：</p>
        <Typography.Paragraph copyable code style={{ fontSize: 18 }}>{issuedCode}</Typography.Paragraph>
      </Modal>
    </div>
  )
}
