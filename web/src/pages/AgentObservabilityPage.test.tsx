import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import type { AgentExecutionDetail, AgentExecutionSummary } from '../api/types'
import { THEMES } from '../themes/registry'
import { AgentObservabilityPage, eventTypeColor } from './AgentObservabilityPage'

const summary: AgentExecutionSummary = {
  total_executions: 2, status_counts: { succeeded: 1, failed: 1 }, average_duration_seconds: 42,
  p50_duration_seconds: 35, p90_duration_seconds: 61, input_tokens: 150, cached_input_tokens: 100,
  output_tokens: 30, cache_hit_ratio: 2 / 3, turn_count: 2, error_count: 1, timeout_count: 1,
  generated_at: '2026-08-19T10:00:00Z',
}

function LocationProbe() {
  const location = useLocation()
  return <div data-testid="location-search">{location.search}</div>
}

describe('AgentObservabilityPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(adminApi, 'agentExecutionSummary').mockResolvedValue(summary)
    vi.spyOn(adminApi, 'listAgentExecutions').mockResolvedValue({
      items: [
        { execution_id: 'run-claude', run_id: 'run-claude', task_id: 'task-claude', task_type: 'review_mr', role: 'ReviewerWorker', repository: 'group/repo', provider: 'claude-cli', status: 'succeeded', model: 'claude-alpha', effort: 'high', request_hash: 'hash', duration_seconds: 30, created_at: '2026-08-19T09:00:00Z' },
        { execution_id: 'run-codex', run_id: 'run-codex', task_id: 'task-codex', task_type: 'review_mr', role: 'DeveloperWorker', repository: 'group/repo', provider: 'codex-cli', status: 'succeeded', model: 'gpt-alpha', effort: 'high', request_hash: 'hash', duration_seconds: 35, created_at: '2026-08-19T08:00:00Z' },
      ],
      page: 1, page_size: 10, total: 2,
    })
  })

  it('renders cross-provider summary with a provider filter defaulting to all', async () => {
    render(<MemoryRouter><AgentObservabilityPage /></MemoryRouter>)

    expect(await screen.findByText('缓存命中')).toBeInTheDocument()
    expect(screen.getByText('66.7%')).toBeInTheDocument()
    expect(screen.getByText('150')).toBeInTheDocument()
    expect(screen.getByText('Claude Code')).toBeInTheDocument()
    expect(screen.getByText('Codex')).toBeInTheDocument()
    expect(screen.queryByText('claude-cli')).not.toBeInTheDocument()
    await waitFor(() => expect(adminApi.listAgentExecutions).toHaveBeenNthCalledWith(1, { page: 1, page_size: 10 }))
  })

  it('filters by provider and renders unknown event types in the detail drawer', async () => {
    const detail: AgentExecutionDetail = {
      execution_id: 'run-claude', run_id: 'run-claude', task_id: 'task-claude', task_type: 'review_mr', role: 'ReviewerWorker', repository: 'group/repo', provider: 'claude-cli', status: 'succeeded', model: 'claude-alpha', effort: 'high', request_hash: 'hash', duration_seconds: 30, created_at: '2026-08-19T09:00:00Z',
      events: [
        { seq: 1, type: 'claude_session', occurred_at: '2026-08-19T09:00:00Z', payload: { session_id: 'session-1' } },
        { seq: 2, type: 'claude_usage', occurred_at: '2026-08-19T09:00:20Z', payload: { model: 'claude-alpha', usage: { input_tokens: 200, output_tokens: 40 } } },
        { seq: 3, type: 'brand_new_event', occurred_at: '2026-08-19T09:00:30Z', payload: { foo: 'bar' } },
      ],
    }
    vi.spyOn(adminApi, 'getAgentExecution').mockResolvedValue(detail)
    render(<MemoryRouter><AgentObservabilityPage /></MemoryRouter>)

    fireEvent.click(await screen.findByText('run-claude'))
    expect(await screen.findByRole('link', { name: '查看 Task' })).toHaveAttribute('href', '/tasks?task_id=task-claude')
    expect(screen.getByRole('link', { name: '查看 Run' })).toHaveAttribute('href', '/explorer?entity=runs&run_id=run-claude')
    expect(screen.getByText('claude_usage')).toBeInTheDocument()
    expect(screen.getByText(/usage: \{"input_tokens":200,"output_tokens":40\}/)).toBeInTheDocument()
    expect(screen.getByText('brand_new_event')).toBeInTheDocument()
    expect(screen.getByText(/foo: bar/)).toBeInTheDocument()
    expect(screen.queryByText(/\[object Object\]/)).not.toBeInTheDocument()

    fireEvent.mouseDown(screen.getAllByRole('combobox')[0])
    const option = (await screen.findAllByText('Codex')).find((element) => element.closest('.ant-select-item-option'))
    fireEvent.click(option!)
    expect(document.querySelector('.filter-panel .ant-select-selection-item')).toHaveTextContent('Codex')
    fireEvent.click(screen.getByRole('button', { name: /查\s*询/ }))
    await waitFor(() => expect(adminApi.listAgentExecutions).toHaveBeenLastCalledWith(expect.objectContaining({ provider: 'codex-cli' })))

    fireEvent.change(screen.getByPlaceholderText('Task ID'), { target: { value: 'task-filter' } })
    fireEvent.click(screen.getByRole('button', { name: /查\s*询/ }))
    await waitFor(() => expect(adminApi.listAgentExecutions).toHaveBeenLastCalledWith(expect.objectContaining({ provider: 'codex-cli', task_id: 'task-filter' })))
  })

  it('derives drawer tokens from the terminal claude_result when per-turn usage is zero', async () => {
    const detail: AgentExecutionDetail = {
      execution_id: 'run-relay', run_id: 'run-relay', task_id: 'task-relay', task_type: 'develop_from_issue', role: 'DeveloperWorker', repository: 'octo-org/external', provider: 'claude-cli', status: 'succeeded', model: 'glm-5.3-flash', effort: 'high', request_hash: 'hash', duration_seconds: 68, created_at: '2026-09-09T12:10:30Z',
      events: [
        { seq: 1, type: 'claude_session', occurred_at: '2026-09-09T12:10:30Z', payload: { model: 'glm-5.3-flash' } },
        { seq: 2, type: 'claude_usage', occurred_at: '2026-09-09T12:10:40Z', payload: { model: 'glm-5.3-flash', usage: { input_tokens: 0, output_tokens: 0 } } },
        { seq: 3, type: 'claude_result', occurred_at: '2026-09-09T12:11:38Z', payload: { subtype: 'success', total_cost_usd: 0.0987, usage: { input_tokens: 7061, output_tokens: 1749, cache_creation_input_tokens: 0, cache_read_input_tokens: 39360 } } },
      ],
    }
    vi.spyOn(adminApi, 'getAgentExecution').mockResolvedValue(detail)
    vi.spyOn(adminApi, 'listAgentExecutions').mockResolvedValue({
      items: [
        { execution_id: 'run-relay', run_id: 'run-relay', task_id: 'task-relay', task_type: 'develop_from_issue', role: 'DeveloperWorker', repository: 'octo-org/external', provider: 'claude-cli', status: 'succeeded', model: 'glm-5.3-flash', effort: 'high', request_hash: 'hash', duration_seconds: 68, created_at: '2026-09-09T12:10:30Z' },
      ],
      page: 1, page_size: 10, total: 1,
    })
    render(<MemoryRouter><AgentObservabilityPage /></MemoryRouter>)

    fireEvent.click(await screen.findByText('run-relay'))
    expect(await screen.findByText(/输入 46,421 · 其中缓存 39,360 · 输出 1,749/)).toBeInTheDocument()
    expect(screen.getByText('$0.0987')).toBeInTheDocument()
  })

  it('renders provider icons in list rows and execution details', async () => {
    const detail: AgentExecutionDetail = {
      execution_id: 'run-codex', run_id: 'run-codex', task_id: 'task-codex', task_type: 'review_mr', role: 'DeveloperWorker', repository: 'group/repo', provider: 'codex-cli', status: 'succeeded', model: 'gpt-alpha', effort: 'high', request_hash: 'hash', duration_seconds: 35, created_at: '2026-08-19T08:00:00Z',
      events: [],
    }
    vi.spyOn(adminApi, 'getAgentExecution').mockResolvedValue(detail)
    render(<MemoryRouter><AgentObservabilityPage /></MemoryRouter>)

    const codexRow = (await screen.findByText('run-codex')).closest('tr')!
    const claudeRow = screen.getByText('run-claude').closest('tr')!
    expect(codexRow.querySelector('.agent-icon--codex')).toBeInTheDocument()
    expect(codexRow.querySelector('.agent-icon--claude')).toBeNull()
    expect(claudeRow.querySelector('.agent-icon--claude')).toBeInTheDocument()
    expect(claudeRow.querySelector('.agent-icon--codex')).toBeNull()

    fireEvent.click(screen.getByText('run-codex'))
    const drawer = (await screen.findByRole('link', { name: '查看 Task' })).closest('.ant-drawer') as HTMLElement
    const providerRow = within(drawer).getByText('Provider').closest('tr')!
    expect(providerRow.querySelector('.agent-icon--codex')).toBeInTheDocument()
    expect(providerRow).toHaveTextContent('Codex')
    expect(providerRow).not.toHaveTextContent('codex-cli')
  })

  it('keeps provider filter values and clear behavior unchanged', async () => {
    render(<MemoryRouter><AgentObservabilityPage /></MemoryRouter>)
    await screen.findByText('run-claude')

    fireEvent.mouseDown(screen.getAllByRole('combobox')[0])
    const codexOption = (await screen.findAllByText('Codex')).find((element) => element.closest('.ant-select-item-option'))!
    const claudeOption = (await screen.findAllByText('Claude Code')).find((element) => element.closest('.ant-select-item-option'))!
    expect(codexOption.closest('.ant-select-item-option')!.querySelector('.agent-icon--codex')).toBeInTheDocument()
    expect(claudeOption.closest('.ant-select-item-option')!.querySelector('.agent-icon--claude')).toBeInTheDocument()

    fireEvent.click(codexOption)
    const selection = document.querySelector('.ant-select-selection-item')!
    expect(selection.querySelector('.agent-icon--codex')).toBeInTheDocument()
    expect(selection).toHaveTextContent('Codex')

    fireEvent.click(screen.getByRole('button', { name: /查\s*询/ }))
    await waitFor(() => expect(adminApi.listAgentExecutions).toHaveBeenLastCalledWith(expect.objectContaining({ provider: 'codex-cli' })))

    fireEvent.click(screen.getByRole('button', { name: /清\s*空/ }))
    await waitFor(() => expect(vi.mocked(adminApi.listAgentExecutions).mock.lastCall?.[0]).not.toHaveProperty('provider'))
    expect(await screen.findByText('Provider（全部）')).toBeInTheDocument()
    const providerSelect = screen.getByText('Provider（全部）').closest('.ant-select')!
    expect(providerSelect.querySelector('.ant-select-selection-item')).toBeNull()
  })

  it('preserves unknown and empty provider text without inferring from the model', async () => {
    vi.spyOn(adminApi, 'listAgentExecutions').mockResolvedValue({
      items: [
        { execution_id: 'run-unknown', run_id: 'run-unknown', task_id: 'task-unknown', task_type: 'review_mr', role: 'ReviewerWorker', repository: 'group/repo', provider: 'future-cli', status: 'succeeded', model: 'claude-alpha', effort: 'high', request_hash: 'hash', duration_seconds: 31, created_at: '2026-08-19T07:00:00Z' },
        { execution_id: 'run-empty', run_id: 'run-empty', task_id: 'task-empty', task_type: 'review_mr', role: 'DeveloperWorker', repository: 'group/repo', provider: '', status: 'succeeded', model: 'gpt-alpha', effort: 'high', request_hash: 'hash', duration_seconds: 32, created_at: '2026-08-19T07:30:00Z' },
      ],
      page: 1, page_size: 10, total: 2,
    })
    vi.spyOn(adminApi, 'getAgentExecution').mockResolvedValue({
      execution_id: 'run-unknown', run_id: 'run-unknown', task_id: 'task-unknown', task_type: 'review_mr', role: 'ReviewerWorker', repository: 'group/repo', provider: 'future-cli', status: 'succeeded', model: 'claude-alpha', effort: 'high', request_hash: 'hash', duration_seconds: 31, created_at: '2026-08-19T07:00:00Z',
      events: [],
    })
    render(<MemoryRouter><AgentObservabilityPage /></MemoryRouter>)

    const unknownRow = (await screen.findByText('run-unknown')).closest('tr')!
    expect(within(unknownRow).getByText('future-cli')).toBeInTheDocument()
    expect(unknownRow.querySelector('.agent-icon--fallback')).toBeInTheDocument()
    expect(unknownRow.querySelector('.agent-icon--claude')).toBeNull()

    const emptyRow = screen.getByText('run-empty').closest('tr')!
    expect(emptyRow.querySelectorAll('td')[2]).toHaveTextContent('—')
    expect(emptyRow.querySelector('.agent-icon')).toBeNull()

    fireEvent.click(screen.getByText('run-unknown'))
    const drawer = (await screen.findByRole('link', { name: '查看 Task' })).closest('.ant-drawer') as HTMLElement
    const providerRow = within(drawer).getByText('Provider').closest('tr')!
    expect(providerRow.querySelector('.agent-icon--fallback')).toBeInTheDocument()
    expect(providerRow).toHaveTextContent('future-cli')
  })

  it('renders the product name for a provider supplied by the URL and keeps the query string', async () => {
    render(
      <MemoryRouter initialEntries={['/agents?provider=claude-cli']}>
        <AgentObservabilityPage />
        <LocationProbe />
      </MemoryRouter>,
    )

    const selection = await waitFor(() => {
      const item = document.querySelector('.filter-panel .ant-select-selection-item')
      if (!item) throw new Error('provider selection not rendered')
      return item as HTMLElement
    })
    expect(selection).toHaveTextContent('Claude Code')
    expect(selection.querySelector('.agent-icon--claude')).toBeInTheDocument()
    expect(screen.getByTestId('location-search')).toHaveTextContent('?provider=claude-cli')
    await waitFor(() => expect(adminApi.listAgentExecutions).toHaveBeenCalledWith(expect.objectContaining({ provider: 'claude-cli' })))
  })

  it('derives drawer tokens from per-turn pi usage events', async () => {
    const detail: AgentExecutionDetail = {
      execution_id: 'run-pi', run_id: 'run-pi', task_id: 'task-pi', task_type: 'review_mr', role: 'ReviewerWorker', repository: 'group/project', provider: 'pi-cli', status: 'succeeded', model: 'deepseek-v4-flash', effort: 'high', request_hash: 'hash', duration_seconds: 42, created_at: '2026-11-20T10:00:00Z',
      events: [
        { seq: 1, type: 'pi_session', occurred_at: '2026-11-20T10:00:00Z', payload: { session_id: 'pi-session', cwd: '.' } },
        { seq: 2, type: 'pi_usage', occurred_at: '2026-11-20T10:00:10Z', payload: { usage: { input: 1200, output: 300, cacheRead: 800, cacheWrite: 0, totalTokens: 1500, cost: { total: 0 } }, stop_reason: 'toolUse' } },
        { seq: 3, type: 'pi_usage', occurred_at: '2026-11-20T10:00:20Z', payload: { usage: { input: 800, output: 200, cacheRead: 400, cacheWrite: 0, totalTokens: 1000, cost: { total: 0 } }, stop_reason: 'stop' } },
      ],
    }
    vi.spyOn(adminApi, 'getAgentExecution').mockResolvedValue(detail)
    vi.spyOn(adminApi, 'listAgentExecutions').mockResolvedValue({
      items: [
        { execution_id: 'run-pi', run_id: 'run-pi', task_id: 'task-pi', task_type: 'review_mr', role: 'ReviewerWorker', repository: 'group/project', provider: 'pi-cli', status: 'succeeded', model: 'deepseek-v4-flash', effort: 'high', request_hash: 'hash', duration_seconds: 42, created_at: '2026-11-20T10:00:00Z' },
      ],
      page: 1, page_size: 10, total: 1,
    })
    render(<MemoryRouter><AgentObservabilityPage /></MemoryRouter>)

    fireEvent.click(await screen.findByText('run-pi'))
    expect(await screen.findByText(/输入 2,000 · 其中缓存 1,200 · 输出 500/)).toBeInTheDocument()
    const drawer = (await screen.findByRole('link', { name: '查看 Task' })).closest('.ant-drawer') as HTMLElement
    const costRow = within(drawer).getByText('估算费用').closest('tr')!
    // 我们不下发模型价格，0 不是"免费"，费用保持未知。
    expect(costRow).toHaveTextContent('—')
  })

  it('colors error and usage events from every provider and defaults unknown to info', () => {
    const slots = THEMES.default.slots.eventType
    expect(eventTypeColor('codex_error', slots)).toBe(slots.error)
    expect(eventTypeColor('claude_error', slots)).toBe(slots.error)
    expect(eventTypeColor('codex_usage', slots)).toBe(slots.usage)
    expect(eventTypeColor('claude_usage', slots)).toBe(slots.usage)
    expect(eventTypeColor('pi_error', slots)).toBe(slots.error)
    expect(eventTypeColor('pi_usage', slots)).toBe(slots.usage)
    expect(eventTypeColor('claude_result', slots)).toBe(slots.info)
    expect(eventTypeColor('brand_new_event', slots)).toBe(slots.info)
  })
})
