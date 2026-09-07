import { Alert, Button, Empty, Skeleton } from 'antd'

export function LoadingState({ rows = 4 }: { rows?: number }) {
  return <div className="state-box" aria-label="正在加载"><Skeleton active paragraph={{ rows }} /></div>
}

export function ErrorState({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const message = error instanceof Error ? error.message : '请求失败'
  return <Alert type="error" showIcon message="数据加载失败" description={message} action={onRetry ? <Button size="small" onClick={onRetry}>重新加载</Button> : undefined} />
}

export function EmptyState({ description = '当前没有记录' }: { description?: string }) {
  return <div className="state-box"><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={description} /></div>
}
