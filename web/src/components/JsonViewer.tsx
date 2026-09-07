import { Button, Tooltip } from 'antd'
import { CopyOutlined } from '@ant-design/icons'

function formatted(value: unknown): string {
  if (typeof value === 'string' && !value.trim()) return '—'
  return JSON.stringify(normalizeJSON(value), null, 2)
}

function normalizeJSON(value: unknown, key = ''): unknown {
  if (typeof value === 'string' && (key.endsWith('_json') || key === '')) {
    try {
      return normalizeJSON(JSON.parse(value))
    } catch {
      return value
    }
  }
  if (Array.isArray(value)) return value.map((item) => normalizeJSON(item))
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.entries(value).map(([childKey, childValue]) => [childKey, normalizeJSON(childValue, childKey)]))
  }
  return value ?? null
}

export function JsonViewer({ value, label = '原始数据' }: { value: unknown; label?: string }) {
  const text = formatted(value)
  return (
    <section className="json-viewer">
      <div className="json-viewer__bar">
        <span>{label}</span>
        <Tooltip title="复制">
          <Button type="text" size="small" aria-label={`复制${label}`} icon={<CopyOutlined />} onClick={() => navigator.clipboard?.writeText(text)} />
        </Tooltip>
      </div>
      <pre>{text}</pre>
    </section>
  )
}
