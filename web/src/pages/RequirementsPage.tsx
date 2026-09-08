import { useEffect, useState } from 'react'
import { Button, Form, Input, Modal, Popconfirm, Select, Space, Table, Tag, message } from 'antd'
import { CheckOutlined, PlusOutlined, StopOutlined, SyncOutlined } from '@ant-design/icons'

import { adminApi } from '../api/client'
import type { RegisteredRepository, Requirement } from '../api/types'
import { ErrorState } from '../components/PageState'
import { PageHeading } from '../components/PageHeading'
import { useCanWrite } from '../app/workspaceSession'

const statusLabels: Record<Requirement['status'], { label: string; color: string }> = {
  queued: { label: '待同步', color: 'warning' },
  published: { label: '已发布', color: 'processing' },
  done: { label: '已完成', color: 'green' },
  failed: { label: '同步失败', color: 'red' },
  cancelled: { label: '已取消', color: 'default' },
}

const priorityLabels: Record<string, string> = { high: '高', medium: '中', low: '低' }

export function RequirementsPage() {
  const canWrite = useCanWrite()
  const [items, setItems] = useState<Requirement[]>([])
  const [repositories, setRepositories] = useState<RegisteredRepository[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<unknown>()
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [editorOpen, setEditorOpen] = useState(false)
  const [editing, setEditing] = useState<Requirement | null>(null)
  const [form] = Form.useForm()

  const load = async (nextPage = page) => {
    setLoading(true)
    setError(undefined)
    try {
      const result = await adminApi.listRequirements({ page: nextPage, page_size: 20 })
      setItems(result.items)
      setPage(result.page)
      setTotal(result.total)
    } catch (reason) {
      setError(reason)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load(1)
    void adminApi.listRepositories()
      .then((result) => setRepositories(result.items.filter((item) => item.status === 'enabled')))
      .catch(() => setRepositories([]))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    setEditorOpen(true)
  }

  const openEdit = (requirement: Requirement) => {
    setEditing(requirement)
    form.setFieldsValue({
      title: requirement.title,
      description: requirement.description,
      repository_id: requirement.repository_id,
      labels: requirement.labels,
      priority: requirement.priority,
    })
    setEditorOpen(true)
  }

  const submit = async () => {
    try {
      const values = await form.validateFields()
      if (editing) {
        await adminApi.patchRequirement(editing.requirement_id, values)
        message.success('需求已更新')
      } else {
        await adminApi.createRequirement(values)
        message.success('需求已创建，等待同步')
      }
      setEditorOpen(false)
      await load(page)
    } catch (reason) {
      if (reason instanceof Error) message.error(reason.message)
    }
  }

  const act = async (action: () => Promise<unknown>, success: string) => {
    try {
      await action()
      message.success(success)
      await load(page)
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '操作失败')
    }
  }

  const runSync = async () => {
    try {
      const { result } = await adminApi.triggerRequirementSync()
      message.success(`同步完成：新建 ${result.created}，恢复 ${result.recovered}，失败 ${result.failed}，完成 ${result.mirrored}`)
      await load(page)
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '同步失败')
    }
  }

  const editable = (requirement: Requirement) => requirement.status === 'queued' || requirement.status === 'failed'

  return <div className="requirements-page">
    <PageHeading eyebrow="需求入口" title="需求管理" />
    <section className="surface-panel">
      <div className="people-toolbar">
        {canWrite ? (
          <Space>
            <Button icon={<SyncOutlined />} onClick={() => void runSync()}>立即同步</Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新建需求</Button>
          </Space>
        ) : null}
      </div>
      {error ? <ErrorState error={error} /> : <Table<Requirement>
        size="small" loading={loading} rowKey="requirement_id" dataSource={items}
        pagination={{ current: page, pageSize: 20, total, showSizeChanger: false, onChange: (nextPage) => void load(nextPage) }}
        columns={[
          { title: '需求', render: (_, requirement) => <div><strong>{requirement.title}</strong><small className="table-secondary">{requirement.description}</small></div> },
          { title: '状态', width: 100, render: (_, requirement) => <Tag color={statusLabels[requirement.status].color}>{statusLabels[requirement.status].label}</Tag> },
          { title: '优先级', width: 80, render: (_, requirement) => priorityLabels[requirement.priority] ?? '—' },
          { title: '仓库', width: 200, dataIndex: 'repository_id' },
          { title: 'Issue', width: 90, render: (_, requirement) => requirement.issue_url ? <a href={requirement.issue_url} target="_blank" rel="noreferrer">#{requirement.issue_iid}</a> : '—' },
          { title: '错误', render: (_, requirement) => requirement.last_error ? <small className="table-secondary">{requirement.last_error}</small> : '—' },
          { title: '创建时间', width: 170, dataIndex: 'created_at', render: (value: string) => value ? new Date(value).toLocaleString() : '—' },
          ...(canWrite ? [{ title: '操作', width: 240, render: (_: unknown, requirement: Requirement) => <Space size={0}>
            {editable(requirement) ? <Button type="text" size="small" onClick={() => openEdit(requirement)}>编辑</Button> : null}
            {requirement.status === 'failed' ? <Button type="text" size="small" onClick={() => void act(() => adminApi.publishRequirement(requirement.requirement_id), '已重新排队')}>重试发布</Button> : null}
            {requirement.status === 'published' ? <Popconfirm title="确认该需求已完成？" okText="完成" cancelText="取消" onConfirm={() => void act(() => adminApi.completeRequirement(requirement.requirement_id), '已标记完成')}><Button type="text" size="small" icon={<CheckOutlined />}>完成</Button></Popconfirm> : null}
            {editable(requirement) ? <Popconfirm title="取消该需求？" okText="取消需求" cancelText="返回" onConfirm={() => void act(() => adminApi.cancelRequirement(requirement.requirement_id), '需求已取消')}><Button type="text" size="small" danger icon={<StopOutlined />}>取消</Button></Popconfirm> : null}
          </Space> }] : []),
        ]}
      />}
    </section>
    <Modal
      open={editorOpen}
      title={editing ? '编辑需求' : '新建需求'}
      okText="保存"
      cancelText="取消"
      onOk={() => void submit()}
      onCancel={() => setEditorOpen(false)}
      destroyOnClose
    >
      <Form form={form} layout="vertical" initialValues={{ priority: '', labels: [] }}>
        <Form.Item name="title" label="标题" rules={[{ required: true, message: '请输入需求标题' }]}>
          <Input placeholder="需求标题" />
        </Form.Item>
        <Form.Item name="repository_id" label="目标仓库" rules={[{ required: true, message: '请选择目标仓库' }]}>
          <Select placeholder="选择已启用的仓库" options={repositories.map((item) => ({ value: item.repository_id, label: item.display_name || item.path_with_namespace }))} />
        </Form.Item>
        <Form.Item name="priority" label="优先级">
          <Select options={[{ value: '', label: '无' }, { value: 'high', label: '高' }, { value: 'medium', label: '中' }, { value: 'low', label: '低' }]} />
        </Form.Item>
        <Form.Item name="labels" label="标签">
          <Select mode="tags" placeholder="回车添加标签（发布时自动追加 developer-agent）" />
        </Form.Item>
        <Form.Item name="description" label="描述">
          <Input.TextArea rows={6} placeholder="需求背景、验收标准（Markdown）" />
        </Form.Item>
      </Form>
    </Modal>
  </div>
}
