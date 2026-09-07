import { useState } from 'react'
import { Button, Descriptions, Drawer, Form, Input, message, Modal, Popconfirm, Space, Switch, Table, Tag } from 'antd'
import { ApiOutlined, DeleteOutlined, EditOutlined, LoginOutlined, PlusOutlined, RadarChartOutlined } from '@ant-design/icons'

import { adminApi } from '../api/client'
import type { Connector } from '../api/types'
import { ErrorState, LoadingState } from '../components/PageState'
import { PageHeading } from '../components/PageHeading'
import { StatusTag } from '../components/StatusTag'
import { ConnectorConfigurationModal } from '../components/ConnectorConfigurationModal'
import { useRemote } from '../hooks/useRemote'

export function ConnectorsPage() {
  const connectors = useRemote(() => adminApi.listConnectors({ page: 1, page_size: 100 }), [])
  const [editing, setEditing] = useState<Connector | 'new'>()
  const [selected, setSelected] = useState<Connector>()
  const [busyID, setBusyID] = useState<string>()
  const [configuring, setConfiguring] = useState<Connector>()

  const probe = async (connector: Connector) => {
    setBusyID(connector.connector_id)
    try {
      await adminApi.probeConnector(connector.connector_id)
      message.success('健康检查完成')
      connectors.reload()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '健康检查失败')
    } finally {
      setBusyID(undefined)
    }
  }

  const remove = async (connector: Connector) => {
    await adminApi.deleteConnector(connector.connector_id)
    message.success('连接器注册信息已删除')
    connectors.reload()
  }

  return (
    <div>
      <PageHeading eyebrow="连接器实例" title="连接器实例" action={<Button type="primary" icon={<PlusOutlined />} onClick={() => setEditing('new')}>注册连接器</Button>} />
      <section className="surface-panel table-panel">
        {connectors.loading ? <LoadingState /> : connectors.error ? <ErrorState error={connectors.error} onRetry={connectors.reload} /> : (
          <Table<Connector>
            rowKey="connector_id"
            dataSource={connectors.data?.items ?? []}
            pagination={false}
            columns={[
              { title: '连接器', render: (_, row) => <Button type="link" className="connector-name" onClick={() => setSelected(row)}><ApiOutlined /> {row.display_name}</Button> },
              { title: '类型', render: (_, row) => <span className="mono">{row.kind}</span> },
              { title: '健康', render: (_, row) => <div><StatusTag status={row.last_health_status} /><small className="table-secondary">{row.last_health_message || '尚未探测'}</small></div> },
              { title: '能力', render: (_, row) => <Space size={[4, 4]} wrap>{row.capabilities.length ? row.capabilities.map((item) => <Tag key={item}>{item}</Tag>) : <span className="muted-copy">等待探测</span>}</Space> },
              { title: '状态', dataIndex: 'enabled', render: (value) => value ? <Tag color="blue">已启用</Tag> : <Tag>已禁用</Tag> },
              { title: '操作', width: 320, render: (_, row) => <Space>
                <Button icon={<LoginOutlined />} onClick={() => setConfiguring(row)}>实例配置</Button>
                <Button loading={busyID === row.connector_id} icon={<RadarChartOutlined />} onClick={() => probe(row)}>探测</Button>
                <Button aria-label={`编辑 ${row.display_name}`} icon={<EditOutlined />} onClick={() => setEditing(row)} />
                <Popconfirm title="删除注册信息？" description="不会停止远端服务，也不会删除远端数据。" onConfirm={() => remove(row)} okText="删除" cancelText="取消">
                  <Button danger aria-label={`删除 ${row.display_name}`} icon={<DeleteOutlined />} />
                </Popconfirm>
              </Space> },
            ]}
          />
        )}
      </section>
      {editing ? <ConnectorForm connector={editing} onClose={() => setEditing(undefined)} onSaved={() => { setEditing(undefined); connectors.reload() }} /> : null}
      <ConnectorConfigurationModal connector={configuring} onClose={() => setConfiguring(undefined)} onSaved={() => { setConfiguring(undefined); window.setTimeout(connectors.reload, 1200) }} />
      <ConnectorDrawer connector={selected} onClose={() => setSelected(undefined)} />
    </div>
  )
}

function ConnectorForm({ connector, onClose, onSaved }: { connector: Connector | 'new'; onClose: () => void; onSaved: () => void }) {
  const [form] = Form.useForm()
  const [saving, setSaving] = useState(false)
  const initialValues: Partial<Connector> = connector === 'new' ? { enabled: true } : connector
  const save = async () => {
    const value = await form.validateFields()
    setSaving(true)
    try {
      if (connector === 'new') await adminApi.createConnector(value)
      else await adminApi.updateConnector(connector.connector_id, value)
      message.success(connector === 'new' ? '连接器已注册' : '连接器已更新')
      onSaved()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }
  return <Modal title={connector === 'new' ? '注册连接器' : '编辑连接器'} open onCancel={onClose} onOk={save} okText="保存" cancelText="取消" confirmLoading={saving} destroyOnClose>
    <Form form={form} layout="vertical" preserve={false} initialValues={initialValues}>
      <Form.Item name="display_name" label="显示名称" rules={[{ required: true, message: '请输入显示名称' }]}><Input placeholder="状态源同步器" /></Form.Item>
      <Form.Item name="kind" label="连接器类型" rules={[{ required: true, message: '请输入连接器类型' }]}><Input className="mono" placeholder="my_state_source_sync" /></Form.Item>
      <Form.Item name="base_url" label="服务地址" rules={[{ required: true, message: '请输入 Coordinator 可访问的 HTTP 地址' }, { type: 'url', message: '请输入有效 URL' }]}><Input className="mono" placeholder="http://connector:8989" /></Form.Item>
      <Form.Item name="enabled" label="允许管理操作" valuePropName="checked"><Switch /></Form.Item>
      <p className="form-note">这里只保存管理地址和展示信息。连接器的具体配置在「实例配置」里以 JSON 形式填写，字段由连接器插件自定义。</p>
    </Form>
  </Modal>
}

function ConnectorDrawer({ connector, onClose }: { connector?: Connector; onClose: () => void }) {
  return <Drawer width={640} open={Boolean(connector)} onClose={onClose} title={connector?.display_name} destroyOnClose>
    {connector ? <div className="drawer-stack">
      <Descriptions bordered size="small" column={2} items={[
        { key: 'health', label: '健康', children: <StatusTag status={connector.last_health_status} /> },
        { key: 'enabled', label: '管理状态', children: connector.enabled ? '已启用' : '已禁用' },
        { key: 'kind', label: '类型', span: 2, children: <span className="mono">{connector.kind}</span> },
        { key: 'url', label: '服务地址', span: 2, children: <span className="mono">{connector.base_url}</span> },
        { key: 'message', label: '最近健康摘要', span: 2, children: connector.last_health_message || '尚未探测' },
      ]} />
    </div> : null}
  </Drawer>
}
