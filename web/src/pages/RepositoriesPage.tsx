import { useEffect, useState } from 'react'
import { Button, Checkbox, Form, Input, Modal, Select, Space, Switch, Table, Tag, message } from 'antd'
import { PlusOutlined } from '@ant-design/icons'

import { adminApi } from '../api/client'
import type { RegisteredRepository, RepositoryCreateInput, RepositoryPatchInput } from '../api/types'
import { ErrorState, LoadingState } from '../components/PageState'
import { PageHeading } from '../components/PageHeading'
import { StatusTag } from '../components/StatusTag'
import { useRemote } from '../hooks/useRemote'

export function RepositoriesPage() {
  const repositories = useRemote(() => adminApi.listRepositories(), [])
  const [editing, setEditing] = useState<RegisteredRepository | 'new'>()
  const [busyID, setBusyID] = useState<string>()

  const enable = async (repo: RegisteredRepository) => {
    setBusyID(repo.repository_id)
    try {
      await adminApi.enableRepository(repo.repository_id)
      message.success('仓库已启用')
      repositories.reload()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '启用失败')
    } finally {
      setBusyID(undefined)
    }
  }

  const disable = async (repo: RegisteredRepository) => {
    setBusyID(repo.repository_id)
    try {
      await adminApi.disableRepository(repo.repository_id)
      message.success('仓库已停用')
      repositories.reload()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '停用失败')
    } finally {
      setBusyID(undefined)
    }
  }

  return (
    <div>
      <PageHeading eyebrow="仓库配置" title="仓库管理" action={<Button type="primary" icon={<PlusOutlined />} onClick={() => setEditing('new')}>新增仓库</Button>} />
      <section className="surface-panel table-panel">
        {repositories.loading ? <LoadingState /> : repositories.error ? <ErrorState error={repositories.error} onRetry={repositories.reload} /> : (
          <Table<RegisteredRepository>
            className="admin-table admin-table--wide"
            rowKey="repository_id"
            size="middle"
            tableLayout="fixed"
            scroll={{ x: 1300 }}
            pagination={false}
            dataSource={repositories.data?.items ?? []}
            columns={[
              { title: '仓库', width: 280, render: (_, row) => <div><strong>{row.display_name}</strong><small className="table-secondary mono">{row.path_with_namespace}</small></div> },
              { title: '平台', width: 200, render: (_, row) => <div><Tag>{row.provider.toUpperCase()}</Tag><small className="table-secondary mono">{row.base_url}</small></div> },
              { title: '状态', dataIndex: 'status', width: 130, render: (value: string) => <StatusTag status={value} /> },
              { title: 'Token', dataIndex: 'token_configured', width: 110, render: (value: boolean) => value ? '已配置' : '未配置' },
              { title: 'Webhook Secret', dataIndex: 'webhook_secret_configured', width: 140, render: (value: boolean) => value ? '已配置' : '未配置' },
              { title: 'Issue 标签', dataIndex: 'codehost_issue_labels', width: 200, ellipsis: true, render: (values?: string[]) => values?.length ? <Space size={4} wrap>{values.map((value) => <Tag key={value}>{value}</Tag>)}</Space> : '—' },
              { title: '操作', width: 160, render: (_, row) => {
                const enabled = row.status !== 'disabled' && row.status !== 'configuring'
                return <Space wrap>
                  <Switch
                    checked={enabled}
                    checkedChildren="启用"
                    unCheckedChildren="停用"
                    aria-label={`${enabled ? '停用' : '启用'} ${row.display_name}`}
                    loading={busyID === row.repository_id}
                    onChange={(checked) => {
                      if (checked) {
                        void enable(row)
                        return
                      }
                      Modal.confirm({ title: '停用该仓库？', content: '活动任务未结束时会拒绝停用。', okText: '停用', cancelText: '取消', onOk: () => disable(row) })
                    }}
                  />
                  <Button aria-label="编辑仓库" onClick={() => setEditing(row)}>编辑</Button>
                </Space>
              } },
              { title: '最近配置错误', dataIndex: 'last_configuration_error', width: 260, ellipsis: true, render: (value: string) => <span className={value ? 'error-copy' : 'muted-copy'}>{value || '—'}</span> },
            ]}
          />
        )}
      </section>
      {editing ? <RepositoryForm repository={editing} onClose={() => setEditing(undefined)} onSaved={() => { setEditing(undefined); repositories.reload() }} /> : null}
    </div>
  )
}

function SecretConfiguredLabel({ label, configured, optional = false }: { label: string; configured: boolean; optional?: boolean }) {
  return (
    <Space size={4}>
      <span>{label}{optional ? '（可选）' : ''}</span>
      {configured ? <Tag color="success">已配置</Tag> : null}
    </Space>
  )
}

const platformOptions = [
  { value: 'gitlab', label: 'GitLab' },
  { value: 'gitea', label: 'Gitea' },
  { value: 'github', label: 'GitHub' },
]

const platformBaseURLPlaceholder: Record<string, string> = {
  gitlab: 'https://gitlab.example.com',
  gitea: 'https://gitea.example.com',
  github: 'https://github.example.com',
}

async function validateBaseURL(_rule: unknown, value: string) {
  if (!value) {
    return
  }
  try {
    const parsed = new URL(value)
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
      throw new Error('protocol')
    }
  } catch {
    throw new Error('平台地址必须是 http(s) 地址')
  }
}

function RepositoryForm({ repository, onClose, onSaved }: { repository: RegisteredRepository | 'new'; onClose: () => void; onSaved: () => void }) {
  const [form] = Form.useForm<RepositoryCreateInput>()
  const [saving, setSaving] = useState(false)
  const creating = repository === 'new'
  const configuration = useRemote(() => adminApi.tenantConfiguration(), [])
  const provider = Form.useWatch('provider', form) || 'gitlab'
  const initialValues = creating
    ? { display_name: '', path_with_namespace: '', provider: 'gitlab', base_url: '', token: '', token_scope_attested: false, webhook_secret: '' }
    : { display_name: repository.display_name, path_with_namespace: repository.path_with_namespace, provider: repository.provider, base_url: repository.base_url, token: '', token_scope_attested: false, webhook_secret: '' }

  // 组织段 codehost 配置作为注册表单默认值:仅创建时注入,运行时不再消费。
  useEffect(() => {
    if (creating && configuration.data?.config?.codehost) {
      form.setFieldsValue({
        provider: configuration.data.config.codehost.provider,
        base_url: configuration.data.config.codehost.base_url,
      })
    }
  }, [creating, configuration.data, form])

  const save = async () => {
    const values = await form.validateFields()
    setSaving(true)
    try {
      if (creating) {
        await adminApi.createRepository(values)
        message.success('仓库已登记')
      } else {
        const patch: RepositoryPatchInput = {
          display_name: values.display_name,
          path_with_namespace: values.path_with_namespace,
          provider: values.provider,
          base_url: values.base_url,
        }
        if (values.token) {
          patch.token = values.token
          patch.token_scope_attested = values.token_scope_attested
        }
        if (values.webhook_secret) {
          patch.webhook_secret = values.webhook_secret
        }
        await adminApi.patchRepository(repository.repository_id, patch)
        message.success('仓库已更新')
      }
      onSaved()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal title={creating ? '新增仓库' : '编辑仓库'} open onCancel={onClose} onOk={save} okText="保存" cancelText="取消" confirmLoading={saving} destroyOnClose width={640}>
      <Form form={form} layout="vertical" preserve={false} initialValues={initialValues}>
        <Form.Item name="display_name" label="显示名称" rules={[{ required: true, message: '请输入显示名称' }]}><Input /></Form.Item>
        <Form.Item name="provider" label="平台" rules={[{ required: true, message: '请选择平台' }]}>
          <Select options={platformOptions} />
        </Form.Item>
        <Form.Item name="base_url" label="平台地址" rules={[{ required: true, message: '请输入平台地址' }, { validator: validateBaseURL }]}>
          <Input className="mono" placeholder={platformBaseURLPlaceholder[provider] ?? 'https://gitlab.example.com'} />
        </Form.Item>
        <Form.Item name="path_with_namespace" label="仓库路径" rules={[{ required: true, message: '请输入仓库路径（owner/repo）' }]}><Input className="mono" placeholder="group/repository" /></Form.Item>
        <Form.Item
          name="token"
          label={<SecretConfiguredLabel label="项目级 Token" configured={!creating && repository.token_configured} />}
          rules={creating ? [{ required: true, message: '请输入该平台的项目级 Token' }] : undefined}
        >
          <Input.Password autoComplete="new-password" placeholder={creating ? '' : '留空表示保持已配置 token'} />
        </Form.Item>
        <Form.Item
          name="webhook_secret"
          label={<SecretConfiguredLabel label="Webhook Secret" configured={!creating && repository.webhook_secret_configured} />}
          rules={creating ? [{ required: true, message: '请输入该仓库的 Webhook Secret' }] : undefined}
        >
          <Input.Password autoComplete="new-password" placeholder={creating ? '平台侧 webhook 配置的签名密钥' : '留空表示保持已配置 secret'} />
        </Form.Item>
        <p className="form-note">Webhook Secret 用于验证此仓库的 webhook 事件（按仓持有、可按仓轮换，改动保存后即时生效）；列表只展示是否已配置，不会回显原值。平台侧 webhook 的 secret 需与此处一致。</p>
        <Form.Item
          name="token_scope_attested"
          valuePropName="checked"
          rules={[{
            validator: async (_, value) => {
              if (creating || form.getFieldValue('token')) {
                if (!value) throw new Error('请确认这是项目级 token')
              }
            },
          }]}
        >
          <Checkbox>
            操作者确认这是该平台项目级（repository 级）token，不是用户/组/实例 token。
          </Checkbox>
        </Form.Item>
        <p className="form-note">Token 只用于创建或轮换，列表和详情只展示是否已配置，不会回显原值。</p>
      </Form>
    </Modal>
  )
}
