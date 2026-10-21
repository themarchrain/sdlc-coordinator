import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, useSearchParams } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import { TasksPage } from './TasksPage'

beforeEach(() => {
  vi.spyOn(adminApi, 'listTasks').mockResolvedValue({ items: [], page: 1, page_size: 20, total: 0 })
  vi.spyOn(adminApi, 'taskTimeline').mockResolvedValue({ items: [] })
})
afterEach(() => vi.restoreAllMocks())

describe('Task drawer state machine', () => {
  it('shows a pending Task without a Run', async () => {
    vi.spyOn(adminApi, 'getTask').mockResolvedValue({ task_id: 'task-a', status: 'pending' })
    openTask()

    expect(await screen.findByRole('group', { name: '待派发（pending），当前状态' })).toBeInTheDocument()
  })

  it('uses the Task status even when the Run timeline has a different status', async () => {
    vi.spyOn(adminApi, 'getTask').mockResolvedValue({ task_id: 'task-a', status: 'human_required' })
    vi.mocked(adminApi.taskTimeline).mockResolvedValue({ items: [{ event_id: 'e1', occurred_at: '2026-10-21T00:00:00Z', category: 'run', action: 'completed', status: 'succeeded', title: 'Run 已结束', summary: '', entity_type: 'run', entity_id: 'run-a' }] })
    openTask()

    expect(await screen.findByRole('group', { name: '需要人工（human_required），当前状态' })).toBeInTheDocument()
    expect(screen.getByRole('group', { name: '已成功（succeeded）' })).not.toHaveAttribute('aria-current')
  })

  it('refreshes the Task and graph together', async () => {
    const getTask = vi.spyOn(adminApi, 'getTask').mockResolvedValueOnce({ task_id: 'task-a', status: 'pending' }).mockResolvedValue({ task_id: 'task-a', status: 'running' })
    openTask()
    await screen.findByRole('group', { name: '待派发（pending），当前状态' })
    fireEvent.click(screen.getByRole('button', { name: /刷\s*新/ }))

    expect(await screen.findByRole('group', { name: '运行中（running），当前状态' })).toBeInTheDocument()
    expect(getTask).toHaveBeenCalledTimes(2)
    expect(adminApi.taskTimeline).toHaveBeenCalledTimes(2)
    expect(screen.getByText(/读取于/)).toBeInTheDocument()
  })

  it('ignores a stale response after switching Tasks', async () => {
    const old = deferred<Record<string, unknown>>()
    vi.spyOn(adminApi, 'getTask').mockImplementation((id) => id === 'task-a' ? old.promise : Promise.resolve({ task_id: 'task-b', status: 'succeeded', objective: 'new task objective' }))
    openTask()
    await waitFor(() => expect(adminApi.getTask).toHaveBeenCalledWith('task-a'))
    fireEvent.click(screen.getByRole('button', { name: 'switch-task' }))
    await screen.findByRole('group', { name: '已成功（succeeded），当前状态' })
    await act(async () => old.resolve({ task_id: 'task-a', status: 'running', objective: 'stale objective' }))

    expect(screen.getByText('new task objective')).toBeInTheDocument()
    expect(screen.queryByText('stale objective')).not.toBeInTheDocument()
    expect(screen.getByRole('group', { name: '已成功（succeeded），当前状态' })).toBeInTheDocument()
  })

  it('ignores an older refresh response for the same Task', async () => {
    const old = deferred<Record<string, unknown>>()
    const getTask = vi.spyOn(adminApi, 'getTask').mockResolvedValueOnce({ task_id: 'task-a', status: 'pending' }).mockReturnValueOnce(old.promise).mockResolvedValue({ task_id: 'task-a', status: 'succeeded' })
    openTask()
    await screen.findByRole('group', { name: '待派发（pending），当前状态' })
    fireEvent.click(screen.getByRole('button', { name: /刷\s*新/ }))
    await waitFor(() => expect(getTask).toHaveBeenCalledTimes(2))
    fireEvent.click(screen.getByRole('button', { name: /刷\s*新/ }))
    await screen.findByRole('group', { name: '已成功（succeeded），当前状态' })
    await act(async () => old.resolve({ task_id: 'task-a', status: 'running' }))

    expect(screen.getByRole('group', { name: '已成功（succeeded），当前状态' })).toBeInTheDocument()
  })

  it('lets a failed detail load be retried', async () => {
    vi.spyOn(adminApi, 'getTask').mockRejectedValueOnce(new Error('temporary failure')).mockResolvedValue({ task_id: 'task-a', status: 'pending' })
    openTask()
    await screen.findByText('temporary failure')
    fireEvent.click(screen.getByRole('button', { name: '重新加载' }))

    expect(await screen.findByRole('group', { name: '待派发（pending），当前状态' })).toBeInTheDocument()
  })
})

function openTask() {
  return render(<MemoryRouter initialEntries={['/tasks?task_id=task-a']}><Navigation /><TasksPage /></MemoryRouter>)
}

function Navigation() {
  const [, setParams] = useSearchParams()
  return <button onClick={() => setParams({ task_id: 'task-b' })}>switch-task</button>
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((complete) => { resolve = complete })
  return { promise, resolve }
}
