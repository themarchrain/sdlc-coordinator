import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { AdminApiError, adminApi } from '../api/client'
import type { PortalSessionPayload } from '../api/portal'
import type { RecoveryAssessment } from '../api/types'
import { WorkspaceSessionProvider, type WorkspaceSessionValue } from '../app/workspaceSession'
import { TaskRecoveryControl } from './TaskRecoveryControl'

afterEach(() => vi.restoreAllMocks())

const session: PortalSessionPayload = {
  user: { id: 17, username: 'member', role: 'member', must_change_password: false },
  scope: { kind: 'tenant', role: 'member', tenant: { id: 1, slug: 'acme', display_name: 'Acme', status: 'active' } },
}
const persona: WorkspaceSessionValue = { session, role: 'member', superAdmin: false }

const transient: RecoveryAssessment = {
  task_id: 'task-1', classification: 'transient', eligible: true, requires_risk_confirmation: false,
  reason_code: 'eligible', reason_summary: '外部故障已恢复', block_revision: 'revision-1',
  failure_summary: '上游返回 503', safety_blockers: [],
}

function renderControl(assessment: RecoveryAssessment, taskID = 'task-1', status = 'blocked', value: WorkspaceSessionValue = persona) {
  return render(<WorkspaceSessionProvider value={value}><TaskRecoveryControl taskID={taskID} status={status} onRecovered={() => {}} /></WorkspaceSessionProvider>)
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail })
  return { promise, resolve, reject }
}

describe('TaskRecoveryControl', () => {
  it('shows member recovery without granting stop', async () => {
    vi.spyOn(adminApi, 'taskRecovery').mockResolvedValue(transient)
    renderControl(transient)

    expect(await screen.findByRole('button', { name: '恢复任务' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /停\s*止/ })).not.toBeInTheDocument()
  })

  it('requires explicit unknown risk and reason', async () => {
    const unknown = { ...transient, classification: 'unknown', requires_risk_confirmation: true, reason_summary: '无法判定失败原因' }
    vi.spyOn(adminApi, 'taskRecovery').mockResolvedValue(unknown)
    const recover = vi.spyOn(adminApi, 'recoverTask').mockResolvedValue({ replayed: false, recovery: { recovery_id: 'r1', task_id: 'task-1', request_id: 'req', source_generation: 0, target_generation: 1, block_revision: 'revision-1', requested_by: '17', requested_at: '2026-11-10T00:00:00Z', risk_acknowledged: true, classification: 'unknown', attempt_count_before: 2, cycle_max_attempts: 3, created_at: '2026-11-10T00:00:00Z' }, task: { task_id: 'task-1', status: 'pending' } })
    renderControl(unknown)

    fireEvent.click(await screen.findByRole('button', { name: '风险恢复' }))
    expect(screen.getByText(/系统无法判定失败原因，恢复后可能立即再次阻塞/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '确认恢复' })).toBeDisabled()
    fireEvent.click(screen.getByLabelText('我已了解并确认风险'))
    fireEvent.change(screen.getByLabelText('恢复说明'), { target: { value: '已确认可以承担风险' } })
    fireEvent.click(screen.getByRole('button', { name: '确认恢复' }))

    await waitFor(() => expect(recover).toHaveBeenCalledWith('task-1', expect.objectContaining({ acknowledge_unknown_risk: true, reason: '已确认可以承担风险', block_revision: 'revision-1' })))
  })

  it('blocks fatal and safety-unknown recoveries', async () => {
    const fatal = { ...transient, classification: 'fatal', eligible: false, reason_code: 'fatal_not_recoverable', reason_summary: '失败不可恢复', safety_blockers: [{ code: 'execution_unconfirmed', summary: '前次执行未确认退出', remediation: '先完成对账' }] }
    vi.spyOn(adminApi, 'taskRecovery').mockResolvedValue(fatal)
    renderControl(fatal)

    expect(await screen.findByText('失败不可恢复')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '恢复任务' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '风险恢复' })).not.toBeInTheDocument()
    expect(screen.getByText(/前次执行未确认退出/)).toBeInTheDocument()
  })

  it('displays the original redacted error as text', async () => {
    vi.spyOn(adminApi, 'taskRecovery').mockResolvedValue({ ...transient, failure_summary: 'token=*** provider timeout' })
    renderControl(transient)

    expect(await screen.findByText('token=*** provider timeout')).toBeInTheDocument()
  })

  it('does not request on cancel and clears risk state', async () => {
    const recover = vi.spyOn(adminApi, 'recoverTask')
    vi.spyOn(adminApi, 'taskRecovery').mockResolvedValue({ ...transient, requires_risk_confirmation: true })
    renderControl(transient)
    fireEvent.click(await screen.findByRole('button', { name: '风险恢复' }))
    fireEvent.click(screen.getByRole('button', { name: /取\s*消/ }))
    expect(recover).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: '风险恢复' }))
    expect(screen.getByLabelText('我已了解并确认风险')).not.toBeChecked()
  })

  it('reports accepted recovery without claiming it is running', async () => {
    const recovered = { replayed: false, recovery: { recovery_id: 'r1', task_id: 'task-1', request_id: 'req', source_generation: 0, target_generation: 1, block_revision: 'revision-1', requested_by: '17', requested_at: '2026-11-10T00:00:00Z', risk_acknowledged: false, classification: 'transient', attempt_count_before: 2, cycle_max_attempts: 3, created_at: '2026-11-10T00:00:00Z' }, task: { task_id: 'task-1', status: 'pending' } }
    const onRecovered = vi.fn()
    vi.spyOn(adminApi, 'taskRecovery').mockResolvedValue(transient)
    vi.spyOn(adminApi, 'recoverTask').mockResolvedValue(recovered)
    render(<WorkspaceSessionProvider value={persona}><TaskRecoveryControl taskID="task-1" status="blocked" onRecovered={onRecovered} /></WorkspaceSessionProvider>)
    fireEvent.click(await screen.findByRole('button', { name: '恢复任务' }))

    expect(await screen.findByText('恢复已受理')).toBeInTheDocument()
    expect(screen.queryByText(/已运行/)).not.toBeInTheDocument()
    expect(onRecovered).toHaveBeenCalledTimes(1)
  })

  it('refreshes assessment after a 409', async () => {
    const load = vi.spyOn(adminApi, 'taskRecovery').mockResolvedValueOnce(transient).mockResolvedValueOnce({ ...transient, block_revision: 'revision-2' })
    vi.spyOn(adminApi, 'recoverTask').mockRejectedValue(new AdminApiError(409, 'task_recovery_stale', '任务状态已变化'))
    renderControl(transient)
    fireEvent.click(await screen.findByRole('button', { name: '恢复任务' }))

    await waitFor(() => expect(load).toHaveBeenCalledTimes(2))
  })

  it('allows retrying after a 503 and sends only one request per click sequence', async () => {
    vi.spyOn(adminApi, 'taskRecovery').mockResolvedValue(transient)
    const recover = vi.spyOn(adminApi, 'recoverTask').mockRejectedValueOnce(new AdminApiError(503, 'recovery_check_unavailable', '暂不可用'))
    renderControl(transient)
    fireEvent.click(await screen.findByRole('button', { name: '恢复任务' }))
    expect(await screen.findByText('恢复安全检查暂不可用，可重试')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /重\s*试/ }))
    await waitFor(() => expect(recover).toHaveBeenCalledTimes(2))
  })

  it('ignores a late assessment after switching Tasks', async () => {
    const old = deferred<RecoveryAssessment>()
    vi.spyOn(adminApi, 'taskRecovery').mockImplementation((id) => id === 'task-1' ? old.promise : Promise.resolve({ ...transient, task_id: 'task-2' }))
    const view = renderControl(transient)
    view.rerender(<WorkspaceSessionProvider value={persona}><TaskRecoveryControl taskID="task-2" status="blocked" onRecovered={() => {}} /></WorkspaceSessionProvider>)
    expect(await screen.findByText('外部故障已恢复')).toBeInTheDocument()
    await act(async () => old.resolve({ ...transient, reason_summary: '旧任务资格' }))
    expect(screen.queryByText('旧任务资格')).not.toBeInTheDocument()
  })

  it('ignores a late recovery after the task scope changes', async () => {
    const pending = deferred<Awaited<ReturnType<typeof adminApi.recoverTask>>>()
    const onRecovered = vi.fn()
    vi.spyOn(adminApi, 'taskRecovery').mockResolvedValue(transient)
    vi.spyOn(adminApi, 'recoverTask').mockReturnValue(pending.promise)
    const view = render(<WorkspaceSessionProvider value={persona}><TaskRecoveryControl taskID="task-1" status="blocked" onRecovered={onRecovered} /></WorkspaceSessionProvider>)
    fireEvent.click(await screen.findByRole('button', { name: '恢复任务' }))
    view.rerender(<WorkspaceSessionProvider value={persona}><TaskRecoveryControl taskID="task-2" status="blocked" onRecovered={onRecovered} /></WorkspaceSessionProvider>)
    await act(async () => pending.resolve({ replayed: false, recovery: { recovery_id: 'r1' } as never, task: {} }))
    expect(onRecovered).not.toHaveBeenCalled()
    expect(screen.queryByText('恢复已受理')).not.toBeInTheDocument()
  })
})
