import { ClockCircleOutlined } from '@ant-design/icons'
import { useState } from 'react'
import { Alert, Button, Modal, Segmented, Select, Table, Tooltip } from 'antd'
import { Link, useSearchParams } from 'react-router-dom'

import { adminApi } from '../api/client'
import type { AutomationMetrics, BusinessSummary, DailyAveragePoint, DailyCountPoint, DailyRatioPoint, RatioMetric } from '../api/types'
import { repositoryFilter } from '../app/searchParams'
import { ErrorState, LoadingState } from '../components/PageState'
import { PageHeading } from '../components/PageHeading'
import { Sparkline, type SparklinePoint } from '../components/Sparkline'
import { StatusTag } from '../components/StatusTag'
import { useTheme } from '../theme/useTheme'
import { useRemote } from '../hooks/useRemote'

export function OverviewPage() {
  const [metricsWindow, setMetricsWindow] = useState<'7d' | '30d' | 'all'>('30d')
  const [taskType, setTaskType] = useState('')
  const [searchParams] = useSearchParams()
  const repositoryID = searchParams.get('repository_id') || undefined
  const overview = useRemote(() => adminApi.overview({ window: metricsWindow, ...repositoryFilter(searchParams) }), [metricsWindow, repositoryID])
  const connectors = useRemote(() => adminApi.listConnectors({ page: 1, page_size: 20 }), [])
  const metrics = useRemote(() => adminApi.metrics({ window: metricsWindow, type: taskType, ...repositoryFilter(searchParams) }), [metricsWindow, taskType, repositoryID])
  const [detailError, setDetailError] = useState<string>()

  return (
    <div>
      <PageHeading eyebrow="协调层运行状态" title="运行总览" action={<Button onClick={() => { overview.reload(); connectors.reload(); metrics.reload() }}>刷新快照</Button>} />
      {overview.data ? (
        <>
          {overview.error ? <Alert className="overview-refresh-alert" type="warning" showIcon message="快照刷新失败，当前显示的是上次获取的数据" /> : null}
          <BusinessOutcomePanel data={overview.data.business_summary} window={metricsWindow} onWindowChange={setMetricsWindow} />

          <AutomationMetricsPanel
            data={metrics.data}
            loading={metrics.loading}
            error={metrics.error}
            onRetry={metrics.reload}
            window={metricsWindow}
            taskType={taskType}
            onWindowChange={setMetricsWindow}
            onTaskTypeChange={setTaskType}
          />

          <div className="dashboard-grid">
            <section className="surface-panel failure-panel">
              <div className="panel-heading panel-heading--compact"><div><h2>最近失败</h2></div><Link to="/tasks?status=human_required">查看待人工任务</Link></div>
              <Table
                className="admin-table admin-table--compact"
                rowKey={(row) => String(row.entity_type) + String(row.entity_id)}
                size="small"
                tableLayout="fixed"
                scroll={{ x: 940 }}
                pagination={false}
                dataSource={overview.data.recent_failures}
                locale={{ emptyText: '最近没有失败记录' }}
                columns={[
                  { title: '对象', width: 280, render: (_, row) => <span className="mono table-cell-wrap">{String(row.entity_type)} / {String(row.entity_id)}</span> },
                  { title: '状态', dataIndex: 'status', width: 150, render: (value) => <StatusTag status={String(value)} /> },
                  { title: '原因', dataIndex: 'error', width: 330, render: (value) => {
                    const text = String(value || '未记录错误摘要')
                    return <span className="error-copy error-clamp" title="点击查看完整错误" onClick={() => setDetailError(text)}>{text}</span>
                  } },
                  { title: '时间', dataIndex: 'occurred_at', width: 180, render: (value) => <time className="mono table-time">{formatTime(value)}</time> },
                ]}
              />
            </section>

            <section className="surface-panel connector-watch">
              <div className="panel-heading panel-heading--compact"><div><h2>连接器</h2></div><Link to="/connectors">管理连接器</Link></div>
              {connectors.loading ? <LoadingState rows={2} /> : connectors.error ? <ErrorState error={connectors.error} onRetry={connectors.reload} /> : (
                <div className="connector-watch__list">
                  {connectors.data?.items.length ? connectors.data.items.map((connector) => (
                    <article key={connector.connector_id} className="connector-watch__item">
                      <div><strong>{connector.display_name}</strong><span>{connector.target_project_path || '未限定仓库'}</span></div>
                      <div className="connector-watch__health"><StatusTag status={connector.last_health_status} /><small>{connector.last_health_message || '尚未探测'}</small></div>
                    </article>
                  )) : <p className="muted-copy">尚未注册状态源连接器。</p>}
                </div>
              )}
            </section>
          </div>
          <div className="snapshot-note"><ClockCircleOutlined /> 快照生成于 <span className="mono">{formatTime(overview.data.generated_at)}</span></div>
          <Modal title="完整错误原因" open={Boolean(detailError)} onCancel={() => setDetailError(undefined)} footer={null} width={720}>
            <pre className="error-detail mono">{detailError}</pre>
          </Modal>
        </>
      ) : overview.loading ? <LoadingState /> : overview.error ? <ErrorState error={overview.error} onRetry={overview.reload} /> : null}
    </div>
  )
}

function BusinessOutcomePanel({ data, window, onWindowChange }: {
  data: BusinessSummary
  window: '7d' | '30d' | 'all'
  onWindowChange: (value: '7d' | '30d' | 'all') => void
}) {
  const { theme } = useTheme()
  const trendColors = theme.slots.chart.trends
  const hasSample = data.automation_success.denominator > 0
  return (
    <section className="surface-panel business-outcomes">
      <div className="panel-heading panel-heading--compact business-outcomes__heading">
        <div><h2>业务处理量</h2></div>
        <Segmented
          value={window}
          options={[{ label: '最近 7 天', value: '7d' }, { label: '最近 30 天', value: '30d' }, { label: '全部', value: 'all' }]}
          onChange={(value) => onWindowChange(value as '7d' | '30d' | 'all')}
        />
      </div>
      <div className="business-outcomes__grid">
        <BusinessMetricCard label="已创建 MR" value={formatBusinessCount(data.merge_requests_created, hasSample)} trend={countTrend(data.daily_merge_requests_created)} trendColor={trendColors.mr} trendLabel="每日创建 MR 趋势" />
        <BusinessMetricCard label="已完成 CR" value={formatBusinessCount(data.reviews_completed, hasSample)} trend={countTrend(data.daily_reviews_completed)} trendColor={trendColors.crDone} trendLabel="每日完成 CR 趋势" />
        <BusinessMetricCard label="已发起 CR" value={formatBusinessCount(data.reviewer_takeovers, hasSample)} trend={countTrend(data.daily_reviewer_takeovers)} trendColor={trendColors.crStart} trendLabel="每日发起 CR 趋势" />
        <BusinessMetricCard label="成功运行平均耗时" value={data.average_successful_run_seconds === null ? '—' : formatDuration(data.average_successful_run_seconds)} trend={avgTrend(data.daily_average_run_seconds)} trendColor={trendColors.avgDuration} trendFormat={formatDuration} trendLabel="每日成功运行平均耗时趋势" />
        <BusinessMetricCard label="待人工处理" value={formatBusinessCount(data.human_required_tasks, hasSample)} detail={`今日新增 ${data.human_required_added_today}`} />
      </div>
    </section>
  )
}

function countTrend(points: DailyCountPoint[]): SparklinePoint[] {
  return points.map((point) => ({ date: point.date, value: point.count }))
}

function avgTrend(points: DailyAveragePoint[]): SparklinePoint[] {
  return points.map((point) => ({ date: point.date, value: point.seconds }))
}

function BusinessMetricCard({ label, value, detail, trend, trendColor, trendFormat, trendLabel }: {
  label: string
  value: string
  detail?: string
  trend?: SparklinePoint[]
  trendColor?: string
  trendFormat?: (value: number) => string
  trendLabel?: string
}) {
  return (
    <article className="business-outcomes__card">
      <span>{label}</span>
      <strong>{value}</strong>
      {trend ? <Sparkline data={trend} color={trendColor} format={trendFormat} ariaLabel={trendLabel} /> : detail ? <small>{detail}</small> : null}
    </article>
  )
}

function AutomationMetricsPanel({
  data, loading, error, onRetry, window, taskType, onWindowChange, onTaskTypeChange,
}: {
  data?: AutomationMetrics
  loading: boolean
  error: unknown
  onRetry: () => void
  window: '7d' | '30d' | 'all'
  taskType: string
  onWindowChange: (value: '7d' | '30d' | 'all') => void
  onTaskTypeChange: (value: string) => void
}) {
  const { theme } = useTheme()
  const ratioColors = theme.slots.chart.ratios
  return (
    <section className="surface-panel automation-metrics">
      <div className="panel-heading panel-heading--compact metrics-heading">
        <div><h2>自动化效果</h2></div>
        <div className="metrics-filters">
          <Segmented
            value={window}
            options={[{ label: '最近 7 天', value: '7d' }, { label: '最近 30 天', value: '30d' }, { label: '全部', value: 'all' }]}
            onChange={(value) => onWindowChange(value as '7d' | '30d' | 'all')}
          />
          <Select
            allowClear
            value={taskType || undefined}
            placeholder="全部任务类型"
            options={[
              { value: 'develop_from_issue', label: 'Issue 开发' },
              { value: 'review_mr', label: 'MR 审查' },
              { value: 'rework_merge_request', label: 'MR 接续开发' },
            ]}
            onChange={(value) => onTaskTypeChange(value ?? '')}
          />
        </div>
      </div>
      {data ? (
        <>
          {error ? <Alert className="overview-refresh-alert" type="warning" showIcon message="指标刷新失败，当前显示的是上次获取的数据" /> : null}
          <div className="automation-metric-grid">
            <RatioCard title="当前任务成功占比" metric={data.current_success} note="当前 succeeded Task / 窗口内全部 Task" trend={ratioTrend(data.daily_series, 'current_success')} trendColor={ratioColors.success} trendFormat={formatRatioPercent} />
            <RatioCard title="首次执行成功率" metric={data.first_run_success} note="第一条已完成 Run 即成功的 Task" trend={ratioTrend(data.daily_series, 'first_run_success')} trendColor={ratioColors.firstRun} trendFormat={formatRatioPercent} inferred />
            <RatioCard title="触发人工介入率" metric={data.intervention_triggered} note="出现 blocked Run 或当前 human_required" trend={ratioTrend(data.daily_series, 'intervention_triggered')} trendColor={ratioColors.intervention} trendFormat={formatRatioPercent} inferred />
            <RatioCard title="受阻后恢复成功率" metric={data.recovered_after_block} note="出现 blocked Run 后最终成功" trend={ratioTrend(data.daily_series, 'recovered_after_block')} trendColor={ratioColors.recovery} trendFormat={formatRatioPercent} inferred />
          </div>
          <div className="duration-section">
            <div className="duration-section__heading"><strong>成功 Run 耗时</strong><span>成功的 Run · Task 样本 {data.task_sample_count}</span></div>
            {data.successful_run_durations.length ? <div className="duration-grid">{data.successful_run_durations.map((duration) => (
              <article className="duration-card" key={duration.worker_role}>
                <div><strong>{duration.worker_role}</strong>{duration.sample_count < 10 ? <span className="sample-warning">样本较少</span> : null}</div>
                <small>样本 {duration.sample_count}</small>
                <div className="duration-values">
                  <span>平均 {formatDuration(duration.average_seconds)}</span>
                  <span>P50 {formatDuration(duration.p50_seconds)}</span>
                  <span>P90 {formatDuration(duration.p90_seconds)}</span>
                </div>
              </article>
            ))}</div> : <p className="muted-copy">暂无可统计的成功 Run 样本。</p>}
          </div>
        </>
      ) : loading ? <LoadingState rows={2} /> : error ? <ErrorState error={error} onRetry={onRetry} /> : null}
    </section>
  )
}

function RatioCard({ title, metric, note, inferred = false, trend, trendColor, trendFormat }: {
  title: string
  metric: RatioMetric
  note: string
  inferred?: boolean
  trend?: SparklinePoint[]
  trendColor?: string
  trendFormat?: (value: number) => string
}) {
  return (
    <article className="ratio-card">
      <div className="ratio-card__title"><span>{title}</span>{inferred ? <Tooltip title="根据 blocked Run 与当前 Task 状态推导，不代表已记录人工操作事件。"><span className="inferred-mark">推导</span></Tooltip> : null}</div>
      <strong>{formatRatio(metric.value)}</strong>
      <div className="ratio-card__sample"><span>{metric.numerator} / {metric.denominator}</span>{metric.denominator > 0 && metric.denominator < 10 ? <span className="sample-warning">样本较少</span> : null}</div>
      {trend ? <Sparkline data={trend} color={trendColor} format={trendFormat} ariaLabel={`${title}每日趋势`} /> : null}
      <p>{note}</p>
    </article>
  )
}

function ratioTrend(points: DailyRatioPoint[], key: 'current_success' | 'first_run_success' | 'intervention_triggered' | 'recovered_after_block'): SparklinePoint[] {
  return points.map((point) => ({ date: point.date, value: point[key] }))
}

function formatRatioPercent(value: number) {
  return `${(value * 100).toFixed(0)}%`
}

function formatBusinessCount(value: number, hasSample: boolean) {
  return hasSample ? String(value) : '—'
}

function formatRatio(value: number | null) {
  return value === null ? '—' : `${(value * 100).toFixed(1)}%`
}

function formatDuration(value: number) {
  const seconds = Math.max(0, Math.round(value))
  if (seconds < 60) return `${seconds} 秒`
  const minutes = Math.floor(seconds / 60)
  const remainder = seconds % 60
  return remainder ? `${minutes} 分 ${remainder} 秒` : `${minutes} 分钟`
}

function formatTime(value: unknown) {
  if (!value) return '—'
  const date = new Date(String(value))
  return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString('zh-CN', { hour12: false })
}
