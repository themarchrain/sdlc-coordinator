import { useEffect, useMemo, useState } from 'react'
import { AutoComplete, Button, Descriptions, Drawer, Form, Input, Select, Space, Table, Timeline } from 'antd'
import { useSearchParams } from 'react-router-dom'

import { adminApi } from '../api/client'
import type { TaskSummary } from '../api/types'
import { patchSearchParams, repositoryFilter } from '../app/searchParams'
import { ErrorState, LoadingState } from '../components/PageState'
import { PageHeading } from '../components/PageHeading'
import { StatusTag } from '../components/StatusTag'
import { TaskStateMachinePanel } from '../components/TaskStateMachinePanel'
import { AuditJson, TaskContextDetail } from '../components/StructuredDetail'
import type { TimelineSlots } from '../themes/types'
import { useTheme } from '../theme/useTheme'
import { useRemote } from '../hooks/useRemote'

const roles = ['DeveloperWorker', 'TesterWorker', 'ReviewerWorker']
const taskTypes = ['develop_from_issue', 'review_mr', 'rework_merge_request', 'upstream_fix', 'verify_merge_request', 'generated_from_next_task']
const taskStatuses = ['pending', 'claimed', 'running', 'finalizing', 'waiting_upstream', 'waiting_merge', 'retry_pending', 'human_required', 'blocked', 'succeeded']
const taskFilterKeys = ['status', 'role', 'type', 'repository'] as const

export function TasksPage() {
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [searchParams, setSearchParams] = useSearchParams()
  const [form] = Form.useForm()
  const filters = useMemo(() => Object.fromEntries(taskFilterKeys.flatMap((key) => {
    const value = searchParams.get(key)
    return value ? [[key, value]] : []
  })), [searchParams])
  const selectedID = searchParams.get('task_id') || undefined
  const repositoryID = searchParams.get('repository_id') || undefined
  const tasks = useRemote(() => adminApi.listTasks({ page, page_size: pageSize, ...filters, ...repositoryFilter(searchParams) }), [page, pageSize, JSON.stringify(filters), repositoryID])

  useEffect(() => {
    form.setFieldsValue(Object.fromEntries(taskFilterKeys.map((key) => [key, filters[key]])))
  }, [form, filters])

  useEffect(() => {
    setPage(1)
  }, [repositoryID])

  function submitFilters(values: Record<string, unknown>) {
    const cleaned = cleanValues(values)
    setPage(1)
    setSearchParams(patchSearchParams(searchParams, Object.fromEntries(taskFilterKeys.map((key) => [key, cleaned[key]]))))
  }

  function clearFilters() {
    setPage(1)
    form.resetFields()
    setSearchParams(patchSearchParams(searchParams, Object.fromEntries(taskFilterKeys.map((key) => [key, undefined]))))
  }

  return (
    <div>
      <PageHeading eyebrow="协调任务状态" title="任务轨迹" />
      <section className="surface-panel filter-panel">
        <Form form={form} layout="inline" onFinish={submitFilters}>
          <Form.Item name="status"><Select allowClear placeholder="任务状态" options={taskStatuses.map((value) => ({ value, label: optionLabel(value) }))} style={{ width: 180 }} /></Form.Item>
          <Form.Item name="role"><Select allowClear placeholder="Worker 角色" options={roles.map((value) => ({ value }))} style={{ width: 180 }} /></Form.Item>
          <Form.Item name="type"><AutoComplete allowClear placeholder="任务类型" options={taskTypes.map((value) => ({ value, label: optionLabel(value) }))} filterOption={(input, option) => String(option?.value ?? '').includes(input) || String(option?.label ?? '').includes(input)} style={{ width: 220 }} /></Form.Item>
          <Form.Item name="repository"><Input allowClear placeholder="仓库路径" /></Form.Item>
          <Form.Item><Space><Button type="primary" htmlType="submit">查询</Button><Button onClick={clearFilters}>清空</Button></Space></Form.Item>
        </Form>
      </section>

      <section className="surface-panel table-panel">
        {tasks.loading ? <LoadingState /> : tasks.error ? <ErrorState error={tasks.error} onRetry={tasks.reload} /> : (
          <Table<TaskSummary>
            className="admin-table admin-table--row-click admin-table--wide"
            rowKey="task_id"
            size="middle"
            tableLayout="fixed"
            scroll={{ x: 1620 }}
            dataSource={tasks.data?.items ?? []}
            onRow={(row) => ({ onClick: () => setSearchParams(patchSearchParams(searchParams, { task_id: String(row.task_id) })) })}
            pagination={{ className: 'table-pagination', current: page, pageSize, total: tasks.data?.total ?? 0, showSizeChanger: true, pageSizeOptions: [10, 15, 20, 50], showLessItems: true, showTotal: (total) => `共 ${total} 条`, onChange: setPage, onShowSizeChange: (_current, size) => { setPageSize(size); setPage(1) } }}
            columns={[
              { title: 'Task', dataIndex: 'task_id', width: 270, render: (value) => <span className="mono table-cell-wrap">{String(value || '—')}</span> },
              { title: '状态', dataIndex: 'status', width: 140, render: (value) => <StatusTag status={String(value)} /> },
              { title: '角色 / 类型', width: 210, render: (_, row) => <div><strong>{row.role}</strong><small className="table-secondary">{row.type}</small></div> },
              { title: '仓库', width: 280, render: (_, row) => {
                const source = row.source_repo ? String(row.source_repo) : ''
                const target = row.target_repo ? String(row.target_repo) : ''
                if (!source && !target) return <span className="dim">—</span>
                if (!target || source === target) return <span className="mono table-repo table-cell-wrap">{source || target}</span>
                return <div className="mono table-repo table-cell-wrap">{source}<span>→ {target}</span></div>
              } },
              { title: '当前 Run', dataIndex: 'current_run_id', width: 270, render: (value) => <span className="mono table-cell-wrap">{String(value || '—')}</span> },
              { title: '错误摘要', dataIndex: 'last_error', width: 270, ellipsis: true, render: (value) => <span className={value ? 'error-copy' : 'muted-copy'}>{String(value || '—')}</span> },
              { title: '更新时间', dataIndex: 'updated_at', width: 180, render: formatTime },
            ]}
          />
        )}
      </section>
      <TaskDrawer taskID={selectedID} onClose={() => setSearchParams(patchSearchParams(searchParams, { task_id: undefined }))} />
    </div>
  )
}

function TaskDrawer({ taskID, onClose }: { taskID?: string; onClose: () => void }) {
  const { theme } = useTheme()
  const request = useRemote(async () => {
    const [task, events] = await Promise.all([adminApi.getTask(taskID!), adminApi.taskTimeline(taskID!)])
    return { taskID, task, timeline: events.items, loadedAt: new Date().toISOString() }
  }, [taskID], Boolean(taskID))
  const snapshot = request.data?.taskID === taskID ? request.data : undefined
  const detail = snapshot?.task

  return (
    <Drawer title={<span className="mono">{taskID}</span>} width="min(860px, 100vw)" open={Boolean(taskID)} onClose={onClose} destroyOnClose>
      <div className="task-detail-toolbar">
        <Button size="small" onClick={request.reload}>刷新</Button>
        {snapshot ? <time dateTime={snapshot.loadedAt}>读取于 {formatTime(snapshot.loadedAt)}</time> : null}
      </div>
      {request.loading || (!snapshot && !request.error && taskID) ? <LoadingState /> : request.error ? <ErrorState error={request.error} onRetry={request.reload} /> : detail && snapshot ? (
        <div className="drawer-stack">
          <Descriptions size="small" column={{ xs: 1, sm: 2 }} bordered items={[
            { key: 'status', label: '状态', children: <StatusTag status={String(detail.status || '')} /> },
            { key: 'role', label: '角色', children: String(detail.role || '—') },
            { key: 'type', label: '类型', children: String(detail.type || '—') },
            { key: 'run', label: '当前 Run', children: <span className="mono">{String(detail.current_run_id || '—')}</span> },
            { key: 'objective', label: '目标', span: { xs: 1, sm: 2 }, children: String(detail.objective || '—') },
            { key: 'acceptance', label: '验收标准', span: { xs: 1, sm: 2 }, children: String(detail.acceptance_criteria || '—') },
            { key: 'error', label: '错误', span: { xs: 1, sm: 2 }, children: <span className="error-copy">{String(detail.last_error || '—')}</span> },
          ]} />
          <TaskStateMachinePanel status={String(detail.status || '')} />
          <TaskContextDetail value={detail.context_json} />
          <section><div className="section-label">生命周期</div><Timeline items={snapshot.timeline.map((event) => ({
            color: timelineColor(event.status, theme.slots.timeline),
            children: <div className="timeline-entry"><div><strong>{event.title}</strong>{event.status ? <StatusTag status={event.status} /> : null}</div><p>{event.summary || '无补充摘要'}</p><time>{formatTime(event.occurred_at)}</time></div>,
          }))} /></section>
          <AuditJson label="原始任务数据" value={detail} />
        </div>
      ) : null}
    </Drawer>
  )
}

function cleanValues(values: Record<string, unknown>) {
  return Object.fromEntries(Object.entries(values).filter(([, value]) => value).map(([key, value]) => [key, String(value)]))
}

function formatTime(value: unknown) {
  if (!value) return '—'
  return new Date(String(value)).toLocaleString('zh-CN', { hour12: false })
}

function timelineColor(status: string, slots: TimelineSlots) {
  if (['failed', 'blocked', 'unavailable'].includes(status)) return slots.error
  if (['human_required', 'retry_pending', 'waiting_upstream'].includes(status)) return slots.warning
  if (['succeeded', 'approved'].includes(status)) return slots.success
  return slots.info
}

function optionLabel(value: string) {
  return ({
    pending: '待派发', claimed: '已领取', running: '运行中', finalizing: '结果收口中', retry_pending: '等待重试', waiting_upstream: '等待上游',
    waiting_merge: '等待合并', human_required: '需要人工', blocked: '已阻塞', succeeded: '已成功', develop_from_issue: 'Issue 开发', review_mr: 'MR 审查',
    rework_merge_request: 'MR 接续开发', upstream_fix: '上游修复', verify_merge_request: 'MR 验证', generated_from_next_task: '派生任务',
    DeveloperWorker: '开发', TesterWorker: '验证', ReviewerWorker: '审查',
  } as Record<string, string>)[value] ?? value
}
