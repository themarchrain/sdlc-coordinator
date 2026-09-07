import { useEffect, useState } from 'react'
import { Alert, Button, Form, InputNumber, message, Modal, Space, Table } from 'antd'
import { EditOutlined, PlayCircleOutlined, RedoOutlined } from '@ant-design/icons'

import { adminApi } from '../api/client'
import type { TickRequest, TickResult } from '../api/types'
import { ErrorState, LoadingState } from '../components/PageState'
import { PageHeading } from '../components/PageHeading'
import { StatusTag } from '../components/StatusTag'

type TickResultGroupKey = 'polled_runs' | 'reconciled_writebacks' | 'finalized_results' | 'recoveries' | 'dispatched_tasks' | 'dispatch_failures' | 'executed_effects'

const resultGroups: Array<{ key: TickResultGroupKey; label: string; noun: string }> = [
  { key: 'polled_runs', label: '轮询', noun: 'Run' },
  { key: 'reconciled_writebacks', label: '写回对账', noun: 'Writeback' },
  { key: 'finalized_results', label: '结果完成', noun: 'Result' },
  { key: 'recoveries', label: '租约恢复', noun: 'Task' },
  { key: 'dispatched_tasks', label: '派发', noun: 'Task' },
  { key: 'dispatch_failures', label: '派发失败', noun: 'Task' },
  { key: 'executed_effects', label: '通知执行', noun: 'Effect' },
]

export function OperationsPage() {
  const [form] = Form.useForm<TickRequest>()
  const [defaultsError, setDefaultsError] = useState<unknown>()
  const [loading, setLoading] = useState(true)
  const [editing, setEditing] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [running, setRunning] = useState(false)
  const [result, setResult] = useState<TickResult>()
  const [effects, setEffects] = useState<Array<Record<string, unknown>>>([])
  const [effectsError, setEffectsError] = useState<unknown>()

  const loadEffects = () => adminApi.listEntities('effects', { page: 1, page_size: 20, status: 'failed' })
    .then((page) => setEffects(page.items)).catch(setEffectsError)

  useEffect(() => {
    Promise.all([adminApi.tickDefaults().then((value) => form.setFieldsValue(value)).catch(setDefaultsError), loadEffects()])
      .finally(() => setLoading(false))
  }, [form])

  const runTick = async () => {
    setConfirmOpen(false)
    setRunning(true)
    try {
      const value = await form.validateFields()
      const tickResult = await adminApi.tick(value)
      setResult(tickResult)
      message.success('Tick 已完成')
      loadEffects()
    } catch (error) {
      message.error(error instanceof Error ? error.message : 'Tick 执行失败')
    } finally {
      setRunning(false)
    }
  }

  const retry = async (effectID: string) => {
    try {
      await adminApi.retryEffect(effectID)
      message.success('Effect 已恢复为 pending')
      await loadEffects()
    } catch (error) {
      message.error(error instanceof Error ? error.message : 'Effect 重试失败')
    }
  }

  const saveEdit = async () => {
    try {
      await form.validateFields()
      setEditing(false)
      message.success('参数已保存，将在下次手动 Tick 时生效')
    } catch {
      message.error('请修正表单校验错误后再保存')
    }
  }

  const cancelEdit = () => {
    setEditing(false)
    adminApi.tickDefaults().then((value) => form.setFieldsValue(value)).catch(setDefaultsError)
  }

  return <div>
    <PageHeading eyebrow="人工介入入口" title="安全操作" />
    {loading ? <LoadingState /> : <div className="operations-grid">
      <section className="surface-panel operation-console">
        <div className="panel-heading"><div><h2>执行一次 Scheduler Tick</h2></div></div>
        {defaultsError ? <ErrorState error={defaultsError} /> : null}
        <Alert type="warning" showIcon message="Tick 可能派发 Agent、轮询 Run、恢复租约、对账写回或发送待处理通知。" />
        <Form form={form} layout="vertical" className="tick-form">
          <div className="limit-grid">
            {[
              ['dispatch_limit', '派发'], ['poll_limit', '轮询'], ['reconciliation_limit', '写回对账'],
              ['finalization_limit', '结果完成'], ['recovery_limit', '租约恢复'], ['effect_limit', '通知执行'],
            ].map(([name, label]) => <Form.Item key={name} name={name} label={`${label}上限`}><InputNumber min={0} precision={0} className="full-width" disabled={!editing} /></Form.Item>)}
          </div>
          <div className="tick-form-actions">
            {editing ? (
              <Space>
                <Button onClick={cancelEdit}>取消</Button>
                <Button type="primary" onClick={saveEdit}>保存</Button>
              </Space>
            ) : (
              <Space>
                <Button type="primary" icon={<PlayCircleOutlined />} loading={running} onClick={() => setConfirmOpen(true)}>执行一次 Tick</Button>
                <Button icon={<EditOutlined />} onClick={() => setEditing(true)}>编辑参数</Button>
              </Space>
            )}
          </div>
          <p className="form-note">此处保存仅对当次手动 Tick 生效；要让参数持久生效，请修改 .env 后重启服务。</p>
        </Form>
        {result ? <div className="tick-result"><div className="section-label">本次结果</div><div className="metric-matrix">{resultGroups.map((group) => <div key={group.key}><span>{group.label}</span><strong>{(result[group.key] ?? []).length} 个 {group.noun}</strong></div>)}</div></div> : null}
      </section>

      <section className="surface-panel">
        <div className="panel-heading"><div><h2>失败的 Effects</h2></div><Button icon={<RedoOutlined />} onClick={loadEffects}>刷新</Button></div>
        {effectsError ? <ErrorState error={effectsError} onRetry={loadEffects} /> : <Table
          size="small" rowKey={(row) => String(row.effect_id)} pagination={false} dataSource={effects}
          locale={{ emptyText: '当前没有待人工重试的 Effect' }}
          columns={[
            { title: 'Effect', dataIndex: 'effect_id', render: (value) => <span className="mono">{String(value)}</span> },
            { title: '类型', dataIndex: 'kind' },
            { title: '状态', dataIndex: 'status', render: (value) => <StatusTag status={String(value)} /> },
            { title: '失败原因', dataIndex: 'error_message', render: (value) => <span className="error-copy">{String(value || '未记录')}</span> },
            { title: '操作', render: (_, row) => <Button size="small" onClick={() => retry(String(row.effect_id))}>重新入队</Button> },
          ]}
        />}
      </section>
    </div>}
    <Modal title="确认执行 Scheduler Tick" open={confirmOpen} onCancel={() => setConfirmOpen(false)} onOk={runTick} okText="确认执行" cancelText="取消">
      <p>Coordinator 将按当前表单限制扫描内部状态并执行可推进动作。该操作不会绕过状态机。</p>
    </Modal>
  </div>
}
