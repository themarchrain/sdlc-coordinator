import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { adminApi } from '../api/client'
import type { AgentExecutionDetail, AgentExecutionSummary, StopStatus } from '../api/types'
import { WorkspaceSessionProvider } from '../app/workspaceSession'
import { TasksPage } from './TasksPage'
import { DataExplorerPage } from './DataExplorerPage'
import { AgentObservabilityPage } from './AgentObservabilityPage'

afterEach(() => vi.restoreAllMocks())

describe('stop drawers', () => {
  it.each(['task', 'run', 'execution'] as const)('stops the selected %s directly', async (target) => {
    const stopped: StopStatus = { stop_id: 'stop-drawer', scope: 'task', task_id: 'task-one', task_status: 'interrupted', delivery_status: 'confirmed', confirmation_kind: 'no_remote_target', exit_confirmed: false, requested_by: '17', requested_at: '2026-10-22T00:00:00Z' }
    const stop = vi.spyOn(adminApi, 'stopTarget').mockResolvedValue(stopped)
    const page = { items: [], page: 1, page_size: 20, total: 0 }
    vi.spyOn(adminApi, 'listTasks').mockResolvedValue(page)
    vi.spyOn(adminApi, 'taskTimeline').mockResolvedValue({ items: [] })
    vi.spyOn(adminApi, 'getTask').mockResolvedValueOnce({ task_id: 'task-one', status: 'running', can_stop: true }).mockResolvedValue({ task_id: 'task-one', status: 'interrupted', stop: stopped })
    vi.spyOn(adminApi, 'listEntities').mockResolvedValue(page)
    vi.spyOn(adminApi, 'getEntity').mockResolvedValueOnce({ run_id: 'run-one', task_id: 'task-one', status: 'running', can_stop: true }).mockResolvedValue({ run_id: 'run-one', status: 'interrupted', stop: stopped })
    vi.spyOn(adminApi, 'agentExecutionSummary').mockResolvedValue({ total_executions: 0, status_counts: {} } as AgentExecutionSummary)
    vi.spyOn(adminApi, 'listAgentExecutions').mockResolvedValue(page)
    const execution: AgentExecutionDetail = { execution_id: 'execution-one', run_id: 'run-one', task_id: 'task-one', task_type: 'develop_from_issue', role: 'DeveloperWorker', repository: 'group/repo', provider: 'fake', model: '', effort: '', request_hash: 'hash', created_at: '2026-10-22T00:00:00Z', status: 'running', events: [], can_stop: true }
    vi.spyOn(adminApi, 'getAgentExecution').mockResolvedValueOnce(execution).mockResolvedValue({ ...execution, status: 'cancelled', can_stop: false, stop: stopped })
    const path = target === 'task' ? '/tasks?task_id=task-one' : target === 'run' ? '/explorer?entity=runs&run_id=run-one' : '/agents?execution_id=execution-one'
    const Page = target === 'task' ? TasksPage : target === 'run' ? DataExplorerPage : AgentObservabilityPage
    render(<WorkspaceSessionProvider value={{ session: null, role: 'tenant_admin', superAdmin: false }}><MemoryRouter initialEntries={[path]}><Page /></MemoryRouter></WorkspaceSessionProvider>)
    fireEvent.click(await screen.findByRole('button', { name: /停\s*止/ }))
    expect(await screen.findByText('无需取消执行')).toBeInTheDocument()
    expect(stop).toHaveBeenCalledWith(target, `${target}-one`)
  })
})
