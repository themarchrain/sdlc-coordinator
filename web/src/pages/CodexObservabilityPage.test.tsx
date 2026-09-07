import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import type { CodexExecutionDetail, CodexObservabilitySummary } from '../api/types'
import { THEMES } from '../themes/registry'
import { CodexObservabilityPage, eventTypeColor } from './CodexObservabilityPage'

const summary: CodexObservabilitySummary = {
  total_executions: 2, status_counts: { succeeded: 1, failed: 1 }, average_duration_seconds: 42,
  p50_duration_seconds: 35, p90_duration_seconds: 61, input_tokens: 150, cached_input_tokens: 100,
  output_tokens: 30, cache_hit_ratio: 2 / 3, turn_count: 2, error_count: 1, timeout_count: 1,
  generated_at: '2026-08-19T10:00:00Z',
}

describe('CodexObservabilityPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(adminApi, 'codexSummary').mockResolvedValue(summary)
    vi.spyOn(adminApi, 'listCodexExecutions').mockResolvedValue({
      items: [{ execution_id: 'run-codex', run_id: 'run-codex', task_id: 'task-codex', task_type: 'review_mr', role: 'ReviewerWorker', repository: 'group/repo', status: 'succeeded', model: 'gpt-alpha', effort: 'high', request_hash: 'hash', duration_seconds: 35, created_at: '2026-08-19T09:00:00Z' }],
      page: 1, page_size: 10, total: 1,
    })
  })

  it('renders provider-specific summary without a provider selector', async () => {
    render(<MemoryRouter><CodexObservabilityPage /></MemoryRouter>)

    expect(await screen.findByText('缓存命中')).toBeInTheDocument()
    expect(screen.getByText('66.7%')).toBeInTheDocument()
    expect(screen.getByText('150')).toBeInTheDocument()
    expect(screen.getByText('ReviewerWorker')).toBeInTheDocument()
    expect(screen.queryByText('Provider')).not.toBeInTheDocument()
    expect(screen.queryByRole('combobox', { name: /Provider/i })).not.toBeInTheDocument()
  })

  it('filters executions and opens correlated detail links', async () => {
    const detail: CodexExecutionDetail = {
      execution_id: 'run-codex', run_id: 'run-codex', task_id: 'task-codex', task_type: 'review_mr', role: 'ReviewerWorker', repository: 'group/repo', status: 'succeeded', model: 'gpt-alpha', effort: 'high', request_hash: 'hash', duration_seconds: 35, created_at: '2026-08-19T09:00:00Z',
      events: [{ seq: 1, type: 'codex_usage', occurred_at: '2026-08-19T09:00:35Z', payload: { input_tokens: 150 } }],
    }
    vi.spyOn(adminApi, 'getCodexExecution').mockResolvedValue(detail)
    render(<MemoryRouter><CodexObservabilityPage /></MemoryRouter>)

    fireEvent.click(await screen.findByText('run-codex'))
    expect(await screen.findByRole('link', { name: '查看 Task' })).toHaveAttribute('href', '/tasks?task_id=task-codex')
    expect(screen.getByRole('link', { name: '查看 Run' })).toHaveAttribute('href', '/explorer?entity=runs&run_id=run-codex')
    expect(screen.getByText('codex_usage')).toBeInTheDocument()

    fireEvent.change(screen.getByPlaceholderText('Task ID'), { target: { value: 'task-filter' } })
	    fireEvent.click(screen.getByRole('button', { name: /查\s*询/ }))
    await waitFor(() => expect(adminApi.listCodexExecutions).toHaveBeenLastCalledWith(expect.objectContaining({ task_id: 'task-filter' })))
  })

  it('highlights terminal error events from both runners in red', () => {
    const slots = THEMES.default.slots.eventType
    expect(eventTypeColor('codex_error', slots)).toBe(slots.error)
    expect(eventTypeColor('claude_error', slots)).toBe(slots.error)
    expect(eventTypeColor('codex_usage', slots)).toBe(slots.usage)
    expect(eventTypeColor('claude_result', slots)).toBe(slots.info)
  })
})
