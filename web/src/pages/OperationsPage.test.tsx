import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import { OperationsPage } from './OperationsPage'

afterEach(() => vi.restoreAllMocks())

describe('OperationsPage', () => {
  it('requires confirmation and renders one Tick result', async () => {
    vi.spyOn(adminApi, 'tickDefaults').mockResolvedValue({ dispatch_limit: 1, poll_limit: 5, recovery_limit: 5, effect_limit: 5 })
    vi.spyOn(adminApi, 'listEntities').mockResolvedValue({ items: [], page: 1, page_size: 20, total: 0 })
    vi.spyOn(adminApi, 'tick').mockResolvedValue({
      polled_runs: [{ run_id: 'run-1' }], reconciled_writebacks: [], finalized_results: [], recoveries: [],
      dispatched_tasks: [{ task_id: 'task-1' }], dispatch_failures: [{ task_id: 'task-2', error: 'fetch failed' }],
      executed_effects: [{ effect_id: 'effect-1' }],
    })

    render(<OperationsPage />)
    expect(screen.queryByText('状态推进')).not.toBeInTheDocument()
    expect(screen.queryByText('人工恢复')).not.toBeInTheDocument()
    fireEvent.click(await screen.findByRole('button', { name: /执行一次 Tick/ }))
    expect(adminApi.tick).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: '确认执行' }))

    expect(await screen.findByText('Tick 已完成')).toBeInTheDocument()
    expect(screen.getByText('1 个 Run')).toBeInTheDocument()
    expect(screen.getByText('派发失败')).toBeInTheDocument()
    expect(screen.getAllByText('1 个 Task')).toHaveLength(2)
    expect(screen.getByText('1 个 Effect')).toBeInTheDocument()
  })

  it('stays on page when a Tick result group is missing', async () => {
    vi.spyOn(adminApi, 'tickDefaults').mockResolvedValue({ dispatch_limit: 1, poll_limit: 5, recovery_limit: 5, effect_limit: 5 })
    vi.spyOn(adminApi, 'listEntities').mockResolvedValue({ items: [], page: 1, page_size: 20, total: 0 })
    vi.spyOn(adminApi, 'tick').mockResolvedValue({
      polled_runs: null, dispatched_tasks: [], executed_effects: null,
    } as never)

    render(<OperationsPage />)
    fireEvent.click(await screen.findByRole('button', { name: /执行一次 Tick/ }))
    fireEvent.click(screen.getByRole('button', { name: '确认执行' }))

    expect(await screen.findByText('Tick 已完成')).toBeInTheDocument()
    expect(screen.getByText('0 个 Run')).toBeInTheDocument()
  })

  it('locks the form until edit, and cancel restores the defaults', async () => {
    vi.spyOn(adminApi, 'tickDefaults').mockResolvedValue({ dispatch_limit: 1, poll_limit: 5, recovery_limit: 5, effect_limit: 5 })
    vi.spyOn(adminApi, 'listEntities').mockResolvedValue({ items: [], page: 1, page_size: 20, total: 0 })

    render(<OperationsPage />)
    const dispatchInput = await screen.findByLabelText('派发上限')
    expect(dispatchInput).toBeDisabled()
    expect(screen.getByRole('button', { name: /编辑参数/ })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /编辑参数/ }))
    expect(dispatchInput).not.toBeDisabled()
    fireEvent.change(dispatchInput, { target: { value: '3' } })

    fireEvent.click(screen.getByRole('button', { name: /取\s*消/ }))
    expect(dispatchInput).toBeDisabled()
    await waitFor(() => expect((dispatchInput as HTMLInputElement).value).toBe('1'))
    expect(adminApi.tickDefaults).toHaveBeenCalledTimes(2)
  })

  it('saves edited values and returns to locked state', async () => {
    vi.spyOn(adminApi, 'tickDefaults').mockResolvedValue({ dispatch_limit: 1, poll_limit: 5, recovery_limit: 5, effect_limit: 5 })
    vi.spyOn(adminApi, 'listEntities').mockResolvedValue({ items: [], page: 1, page_size: 20, total: 0 })

    render(<OperationsPage />)
    const dispatchInput = await screen.findByLabelText('派发上限')
    fireEvent.click(screen.getByRole('button', { name: /编辑参数/ }))
    fireEvent.change(dispatchInput, { target: { value: '3' } })
    fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    expect(await screen.findByText('参数已保存，将在下次手动 Tick 时生效')).toBeInTheDocument()
    expect(dispatchInput).toBeDisabled()
    expect((dispatchInput as HTMLInputElement).value).toBe('3')
    expect(adminApi.tickDefaults).toHaveBeenCalledTimes(1)
  })

  it('retries a failed Effect by effect ID', async () => {
    vi.spyOn(adminApi, 'tickDefaults').mockResolvedValue({})
    vi.spyOn(adminApi, 'listEntities').mockResolvedValue({
      items: [{ effect_id: 'effect-1', task_id: 'task-1', kind: 'send_im_message', status: 'failed', error_message: 'IM timeout', created_at: '2026-08-08T08:00:00Z' }],
      page: 1, page_size: 20, total: 1,
    })
    vi.spyOn(adminApi, 'retryEffect').mockResolvedValue({ effect_id: 'effect-1', status: 'pending' })

    render(<OperationsPage />)
    fireEvent.click(await screen.findByRole('button', { name: '重新入队' }))
    expect(await screen.findByText('Effect 已恢复为 pending')).toBeInTheDocument()
    expect(adminApi.retryEffect).toHaveBeenCalledWith('effect-1')
  })
})
