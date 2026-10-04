import { Tag } from 'antd'

const statusColors: Record<string, string> = {
  succeeded: 'success',
  finished: 'success',
  healthy: 'success',
  approved: 'success',
  enabled: 'success',
  ready: 'success',
  running: 'processing',
  claimed: 'processing',
  configuring: 'processing',
  pending: 'default',
  retry_pending: 'warning',
  delivery_unverified: 'warning',
  paused: 'warning',
  waiting_for_confirmation: 'warning',
  waiting_upstream: 'warning',
  human_required: 'warning',
  degraded: 'warning',
  unconfigured: 'default',
  disabled: 'default',
  idle: 'default',
  skipped: 'default',
  failed: 'error',
  error: 'error',
  stuck: 'error',
  blocked: 'error',
  unavailable: 'error',
  unknown: 'default',
}

export function StatusTag({ status }: { status?: string }) {
  const value = status || 'unknown'
  return <Tag color={statusColors[value] ?? 'default'} className="status-tag">{value}</Tag>
}
