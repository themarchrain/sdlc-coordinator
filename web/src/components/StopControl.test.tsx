import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { adminApi } from '../api/client'
import type { StopStatus } from '../api/types'
import { WorkspaceSessionProvider } from '../app/workspaceSession'
import { StopControl } from './StopControl'

afterEach(() => { vi.restoreAllMocks(); vi.useRealTimers() })

const pending: StopStatus = { stop_id: 'stop-one', scope: 'task', task_id: 'task-one', task_status: 'interrupted', delivery_status: 'waiting_exit', exit_confirmed: false, requested_by: '17', requested_at: '2026-10-22T00:00:00Z' }

function control(id = 'task-one', stop?: StopStatus, role: 'tenant_admin' | 'member' = 'tenant_admin') {
  return <WorkspaceSessionProvider value={{ session: null, role, superAdmin: false }}><StopControl target="task" id={id} canStop initialStop={stop} /></WorkspaceSessionProvider>
}

describe('StopControl', () => {
  it('stops directly and blocks duplicate clicks while the request is pending', async () => {
    let finish!: (value: StopStatus) => void
    const stop = vi.spyOn(adminApi, 'stopTarget').mockImplementation(() => new Promise((resolve) => { finish = resolve }))
    render(control())
    fireEvent.click(screen.getByRole('button', { name: /停\s*止/ }))
    fireEvent.click(screen.getByRole('button', { name: /停\s*止/ }))
    expect(stop).toHaveBeenCalledTimes(1)
    expect(stop).toHaveBeenCalledWith('task', 'task-one')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    await act(async () => { finish(pending) })
    expect(screen.getByText('已中断，执行停止待确认')).toBeInTheDocument()
    expect(screen.queryByText('执行已退出')).not.toBeInTheDocument()
  })

  it.each([
    ['no_remote_target', '无需取消执行', false],
    ['create_blocked', '未启动，已阻止启动', false],
    ['execution_exited', '执行已退出', true],
  ] as const)('renders %s without inventing exit evidence', (kind, label, exited) => {
    const load = vi.spyOn(adminApi, 'getTask')
    render(control('task-one', { ...pending, delivery_status: 'confirmed', confirmation_kind: kind, exit_confirmed: exited }))
    expect(screen.getByText(label)).toBeInTheDocument()
    expect(load).not.toHaveBeenCalled()
  })

  it('polls every two seconds and stops after confirmation', async () => {
    vi.useFakeTimers()
    const load = vi.spyOn(adminApi, 'getTask').mockResolvedValue({ stop: { ...pending, delivery_status: 'confirmed', confirmation_kind: 'create_blocked' } })
    render(control('task-one', pending))
    await act(async () => { await vi.advanceTimersByTimeAsync(1999) })
    expect(load).not.toHaveBeenCalled()
    await act(async () => { await vi.advanceTimersByTimeAsync(1) })
    expect(load).toHaveBeenCalledTimes(1)
    expect(screen.getByText('未启动，已阻止启动')).toBeInTheDocument()
    await act(async () => { await vi.advanceTimersByTimeAsync(6000) })
    expect(load).toHaveBeenCalledTimes(1)
  })

  it('ignores old responses after switching targets and stops polling on close', async () => {
    vi.useFakeTimers()
    let finish!: (value: Record<string, unknown>) => void
    const load = vi.spyOn(adminApi, 'getTask').mockImplementation(() => new Promise((resolve) => { finish = resolve }))
    const view = render(control('task-one', pending))
    await act(async () => { await vi.advanceTimersByTimeAsync(2000) })
    view.rerender(control('task-two'))
    await act(async () => { finish({ stop: { ...pending, delivery_status: 'confirmed', confirmation_kind: 'execution_exited', exit_confirmed: true } }) })
    expect(screen.queryByText('执行已退出')).not.toBeInTheDocument()
    view.unmount()
    await act(async () => { await vi.advanceTimersByTimeAsync(6000) })
    expect(load).toHaveBeenCalledTimes(1)
  })

  it('does not report a failed request as stopped', async () => {
    vi.spyOn(adminApi, 'stopTarget').mockRejectedValue(new Error('stop rejected'))
    render(control())
    fireEvent.click(screen.getByRole('button', { name: /停\s*止/ }))
    expect(await screen.findByText('stop rejected')).toBeInTheDocument()
    expect(screen.queryByText('已中断，执行停止待确认')).not.toBeInTheDocument()
  })

  it('hides the stop action from members', () => {
    render(control('task-one', undefined, 'member'))
    expect(screen.queryByRole('button', { name: /停\s*止/ })).not.toBeInTheDocument()
  })

  it('keeps polling legacy confirmation without evidence', async () => {
    vi.useFakeTimers()
    const load = vi.spyOn(adminApi, 'getTask').mockResolvedValue({ stop: pending })
    render(control('task-one', { ...pending, delivery_status: 'confirmed' }))
    await act(async () => { await vi.advanceTimersByTimeAsync(2000) })
    expect(load).toHaveBeenCalledTimes(1)
    expect(screen.queryByText('执行已退出')).not.toBeInTheDocument()
  })
})
