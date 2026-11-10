import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { TaskStateMachinePanel } from './TaskStateMachinePanel'

describe('TaskStateMachinePanel', () => {
  it.each([
    ['pending', '待派发'], ['claimed', '已领取'], ['running', '运行中'],
    ['finalizing', '结果收口中'], ['retry_pending', '等待重试'],
    ['waiting_upstream', '等待上游'], ['waiting_merge', '等待合并'],
    ['human_required', '需要人工'], ['blocked', '已阻塞'], ['succeeded', '已成功'],
    ['interrupted', '已中断'],
  ])('highlights only the current Task status %s', (status, label) => {
    const { container } = render(<TaskStateMachinePanel status={status} />)

    expect(screen.getByRole('img', { name: 'Task 状态机' })).toBeInTheDocument()
    expect(screen.getByRole('group', { name: `${label}（${status}），当前状态` })).toHaveAttribute('aria-current', 'true')
    expect(container.querySelectorAll('[aria-current="true"]')).toHaveLength(1)
    expect(container.querySelectorAll('path[marker-end]')).toHaveLength(30)
  })

  it('distinguishes manual stops and shows interruption as a terminal state', () => {
    const { container } = render(<TaskStateMachinePanel status="interrupted" />)
    const manualStops = container.querySelectorAll('.task-state-edge--manual-stop')

    expect(screen.getByText(/虚线表示人工停止/)).toBeInTheDocument()
    expect(manualStops).toHaveLength(9)
    expect([...manualStops].map((path) => path.textContent)).toEqual(expect.arrayContaining([
      'pending → interrupted（人工停止）', 'claimed → interrupted（人工停止）',
      'running → interrupted（人工停止）', 'finalizing → interrupted（人工停止）',
      'retry_pending → interrupted（人工停止）', 'waiting_upstream → interrupted（人工停止）',
      'waiting_merge → interrupted（人工停止）', 'human_required → interrupted（人工停止）',
      'blocked → interrupted（人工停止）',
    ]))
    for (const path of manualStops) expect(path).toHaveAttribute('stroke-dasharray', '6 4')
    expect([...container.querySelectorAll('path[marker-end]')].some((path) => path.textContent?.startsWith('interrupted →'))).toBe(false)
    expect([...manualStops].some((path) => path.textContent?.startsWith('succeeded →'))).toBe(false)
    expect(container.querySelector('path[marker-end] title')?.textContent).toBeDefined()
    expect([...container.querySelectorAll('path[marker-end] title')].map((title) => title.textContent)).toContain('blocked → pending（经恢复资格检查）')
  })

  it('keeps unknown states visible without highlighting a known state', () => {
    const { container } = render(<TaskStateMachinePanel status="future_status" />)

    expect(screen.getByText('未识别状态：future_status')).toBeInTheDocument()
    expect(container.querySelector('[aria-current="true"]')).not.toBeInTheDocument()
  })

  it('moves the current marker when the Task status changes', () => {
    const { rerender } = render(<TaskStateMachinePanel status="running" />)
    rerender(<TaskStateMachinePanel status="retry_pending" />)

    expect(screen.getByRole('group', { name: '等待重试（retry_pending），当前状态' })).toHaveAttribute('aria-current', 'true')
    expect(screen.getByRole('group', { name: '运行中（running）' })).not.toHaveAttribute('aria-current')
  })

  it('gives each diagram its own arrow marker', () => {
    const { container } = render(<><TaskStateMachinePanel status="pending" /><TaskStateMachinePanel status="running" /></>)
    const markers = [...container.querySelectorAll('marker')].map((marker) => marker.id)

    expect(markers).toHaveLength(2)
    expect(new Set(markers).size).toBe(2)
    for (const path of container.querySelectorAll('path[marker-end]')) {
      expect(markers).toContain(path.getAttribute('marker-end')?.slice(5, -1))
    }
  })
})
