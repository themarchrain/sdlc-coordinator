import { useEffect, useMemo, useState } from 'react'
import { AutoComplete, Button, Drawer, Dropdown, Form, Input, Select, Space, Table } from 'antd'
import { CheckOutlined, DownOutlined } from '@ant-design/icons'
import { useSearchParams } from 'react-router-dom'

import { adminApi } from '../api/client'
import type { EntityKey } from '../api/types'
import { patchSearchParams, repositoryFilter } from '../app/searchParams'
import { EntityDetail } from '../components/StructuredDetail'
import { ErrorState, LoadingState } from '../components/PageState'
import { PageHeading } from '../components/PageHeading'
import { StatusTag } from '../components/StatusTag'
import { TermHint } from '../components/TermHint'
import { termHints } from '../content/term-hints'
import { useRemote } from '../hooks/useRemote'

interface FilterConfig {
  key: string
  kind: 'input' | 'select' | 'autocomplete'
  placeholder: string
  options?: string[]
  width?: number
}

interface EntityConfig {
  key: EntityKey
  label: string
  title: string
  id: string
  columns: string[]
  filters: FilterConfig[]
}

const workerRoles = ['DeveloperWorker', 'TesterWorker', 'ReviewerWorker']
const runStatuses = ['pending', 'running', 'finalizing', 'succeeded', 'failed', 'blocked']
const dependencyStatuses = ['pending', 'ready', 'blocked', 'resolved']
const writebackStatuses = ['pending', 'running', 'succeeded', 'failed', 'unknown']
const effectStatuses = ['pending', 'running', 'succeeded', 'failed', 'skipped']
const inboundEventStatuses = ['received', 'normalized', 'ignored', 'task_created', 'failed']
const resultSubmissionStatuses = ['processing', 'succeeded', 'failed', 'unknown']

const entities: EntityConfig[] = [
  {
    key: 'runs', label: '运行记录', title: '运行记录',
    id: 'run_id', columns: ['task_id', 'worker_role', 'status', 'external_system', 'created_at'],
    filters: [
      { key: 'task_id', kind: 'input', placeholder: 'Task ID', width: 260 },
      { key: 'worker_role', kind: 'select', placeholder: 'Worker 角色', options: workerRoles, width: 190 },
      { key: 'status', kind: 'select', placeholder: '执行状态', options: runStatuses, width: 180 },
    ],
  },
  {
    key: 'artifacts', label: '交付产物', title: '交付产物',
    id: 'artifact_id', columns: ['task_id', 'run_id', 'kind', 'name', 'created_at'],
    filters: [
      { key: 'task_id', kind: 'input', placeholder: 'Task ID', width: 260 },
      { key: 'run_id', kind: 'input', placeholder: 'Run ID', width: 260 },
      { key: 'kind', kind: 'select', placeholder: '产物类型', options: ['merge_request', 'issue', 'branch', 'diff', 'test_result', 'commit', 'generic'], width: 190 },
    ],
  },
  {
    key: 'dependencies', label: '任务依赖', title: '任务依赖',
    id: 'dependency_id', columns: ['parent_task_id', 'child_task_id', 'relation', 'status', 'created_at'],
    filters: [
      { key: 'parent_task_id', kind: 'input', placeholder: '上游 Task ID', width: 260 },
      { key: 'child_task_id', kind: 'input', placeholder: '下游 Task ID', width: 260 },
      { key: 'relation', kind: 'autocomplete', placeholder: '依赖关系', options: ['upstream_fix', 'blocks', 'depends_on', 'verify_after_dependency'], width: 210 },
      { key: 'status', kind: 'select', placeholder: '依赖状态', options: dependencyStatuses, width: 180 },
    ],
  },
  {
    key: 'writebacks', label: '外部写回', title: '外部写回',
    id: 'writeback_id', columns: ['task_id', 'kind', 'project_id', 'status', 'created_at'],
    filters: [
      { key: 'task_id', kind: 'input', placeholder: 'Task ID', width: 260 },
      { key: 'run_id', kind: 'input', placeholder: 'Run ID', width: 260 },
      { key: 'kind', kind: 'select', placeholder: '写回类型', options: ['create_issue', 'create_merge_request', 'create_issue_note', 'create_merge_request_note', 'set_issue_labels', 'set_merge_request_labels', 'add_merge_request_labels'], width: 230 },
      { key: 'status', kind: 'select', placeholder: '写回状态', options: writebackStatuses, width: 180 },
    ],
  },
  {
    key: 'effects', label: '通知与副作用', title: '通知与副作用',
    id: 'effect_id', columns: ['task_id', 'kind', 'target', 'status', 'created_at'],
    filters: [
      { key: 'task_id', kind: 'input', placeholder: 'Task ID', width: 260 },
      { key: 'run_id', kind: 'input', placeholder: 'Run ID', width: 260 },
      { key: 'kind', kind: 'select', placeholder: 'Effect 类型', options: ['send_im_message'], width: 190 },
      { key: 'status', kind: 'select', placeholder: '执行状态', options: effectStatuses, width: 180 },
    ],
  },
  {
    key: 'inbound-events', label: '入站事件', title: '入站事件',
    id: 'event_id', columns: ['source', 'event_type', 'event_action', 'status', 'received_at'],
    filters: [
      { key: 'source', kind: 'select', placeholder: '事件来源', options: ['gitlab'], width: 170 },
      { key: 'type', kind: 'select', placeholder: '事件类型', options: ['issue', 'merge_request', 'note', 'push'], width: 190 },
      { key: 'status', kind: 'select', placeholder: '处理状态', options: inboundEventStatuses, width: 180 },
    ],
  },
  {
    key: 'result-submissions', label: '结果回传', title: '结果回传',
    id: 'submission_id', columns: ['task_id', 'run_id', 'source', 'role', 'status'],
    filters: [
      { key: 'task_id', kind: 'input', placeholder: 'Task ID', width: 260 },
      { key: 'run_id', kind: 'input', placeholder: 'Run ID', width: 260 },
      { key: 'source', kind: 'autocomplete', placeholder: '回传来源', options: ['mcp', 'http_callback', 'polling'], width: 220 },
      { key: 'role', kind: 'select', placeholder: 'Worker 角色', options: workerRoles, width: 190 },
      { key: 'status', kind: 'select', placeholder: '处理状态', options: resultSubmissionStatuses, width: 180 },
    ],
  },
]

const columnLabels: Record<string, string> = {
  artifact_id: 'Artifact ID', child_task_id: '下游 Task', created_at: '创建时间', dependency_id: 'Dependency ID', effect_id: 'Effect ID',
  event_action: '事件动作', event_id: 'Event ID', event_type: '事件类型', external_system: '外部系统', kind: '类型', name: '名称',
  parent_task_id: '上游 Task', project_id: '项目 ID', received_at: '接收时间', relation: '依赖关系', role: 'Worker 角色',
  run_id: 'Run ID', source: '来源', status: '状态', submission_id: 'Submission ID', target: '目标', task_id: 'Task ID',
  worker_role: 'Worker 角色', writeback_id: 'Writeback ID',
}
const entityKeys = new Set(entities.map(({ key }) => key))
const explorerParamKeys = [...new Set(entities.flatMap(({ id, filters }) => [id, ...filters.map(({ key }) => key)]))]

export function DataExplorerPage() {
  const [form] = Form.useForm()
  const [searchParams, setSearchParams] = useSearchParams()
  const requestedEntity = searchParams.get('entity')
  const entity = entityKeys.has(requestedEntity as EntityKey) ? requestedEntity as EntityKey : 'runs'
  useEffect(() => {
    if (requestedEntity !== null && !entityKeys.has(requestedEntity as EntityKey)) {
      setSearchParams(patchSearchParams(searchParams, { entity: 'runs' }))
    }
  }, [requestedEntity, searchParams, setSearchParams])
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const config = entities.find((item) => item.key === entity)!
  const filters = useMemo(() => Object.fromEntries(config.filters.flatMap(({ key }) => {
    const value = searchParams.get(key)
    return value ? [[key, value]] : []
  })), [config, searchParams])
  const selectedID = searchParams.get(config.id) || undefined
  const repositoryID = searchParams.get('repository_id') || undefined
  const data = useRemote(() => adminApi.listEntities(entity, { page, page_size: pageSize, ...filters, ...repositoryFilter(searchParams) }), [entity, page, pageSize, JSON.stringify(filters), repositoryID])

  useEffect(() => {
    form.resetFields()
    form.setFieldsValue(filters)
  }, [entity, filters, form])

  useEffect(() => {
    setPage(1)
  }, [repositoryID])

  const switchEntity = (key: string) => {
    setPage(1)
    form.resetFields()
    setSearchParams(patchSearchParams(searchParams, {
      ...Object.fromEntries(explorerParamKeys.map((param) => [param, undefined])),
      entity: key,
    }))
  }

  const submitFilters = (values: Record<string, unknown>) => {
    const cleaned = cleanValues(values)
    setPage(1)
    setSearchParams(patchSearchParams(searchParams, Object.fromEntries(config.filters.map(({ key }) => [key, cleaned[key]]))))
  }

  const clearFilters = () => {
    setPage(1)
    form.resetFields()
    setSearchParams(patchSearchParams(searchParams, Object.fromEntries(config.filters.map(({ key }) => [key, undefined]))))
  }

  const openDetail = (id: string) => setSearchParams(patchSearchParams(searchParams, { [config.id]: id }))

  return (
    <div>
      <PageHeading eyebrow="协调数据查询" title={config.title} />
      <section className="surface-panel explorer-tabs">
        <div className="explorer-nav">
          <div className="explorer-nav__tabs" role="tablist" aria-label="数据记录入口">
            <button type="button" role="tab" aria-selected={entity === 'runs'} className={entity === 'runs' ? 'explorer-tab is-active' : 'explorer-tab'} onClick={() => switchEntity('runs')}>运行记录</button>
            {entity !== 'runs' ? <button type="button" role="tab" aria-selected className="explorer-tab is-active">{config.label}</button> : null}
          </div>
          {termHints[config.label] ? <TermHint title={config.label} content={termHints[config.label]} /> : null}
          <Dropdown
            trigger={['click']}
            menu={{
              items: entities.filter(({ key }) => key !== 'runs').map(({ key, label }) => ({ key, label, icon: entity === key ? <CheckOutlined /> : undefined })),
              onClick: ({ key }) => switchEntity(key),
              selectable: true,
              selectedKeys: entity === 'runs' ? [] : [entity],
            }}
          >
            <button type="button" className="explorer-more">更多记录 <DownOutlined /></button>
          </Dropdown>
        </div>
        <Form form={form} layout="inline" onFinish={submitFilters}>
          {config.filters.map((filter) => (
            <Form.Item name={filter.key} key={filter.key}>
              {filter.kind === 'select' ? (
                <Select allowClear showSearch optionFilterProp="label" placeholder={filter.placeholder} options={filter.options?.map((value) => ({ value, label: optionLabel(value) }))} style={{ width: filter.width }} />
              ) : filter.kind === 'autocomplete' ? (
                <AutoComplete allowClear placeholder={filter.placeholder} options={filter.options?.map((value) => ({ value, label: optionLabel(value) }))} filterOption={(input, option) => String(option?.value ?? '').includes(input) || String(option?.label ?? '').includes(input)} style={{ width: filter.width }} />
              ) : <Input allowClear placeholder={filter.placeholder} style={{ width: filter.width }} />}
            </Form.Item>
          ))}
          <Form.Item><Space><Button type="primary" htmlType="submit">查询</Button><Button onClick={clearFilters}>清空</Button></Space></Form.Item>
        </Form>
      </section>
      <section className="surface-panel table-panel">
        {data.loading ? <LoadingState /> : data.error ? <ErrorState error={data.error} onRetry={data.reload} /> : (
          <Table
            className="admin-table admin-table--row-click admin-table--wide"
            rowKey={(row) => String(row[config.id])}
            size="middle"
            tableLayout="fixed"
            scroll={{ x: Math.max(1180, 260 + config.columns.length * 200) }}
            dataSource={data.data?.items ?? []}
            onRow={(row) => ({ onClick: () => openDetail(String(row[config.id])) })}
            locale={config.key === 'runs' ? { emptyText: '当前没有匹配的执行记录，试试清空筛选后重试。' } : undefined}
            pagination={{ className: 'table-pagination', current: page, pageSize, total: data.data?.total ?? 0, showSizeChanger: true, pageSizeOptions: [10, 15, 20, 50], showLessItems: true, showTotal: (total) => `共 ${total} 条`, onChange: setPage, onShowSizeChange: (_current, size) => { setPageSize(size); setPage(1) } }}
            columns={entity === 'runs'
              ? [
                { title: <span className="explorer-column-head">角色{termHints['角色'] ? <TermHint title="角色" content={termHints['角色']} /> : null}</span>, dataIndex: 'worker_role', width: 160, render: (value: unknown) => <span className="mono">{String(value || '—')}</span> },
                { title: '状态', dataIndex: 'status', width: 120, render: (value: unknown) => <StatusTag status={String(value)} /> },
                { title: '任务', dataIndex: 'task_id', width: 240, ellipsis: true, render: (value: unknown) => value ? <span className="mono">{String(value)}</span> : <span className="dim">—</span> },
                { title: '会话', dataIndex: 'external_id', width: 220, ellipsis: true, render: (value: unknown, row) => sessionLink(value, row) },
                { title: '开始时间', dataIndex: 'started_at', width: 180, render: (value: unknown) => formatTime(value) },
                { title: '耗时', key: 'duration', width: 110, render: (_: unknown, row) => formatDuration(row.started_at, row.completed_at) },
              ]
              : [
                { title: columnLabels[config.id] ?? config.id, dataIndex: config.id, width: 260, render: (value: unknown) => value ? <span className="mono">{String(value)}</span> : <span className="dim">—</span> },
                ...config.columns.map((key) => ({
                  title: columnLabels[key] ?? humanize(key), dataIndex: key, width: key.endsWith('_at') ? 190 : 200, ellipsis: true,
                  render: (value: unknown) => key === 'status' ? <StatusTag status={String(value)} /> : key.endsWith('_at') ? formatTime(value) : <span className={key.endsWith('_id') ? 'mono' : ''}>{String(value ?? '—')}</span>,
                })),
              ]}
          />
        )}
      </section>
      <EntityDrawer config={config} id={selectedID} onClose={() => setSearchParams(patchSearchParams(searchParams, { [config.id]: undefined }))} />
    </div>
  )
}

function EntityDrawer({ config, id, onClose }: { config: EntityConfig; id?: string; onClose: () => void }) {
  const [detail, setDetail] = useState<Record<string, unknown>>()
  const [error, setError] = useState<unknown>()
  const [loading, setLoading] = useState(false)
  useEffect(() => {
    if (!id) return
    setLoading(true)
    setDetail(undefined)
    setError(undefined)
    adminApi.getEntity(config.key, id).then(setDetail).catch(setError).finally(() => setLoading(false))
  }, [config.key, id])
  return <Drawer width="min(860px, 100vw)" title={<div><span>{config.title}</span><small className="drawer-title-id mono">{id}</small></div>} open={Boolean(id)} onClose={onClose} destroyOnClose>
    {loading ? <LoadingState /> : error ? <ErrorState error={error} /> : detail ? <>
      {config.key === 'runs' ? <RunSummary detail={detail} /> : null}
      <EntityDetail entity={config.key} detail={detail} />
    </> : null}
  </Drawer>
}

function cleanValues(values: Record<string, unknown>) {
  return Object.fromEntries(Object.entries(values).filter(([, value]) => value).map(([key, value]) => [key, String(value)]))
}

function sessionLink(value: unknown, row: Record<string, unknown>) {
  if (row.external_url) {
    return <a className="mono" href={String(row.external_url)} target="_blank" rel="noreferrer">{value ? String(value) : '打开会话'}</a>
  }
  return value ? <span className="mono">{String(value)}</span> : <span className="dim">—</span>
}

function formatDuration(started?: unknown, completed?: unknown) {
  if (!started || !completed) return '—'
  const start = new Date(String(started)).getTime()
  const end = new Date(String(completed)).getTime()
  if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) return '—'
  const ms = end - start
  if (ms < 60000) return `${(ms / 1000).toFixed(1)} 秒`
  return `${(ms / 60000).toFixed(1)} 分钟`
}

function RunSummary({ detail }: { detail: Record<string, unknown> }) {
  return <div className="run-summary">
    <span><StatusTag status={String(detail.status ?? '')} /></span>
    <span>角色 {String(detail.worker_role ?? '—')}</span>
    <span>系统 {String(detail.external_system ?? '—')}</span>
    {detail.started_at ? <span>{formatTime(detail.started_at)} → {formatTime(detail.completed_at)}</span> : null}
    <span className="run-summary__duration">耗时 {formatDuration(detail.started_at, detail.completed_at)}</span>
  </div>
}

function humanize(value: string) {
  return value.replaceAll('_', ' ').replace(/\b\w/g, (letter) => letter.toUpperCase())
}

function formatTime(value: unknown) {
  if (!value) return '—'
  return new Date(String(value)).toLocaleString('zh-CN', { hour12: false })
}

function optionLabel(value: string) {
  return ({
    pending: '待处理', received: '已接收', normalized: '已规范化', task_created: '已创建 Task', running: '运行中', finalizing: '结果收口中',
    processing: '处理中', ready: '已就绪', resolved: '已解决', blocked: '已阻塞', unknown: '结果未知', succeeded: '已成功', failed: '已失败',
    ignored: '已忽略', skipped: '已跳过', develop_from_issue: 'Issue 开发', review_mr: 'MR 审查', rework_merge_request: 'MR 接续开发',
    verify_merge_request: 'MR 验证', DeveloperWorker: '开发', TesterWorker: '验证', ReviewerWorker: '审查',
    merge_request: 'Merge Request', issue: 'Issue', branch: '分支', diff: 'Diff', test_result: '测试结果', commit: 'Commit', generic: '其他',
    send_im_message: '发送 IM 消息', create_issue: '创建 Issue', create_merge_request: '创建 MR',
    create_issue_note: '创建 Issue 评论', create_merge_request_note: '创建 MR 评论', set_issue_labels: '设置 Issue 标签',
    set_merge_request_labels: '设置 MR 标签', add_merge_request_labels: '追加 MR 标签', blocks: '阻塞', depends_on: '依赖',
    upstream_fix: '上游修复', verify_after_dependency: '依赖完成后验证', gitlab: 'GitLab', note: '评论', push: 'Push',     mcp: 'MCP', http_callback: 'HTTP Callback', polling: 'Polling', agent_runner: 'Agent Runner',
  } as Record<string, string>)[value] ?? value
}
