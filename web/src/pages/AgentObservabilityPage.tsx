import { useEffect, useMemo, useState } from 'react'
import { Button, Descriptions, Drawer, Form, Input, Select, Space, Table, Timeline } from 'antd'
import { Link, useSearchParams } from 'react-router-dom'

import { adminApi } from '../api/client'
import type { AgentExecution, AgentExecutionDetail, AgentExecutionEvent, Query } from '../api/types'
import { patchSearchParams } from '../app/searchParams'
import { PageHeading } from '../components/PageHeading'
import { ErrorState, LoadingState } from '../components/PageState'
import { StatusTag } from '../components/StatusTag'
import type { EventTypeSlots } from '../themes/types'
import { useTheme } from '../theme/useTheme'
import { useRemote } from '../hooks/useRemote'

const statuses = ['accepted', 'starting', 'running', 'succeeded', 'failed', 'cancelled', 'interrupted']
const roles = ['ReviewerWorker', 'DeveloperWorker']
const providers = ['codex-cli', 'claude-cli']
const filterKeys = ['provider', 'status', 'role', 'task_id', 'run_id'] as const

export function AgentObservabilityPage() {
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [searchParams, setSearchParams] = useSearchParams()
  const [form] = Form.useForm()
  const filters = useMemo(() => Object.fromEntries(filterKeys.flatMap((key) => {
    const value = searchParams.get(key)
    return value ? [[key, value]] : []
  })) as Query, [searchParams])
  const selectedID = searchParams.get('execution_id') || searchParams.get('run_id') || undefined
  const summary = useRemote(() => adminApi.agentExecutionSummary(filters), [JSON.stringify(filters)])
  const executions = useRemote(() => adminApi.listAgentExecutions({ ...filters, page, page_size: pageSize }), [JSON.stringify(filters), page, pageSize])

  useEffect(() => {
    form.setFieldsValue(Object.fromEntries(filterKeys.map((key) => [key, filters[key]])))
  }, [filters, form])

  function submit(values: Record<string, unknown>) {
    setPage(1)
    setSearchParams(patchSearchParams(searchParams, Object.fromEntries(filterKeys.map((key) => [key, values[key] ? String(values[key]).trim() : undefined]))))
  }

  function clear() {
    setPage(1)
    form.resetFields()
    setSearchParams(patchSearchParams(searchParams, Object.fromEntries([...filterKeys.map((key) => [key, undefined]), ['execution_id', undefined]])))
  }

  return <div>
    <PageHeading eyebrow="Agent 执行观测" title="Agent 运行" action={<Button onClick={() => { summary.reload(); executions.reload() }}>刷新快照</Button>} />
    {summary.loading ? <LoadingState /> : summary.error ? <ErrorState error={summary.error} onRetry={summary.reload} /> : summary.data ? <ExecutionSummaryPanel data={summary.data} /> : null}

    <section className="surface-panel filter-panel codex-filter-panel">
      <Form form={form} layout="inline" onFinish={submit}>
        <Form.Item name="provider"><Select allowClear placeholder="Provider（全部）" options={providers.map((value) => ({ value }))} style={{ width: 180 }} /></Form.Item>
        <Form.Item name="status"><Select allowClear placeholder="执行状态" options={statuses.map((value) => ({ value, label: statusLabel(value) }))} style={{ width: 170 }} /></Form.Item>
        <Form.Item name="role"><Select allowClear placeholder="Worker 角色" options={roles.map((value) => ({ value }))} style={{ width: 180 }} /></Form.Item>
        <Form.Item name="task_id"><Input allowClear placeholder="Task ID" /></Form.Item>
        <Form.Item name="run_id"><Input allowClear placeholder="Run ID" /></Form.Item>
        <Form.Item><Space><Button type="primary" htmlType="submit">查询</Button><Button onClick={clear}>清空</Button></Space></Form.Item>
      </Form>
    </section>

    <section className="surface-panel table-panel">
      {executions.loading ? <LoadingState /> : executions.error ? <ErrorState error={executions.error} onRetry={executions.reload} /> : <Table<AgentExecution>
        className="admin-table admin-table--row-click"
        rowKey="execution_id"
        dataSource={executions.data?.items ?? []}
        onRow={(row) => ({ onClick: () => setSearchParams(patchSearchParams(searchParams, { execution_id: row.execution_id })) })}
        pagination={{ current: page, pageSize, total: executions.data?.total ?? 0, showSizeChanger: true, pageSizeOptions: [10, 20, 50], showTotal: (total) => `共 ${total} 条`, onChange: setPage, onShowSizeChange: (_current, size) => { setPage(1); setPageSize(size) } }}
        scroll={{ x: 1290 }}
        columns={[
          { title: 'Execution', dataIndex: 'execution_id', width: 260, render: (value) => <span className="mono table-cell-wrap">{value}</span> },
          { title: '状态', dataIndex: 'status', width: 120, render: (value) => <StatusTag status={value} /> },
          { title: 'Provider', dataIndex: 'provider', width: 110, render: (value) => <span className="mono">{value || '—'}</span> },
          { title: '职责', width: 190, render: (_, row) => <div><strong>{row.role}</strong><small className="table-secondary">{taskTypeLabel(row.task_type)}</small></div> },
          { title: '模型', width: 180, render: (_, row) => <div><span className="mono">{row.model || '—'}</span><small className="table-secondary">{row.effort || '—'}</small></div> },
          { title: '仓库', dataIndex: 'repository', width: 220, render: (value) => <span className="mono table-cell-wrap">{value || '—'}</span> },
          { title: '耗时', dataIndex: 'duration_seconds', width: 100, render: formatDuration },
          { title: '启动时间', dataIndex: 'started_at', width: 180, render: formatTime },
        ]}
      />}
    </section>
    <AgentExecutionDrawer executionID={selectedID} onClose={() => setSearchParams(patchSearchParams(searchParams, { execution_id: undefined, run_id: undefined }))} />
  </div>
}

function ExecutionSummaryPanel({ data }: { data: import('../api/types').AgentExecutionSummary }) {
  const success = data.total_executions ? (data.status_counts.succeeded ?? 0) / data.total_executions : null
  const cache = data.cache_hit_ratio
  return <section className="surface-panel codex-summary">
    <div className="codex-summary__facts">
      <Metric label="执行总数" value={formatNumber(data.total_executions)} note={`${data.turn_count} 个模型回合`} />
      <Metric label="成功占比" value={formatPercent(success)} note={`${data.status_counts.succeeded ?? 0} 次成功`} />
      <Metric label="平均耗时" value={formatDuration(data.average_duration_seconds)} note={`P50 ${formatDuration(data.p50_duration_seconds)}`} />
      <Metric label="长尾耗时" value={formatDuration(data.p90_duration_seconds)} note="成功与失败执行均计入" />
      <Metric label="缓存命中" value={formatPercent(cache)} note={`${formatNumber(data.cached_input_tokens)} 个缓存 Token`} />
      <Metric label="异常执行" value={formatNumber(data.error_count)} note={`${data.timeout_count} 次超时`} />
    </div>
    <div className="codex-token-ledger" aria-label="Token 构成">
      <div><span>输入 Token</span><strong>{formatNumber(data.input_tokens)}</strong></div>
      <div><span>其中缓存</span><strong>{formatNumber(data.cached_input_tokens)}</strong></div>
      <div><span>输出 Token</span><strong>{formatNumber(data.output_tokens)}</strong></div>
      <div className="codex-token-ledger__track" aria-hidden="true"><i style={{ width: `${Math.min(100, (cache ?? 0) * 100)}%` }} /></div>
    </div>
  </section>
}

function Metric({ label, value, note }: { label: string; value: string; note: string }) {
  return <div className="codex-metric"><span>{label}</span><strong>{value}</strong><small>{note}</small></div>
}

function AgentExecutionDrawer({ executionID, onClose }: { executionID?: string; onClose: () => void }) {
  const detail = useRemote(() => adminApi.getAgentExecution(executionID!), [executionID], Boolean(executionID))
  return <Drawer width="min(820px, 100vw)" open={Boolean(executionID)} onClose={onClose} destroyOnClose title={<span className="mono">{executionID}</span>}>
    {detail.loading ? <LoadingState /> : detail.error ? <ErrorState error={detail.error} onRetry={detail.reload} /> : detail.data ? <ExecutionDetailBody data={detail.data} /> : null}
  </Drawer>
}

function ExecutionDetailBody({ data }: { data: AgentExecutionDetail }) {
  const { theme } = useTheme()
  const usage = executionUsage(data.events)
  return <div className="drawer-stack">
    <div className="detail-actions"><Link to={`/tasks?task_id=${encodeURIComponent(data.task_id)}`}>查看 Task</Link><Link to={`/explorer?entity=runs&run_id=${encodeURIComponent(data.run_id)}`}>查看 Run</Link></div>
    <Descriptions size="small" bordered column={{ xs: 1, sm: 2 }} items={[
      { key: 'status', label: '状态', children: <StatusTag status={data.status} /> },
      { key: 'provider', label: 'Provider', children: <span className="mono">{data.provider || '—'}</span> },
      { key: 'role', label: '职责', children: data.role },
      { key: 'model', label: '模型', children: <span className="mono">{data.model || '—'}</span> },
      { key: 'effort', label: '推理强度', children: data.effort || '—' },
      { key: 'repository', label: '仓库', span: { xs: 1, sm: 2 }, children: <span className="mono">{data.repository || '—'}</span> },
      { key: 'duration', label: '耗时', children: formatDuration(data.duration_seconds) },
      { key: 'tokens', label: 'Token 消耗', span: { xs: 1, sm: 2 }, children: usage ? `输入 ${formatNumber(usage.input)} · 其中缓存 ${formatNumber(usage.cached)} · 输出 ${formatNumber(usage.output)}` : '—' },
      { key: 'cost', label: '估算费用', children: usage?.cost === undefined ? '—' : `$${usage.cost.toFixed(4)}` },
      { key: 'error', label: '错误', children: data.error_code || '—' },
    ]} />
    <section><div className="section-label">执行事件</div><Timeline items={data.events.map((event) => ({
      color: eventTypeColor(event.type, theme.slots.eventType),
      children: <div className="timeline-entry"><div><strong>{event.type}</strong><span className="mono">#{event.seq}</span></div><p>{eventSummary(event.payload)}</p><time>{formatTime(event.occurred_at)}</time></div>,
    }))} /></section>
  </div>
}

export function eventTypeColor(type: string, slots: EventTypeSlots) {
  return type === 'codex_error' || type === 'claude_error'
    ? slots.error
    : type === 'codex_usage' || type === 'claude_usage'
      ? slots.usage
      : slots.info
}

function eventSummary(payload: Record<string, unknown>) {
  const values = Object.entries(payload).map(([key, value]) => `${key}: ${formatPayloadValue(value)}`)
  return values.length ? values.join(' · ') : '无附加字段'
}

function formatPayloadValue(value: unknown) {
  return typeof value === 'object' && value !== null ? JSON.stringify(value) : String(value)
}

function formatNumber(value: number) { return new Intl.NumberFormat('zh-CN').format(value ?? 0) }

type ExecutionUsage = { input: number; cached: number; output: number; cost?: number }

// claude 的终态 claude_result 携带整会话累计 usage（部分兼容中转的中间
// claude_usage 全是零）；有终态取终态，否则累加逐回合事件（codex 与兜底）。
// claude 口径与汇总页一致：缓存读/写在 Anthropic 语义里与 input_tokens
// 互斥，折入输入总数使「其中缓存」呈子集关系；codex 的 input 已含缓存。
function executionUsage(events: AgentExecutionEvent[]): ExecutionUsage | undefined {
  const result = [...events].reverse().find((event) => event.type === 'claude_result')
  const resultUsage = result && isRecord(result.payload.usage) ? result.payload.usage : undefined
  if (result && resultUsage) {
    return {
      input: toNumber(resultUsage.input_tokens) + toNumber(resultUsage.cache_read_input_tokens) + toNumber(resultUsage.cache_creation_input_tokens),
      cached: toNumber(resultUsage.cache_read_input_tokens),
      output: toNumber(resultUsage.output_tokens),
      cost: toNumber(result.payload.total_cost_usd) || undefined,
    }
  }
  const turnEvents = events.filter((event) => event.type === 'codex_usage' || event.type === 'claude_usage')
  if (!turnEvents.length) return undefined
  return turnEvents.reduce<ExecutionUsage>((total, event) => {
    const usage = isRecord(event.payload.usage) ? event.payload.usage : event.payload
    total.input += toNumber(usage.input_tokens) + toNumber(usage.cache_read_input_tokens) + toNumber(usage.cache_creation_input_tokens)
    total.cached += toNumber(usage.cache_read_input_tokens) + toNumber(usage.cached_input_tokens)
    total.output += toNumber(usage.output_tokens)
    return total
  }, { input: 0, cached: 0, output: 0 })
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function toNumber(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}
function formatPercent(value: number | null) { return value === null ? '—' : `${(value * 100).toFixed(1)}%` }
function formatDuration(value?: number) { return value === undefined || value === null ? '—' : value < 60 ? `${value.toFixed(1)} 秒` : `${(value / 60).toFixed(1)} 分钟` }
function formatTime(value?: string) { return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—' }
function taskTypeLabel(value: string) { return ({ review_mr: 'MR 审查', develop_from_issue: 'Issue 开发', rework_merge_request: 'MR 接续开发' } as Record<string, string>)[value] ?? value }
function statusLabel(value: string) { return ({ accepted: '已接收', starting: '启动中', running: '运行中', succeeded: '已成功', failed: '已失败', cancelled: '已取消', interrupted: '已中断' } as Record<string, string>)[value] ?? value }
