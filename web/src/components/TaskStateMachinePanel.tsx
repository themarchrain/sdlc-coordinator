import { useId } from 'react'

import graph from '../content/task-state-machine.json'

export function TaskStateMachinePanel({ status }: { status: string }) {
  const id = useId().replace(/:/g, '')
  const arrowID = `task-state-arrow-${id}`
  const descriptionID = `task-state-description-${id}`
  const current = graph.nodes.find((node) => node.id === status)

  return <section className="task-state-panel" aria-label="任务状态机">
    <div className="task-state-heading">
      <span className="section-label">状态机</span>
      <span className={current ? 'task-state-current' : 'error-copy'}>
        {current ? `当前：${current.label}` : `未识别状态：${status || 'unknown'}`}
      </span>
    </div>
    <p id={descriptionID} className="task-state-description">高亮为 Task 当前状态，箭头表示允许迁移，不代表实际历史路径。虚线表示人工停止。专用生命周期仍由后端裁决。</p>
    <div className="task-state-scroll" role="region" aria-label="状态图，可横向滚动" tabIndex={0}>
      <svg viewBox="-44 -136 984 768" role="img" aria-label="Task 状态机" aria-describedby={descriptionID}>
        <defs><marker id={arrowID} viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
          <path d="M0 0 L8 4 L0 8 Z" className="task-state-arrow" />
        </marker></defs>
        {graph.edges.map((edge) => <path key={`${edge.from}-${edge.to}`} d={edge.path} className={`task-state-edge${edge.kind === 'manual_stop' ? ' task-state-edge--manual-stop' : ''}`} strokeDasharray={edge.kind === 'manual_stop' ? '6 4' : undefined} markerEnd={`url(#${arrowID})`}>
          <title>{edge.from} → {edge.to}{edge.kind === 'manual_stop' ? '（人工停止）' : ''}</title>
        </path>)}
        {graph.nodes.map((node) => {
          const selected = node.id === status
          return <g key={node.id} transform={`translate(${node.x} ${node.y})`} role="group" aria-label={`${node.label}（${node.id}）${selected ? '，当前状态' : ''}`} aria-current={selected ? 'true' : undefined} className={`task-state-node${selected ? ' task-state-node--current' : ''}`}>
            <rect width="150" height="64" rx="8" />
            <text x="14" y="26" className="task-state-node-label">{node.label}</text>
            <text x="14" y="46" className="task-state-node-code">{node.id}</text>
            {selected ? <text x="136" y="25" textAnchor="end" className="task-state-node-current">当前</text> : null}
          </g>
        })}
      </svg>
    </div>
  </section>
}
