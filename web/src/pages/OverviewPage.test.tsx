import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'

import { adminApi } from '../api/client'
import type { AutomationMetrics, Connector, Overview, Page } from '../api/types'
import { OverviewPage } from './OverviewPage'

afterEach(() => vi.restoreAllMocks())

const overviewPayload: Overview = {
  tasks_by_status: { running: 2, human_required: 1 },
  runs_by_status: { running: 2, failed: 1 },
  writebacks_by_status: { succeeded: 4 },
  effects_by_status: { failed: 1 },
  recent_failures: [{ entity_type: 'run', entity_id: 'run-failed', task_id: 'task-1', status: 'failed', error: '模型调用失败', occurred_at: '2026-08-08T08:00:00Z' }],
  business_summary: {
    window: '30d', merge_requests_created: 8, reviews_completed: 10, reviewer_takeovers: 3,
    automation_success: { numerator: 9, denominator: 10, value: 0.9 }, average_successful_run_seconds: 150, human_required_tasks: 2,
    human_required_added_today: 1,
    daily_merge_requests_created: [],
    daily_reviews_completed: [],
    daily_reviewer_takeovers: [
      { date: '2026-08-12', count: 1 }, { date: '2026-08-13', count: 2 }, { date: '2026-08-14', count: 3 },
    ],
    daily_average_run_seconds: [],
    generated_at: '2026-08-08T08:01:00Z',
  },
  generated_at: '2026-08-08T08:01:00Z',
}

const connectorsPayload: Page<Connector> = {
  items: [{
    connector_id: 'connector-1', kind: 'state_source_sync', display_name: '状态源同步器', base_url: 'http://sync:8989', enabled: true,
    target_project_path: 'group/repo', capabilities: [], last_health_status: 'unavailable', last_health_message: '连接超时',
    created_at: '2026-08-08T07:00:00Z', updated_at: '2026-08-08T08:00:00Z',
  }],
  page: 1, page_size: 20, total: 1,
}

const metricsPayload: AutomationMetrics = {
  window: '30d', type: '', task_sample_count: 5,
  current_success: { numerator: 3, denominator: 5, value: 0.6 },
  first_run_success: { numerator: 2, denominator: 4, value: 0.5 },
  intervention_triggered: { numerator: 2, denominator: 5, value: 0.4 },
  recovered_after_block: { numerator: 1, denominator: 2, value: 0.5 },
  successful_run_durations: [
    { worker_role: 'DeveloperWorker', sample_count: 2, average_seconds: 90, p50_seconds: 60, p90_seconds: 120 },
    { worker_role: 'ReviewerWorker', sample_count: 1, average_seconds: 30, p50_seconds: 30, p90_seconds: 30 },
  ],
  daily_series: [
    { date: '2026-08-12', current_success: 0.5, first_run_success: 0.5, intervention_triggered: 0.3, recovered_after_block: null },
    { date: '2026-08-13', current_success: 0.6, first_run_success: 0.5, intervention_triggered: 0.2, recovered_after_block: 0.5 },
    { date: '2026-08-14', current_success: 0.7, first_run_success: 0.6, intervention_triggered: 0.2, recovered_after_block: 0.6 },
  ],
  generated_at: '2026-08-08T08:01:00Z',
}

function mockOverview() {
  vi.spyOn(adminApi, 'overview').mockResolvedValue(overviewPayload)
  vi.spyOn(adminApi, 'listConnectors').mockResolvedValue(connectorsPayload)
  vi.spyOn(adminApi, 'metrics').mockResolvedValue(metricsPayload)
}

describe('OverviewPage', () => {
  it('shows actionable counts, recent failures, and connector health', async () => {
    mockOverview()

    const { container } = render(<MemoryRouter><OverviewPage /></MemoryRouter>)

    expect(await screen.findByText('模型调用失败')).toBeInTheDocument()
    expect(screen.getByText('已创建 MR')).toBeInTheDocument()
    expect(screen.getByText('已完成 CR')).toBeInTheDocument()
    expect(screen.getByText('已发起 CR')).toBeInTheDocument()
    expect(screen.getByLabelText('每日发起 CR 趋势')).toBeInTheDocument()
    expect(screen.queryByText('已接管 Issue')).not.toBeInTheDocument()
    expect(screen.queryByText('自动化成功率')).not.toBeInTheDocument()
    expect(screen.getByText('成功运行平均耗时')).toBeInTheDocument()
    expect(screen.getByText('待人工处理')).toBeInTheDocument()
    expect(screen.getByText('2 分 30 秒')).toBeInTheDocument()
    expect(screen.queryByText('任务（Tasks）')).not.toBeInTheDocument()
    expect(screen.queryByText('human_required')).not.toBeInTheDocument()
    expect(screen.getByText('状态源同步器')).toBeInTheDocument()
    expect(screen.getByText('连接超时')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '业务处理量' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '自动化效果' })).toBeInTheDocument()
    expect(screen.getByText('当前任务成功占比')).toBeInTheDocument()
    expect(screen.getByText('60.0%')).toBeInTheDocument()
    expect(screen.getByText('3 / 5')).toBeInTheDocument()
    expect(screen.getByText('成功的 Run · Task 样本 5')).toBeInTheDocument()
    expect(screen.getByText('DeveloperWorker')).toBeInTheDocument()
    expect(screen.getByText('P90 2 分钟')).toBeInTheDocument()
    expect(screen.getAllByText('样本较少').length).toBeGreaterThan(0)
    expect(screen.queryByText('AUTOMATION SIGNAL')).not.toBeInTheDocument()
    expect(screen.queryByText('需要处理')).not.toBeInTheDocument()
    expect(screen.queryByText('外围信号')).not.toBeInTheDocument()
    expect(screen.queryByText('先看停在哪里，再决定是否需要介入。所有数字都来自 Coordinator 当前数据库快照。')).not.toBeInTheDocument()
    expect(container.querySelectorAll('.panel-heading--compact')).toHaveLength(4)
    expect(container.querySelector('.admin-table')).toBeInTheDocument()
    const failureRow = container.querySelector('.admin-table--compact .ant-table-tbody > tr')!
    expect(window.getComputedStyle(failureRow).cursor).not.toBe('pointer')
    expect(container.querySelector('.business-outcomes__grid .sparkline polyline')).toBeInTheDocument()
    expect(container.querySelector('.business-outcomes__grid .sparkline polyline')).toHaveAttribute('stroke', '#7c3aed')
    expect(container.querySelector('.automation-metric-grid .sparkline polyline')).toHaveAttribute('stroke', '#16a34a')
    expect(container.querySelectorAll('.automation-metric-grid .sparkline polyline')).toHaveLength(4)
    expect(screen.getAllByText('暂无趋势').length).toBeGreaterThan(0)
    expect(screen.getByText('今日新增 1')).toBeInTheDocument()
  })

  it('passes repository_id from the URL into overview and metrics requests', async () => {
    mockOverview()
    render(<MemoryRouter initialEntries={['/?repository_id=repo_a']}><OverviewPage /></MemoryRouter>)

    await screen.findByText('已创建 MR')
    expect(adminApi.overview).toHaveBeenCalledWith({ window: '30d', repository_id: 'repo_a' })
    expect(adminApi.metrics).toHaveBeenCalledWith({ window: '30d', type: '', repository_id: 'repo_a' })
  })

  it('reloads the business summary when the statistics window changes', async () => {
    mockOverview()
    render(<MemoryRouter><OverviewPage /></MemoryRouter>)

    await screen.findByText('已创建 MR')
    expect(adminApi.overview).toHaveBeenCalledWith({ window: '30d' })
    expect(adminApi.metrics).toHaveBeenCalledWith({ window: '30d', type: '' })
    fireEvent.click(screen.getAllByText('最近 7 天')[0])
    await waitFor(() => expect(adminApi.overview).toHaveBeenLastCalledWith({ window: '7d' }))
    await waitFor(() => expect(adminApi.metrics).toHaveBeenLastCalledWith({ window: '7d', type: '' }))
  })

  it('keeps the previous snapshot visible while a window switch reloads', async () => {
    let calls = 0
    vi.spyOn(adminApi, 'overview').mockImplementation(() => {
      calls += 1
      return calls === 1 ? Promise.resolve(overviewPayload) : new Promise(() => {})
    })
    vi.spyOn(adminApi, 'listConnectors').mockResolvedValue(connectorsPayload)
    vi.spyOn(adminApi, 'metrics').mockResolvedValue(metricsPayload)

    const { container } = render(<MemoryRouter><OverviewPage /></MemoryRouter>)
    await screen.findByText('已创建 MR')

    fireEvent.click(screen.getAllByText('最近 7 天')[0])

    // 第二次请求挂起中：旧快照与两个面板仍在渲染，没有骨架屏
    expect(adminApi.overview).toHaveBeenCalledTimes(2)
    expect(adminApi.overview).toHaveBeenLastCalledWith({ window: '7d' })
    expect(container.querySelector('.business-outcomes__grid')).toBeInTheDocument()
    expect(container.querySelector('.automation-metric-grid')).toBeInTheDocument()
    expect(screen.queryByLabelText('正在加载')).not.toBeInTheDocument()
  })

  it('keeps the previous snapshot and shows a warning when a window switch reload fails', async () => {
    let calls = 0
    vi.spyOn(adminApi, 'overview').mockImplementation(() => {
      calls += 1
      return calls === 1 ? Promise.resolve(overviewPayload) : Promise.reject(new Error('后端不可用'))
    })
    vi.spyOn(adminApi, 'listConnectors').mockResolvedValue(connectorsPayload)
    let metricsCalls = 0
    vi.spyOn(adminApi, 'metrics').mockImplementation(() => {
      metricsCalls += 1
      return metricsCalls === 1 ? Promise.resolve(metricsPayload) : Promise.reject(new Error('指标不可用'))
    })

    const { container } = render(<MemoryRouter><OverviewPage /></MemoryRouter>)
    await screen.findByText('已创建 MR')

    fireEvent.click(screen.getAllByText('最近 7 天')[0])
    await waitFor(() => expect(screen.getByText(/快照刷新失败/)).toBeInTheDocument())

    expect(screen.getByText(/指标刷新失败/)).toBeInTheDocument()
    expect(container.querySelector('.business-outcomes__grid')).toBeInTheDocument()
  })

  it('shows the full error in a modal when the reason cell is clicked', async () => {
    mockOverview()

    render(<MemoryRouter><OverviewPage /></MemoryRouter>)
    const reason = await screen.findByText('模型调用失败')

    expect(reason.className).toContain('error-clamp')
    expect(screen.queryByText('完整错误原因')).not.toBeInTheDocument()

    fireEvent.click(reason)
    expect(screen.getByText('完整错误原因')).toBeInTheDocument()
    expect(document.querySelector('.error-detail')?.textContent).toBe('模型调用失败')
  })
})
