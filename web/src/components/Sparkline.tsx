import { useMemo, useState } from 'react'

export interface SparklinePoint {
  date: string
  value: number | null
}

interface SparklineProps {
  data: SparklinePoint[]
  color?: string
  format?: (value: number) => string
  ariaLabel?: string
}

const VIEW_W = 100
const VIEW_H = 28

export function Sparkline({ data, color = '#2f6bff', format, ariaLabel }: SparklineProps) {
  const [hoverIndex, setHoverIndex] = useState<number | null>(null)
  const points = useMemo(() => data.filter((point) => point.value != null), [data])

  if (points.length < 2) {
    return <div className="sparkline sparkline--empty">暂无趋势</div>
  }

  const values = points.map((point) => point.value as number)
  const min = Math.min(...values)
  const max = Math.max(...values)
  const range = max - min || 1
  const coords = points.map((point, index) => {
    const x = (index / (points.length - 1)) * VIEW_W
    const y = VIEW_H - 3 - ((point.value as number - min) / range) * (VIEW_H - 6)
    return { x, y, date: point.date, value: point.value as number }
  })
  const line = coords.map((point) => `${point.x.toFixed(2)},${point.y.toFixed(2)}`).join(' ')
  const area = `0,${VIEW_H} ${line} ${VIEW_W},${VIEW_H}`

  const onMove = (event: React.MouseEvent<SVGSVGElement>) => {
    const rect = event.currentTarget.getBoundingClientRect()
    if (rect.width === 0) return
    const ratio = (event.clientX - rect.left) / rect.width
    const index = Math.round(ratio * (coords.length - 1))
    setHoverIndex(Math.min(Math.max(index, 0), coords.length - 1))
  }

  const hovered = hoverIndex != null ? coords[hoverIndex] : null
  const tipLeft = hovered ? Math.min(95, Math.max(5, (hovered.x / VIEW_W) * 100)) : 0

  return (
    <div className="sparkline">
      <svg
        viewBox={`0 0 ${VIEW_W} ${VIEW_H}`}
        preserveAspectRatio="none"
        role="img"
        aria-label={ariaLabel}
        onMouseMove={onMove}
        onMouseLeave={() => setHoverIndex(null)}
      >
        <polygon points={area} fill={color} opacity="0.08" />
        <polyline points={line} fill="none" stroke={color} strokeWidth="1.6" vectorEffect="non-scaling-stroke" />
        {coords.map((point, index) => <circle key={point.date} cx={point.x} cy={point.y} r="1.2" fill={color} />)}
        {hovered ? <line x1={hovered.x} y1="0" x2={hovered.x} y2={VIEW_H} stroke="#c5cfdd" strokeWidth="0.5" vectorEffect="non-scaling-stroke" strokeDasharray="2 2" /> : null}
      </svg>
      {hovered ? (
        <div className="sparkline__tip" style={{ left: `${tipLeft}%` }}>
          <span>{hovered.date}</span>
          <strong>{format ? format(hovered.value) : hovered.value}</strong>
        </div>
      ) : null}
    </div>
  )
}
