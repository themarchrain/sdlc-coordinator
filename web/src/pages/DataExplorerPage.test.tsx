import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import { DataExplorerPage } from './DataExplorerPage'

afterEach(() => vi.restoreAllMocks())

describe('DataExplorerPage', () => {
  it('offers the interrupted run status filter', async () => {
    vi.spyOn(adminApi, 'listEntities').mockResolvedValue({ items: [], page: 1, page_size: 20, total: 0 })
    renderExplorer('/explorer?entity=runs')
    fireEvent.mouseDown(screen.getAllByRole('combobox')[1])
    expect(await screen.findByTitle('已中断')).toBeInTheDocument()
  })
  it('restores the entity and opens a run detail from a deep link', async () => {
    vi.spyOn(adminApi, 'listEntities').mockResolvedValue({ items: [], page: 1, page_size: 20, total: 0 })
    vi.spyOn(adminApi, 'getEntity').mockResolvedValue({
      run_id: 'run-deep-link', task_id: 'task-1', worker_role: 'ReviewerWorker', status: 'running', external_id: 'conversation-1',
    })

    renderExplorer('/explorer?entity=runs&run_id=run-deep-link')

    expect(await screen.findByText('run-deep-link')).toBeInTheDocument()
    expect(screen.getByText('当前没有匹配的执行记录，试试清空筛选后重试。')).toBeInTheDocument()
    expect(adminApi.getEntity).toHaveBeenCalledWith('runs', 'run-deep-link')
  })

  it('passes repository_id from the URL into entity list requests', async () => {
    const listEntities = vi.spyOn(adminApi, 'listEntities').mockResolvedValue({ items: [], page: 1, page_size: 20, total: 0 })

    renderExplorer('/explorer?entity=runs&repository_id=repo_a')

    await screen.findByText('当前没有匹配的执行记录，试试清空筛选后重试。')
    expect(listEntities).toHaveBeenCalledWith('runs', expect.objectContaining({ repository_id: 'repo_a', page: 1 }))
  })

  it('renders localized entity guidance and structured run detail on demand', async () => {
    vi.spyOn(adminApi, 'listEntities').mockImplementation(async (entity) => ({
      items: entity === 'runs'
        ? [{ run_id: 'run-1', task_id: 'task-1', status: 'failed', worker_role: 'DeveloperWorker', external_system: 'agent-runner', external_id: 'conversation-1', started_at: '2026-08-08T08:00:00Z', completed_at: '2026-08-08T08:04:00Z', created_at: '2026-08-08T08:00:00Z' }]
        : [{ effect_id: 'effect-1', task_id: 'task-1', status: 'failed', kind: 'send_im_message', created_at: '2026-08-08T08:00:00Z' }],
      page: 1, page_size: 20, total: 1,
    }))
    vi.spyOn(adminApi, 'getEntity').mockResolvedValue({
      run_id: 'run-1',
      task_id: 'task-1',
      worker_role: 'DeveloperWorker',
      status: 'succeeded',
      external_system: 'agent-runner',
      external_id: 'conversation-1',
      external_url: 'http://agent-runner.example/executions/conversation-1',
      started_at: '2026-08-08T08:00:00Z',
      completed_at: '2026-08-08T08:04:00Z',
      input_json: JSON.stringify({
        type: 'develop_from_issue',
        objective: '修复登录失败',
        workspace_strategy: 'agent_managed_clone',
        repos: [{ name: 'repo', branch: 'main', access: 'writable', url: 'https://gitlab.example/group/repo.git' }],
      }),
      output_json: JSON.stringify({
        status: 'succeeded',
        summary: '已修复并通过测试',
        artifacts: {
          commit_sha: 'abc123456789',
          delivery_branch: 'alice/issue-12-20260808',
          changed_files: ['src/login.ts'],
          test_results: [{ command: 'npm test', passed: true, summary: '12 tests passed' }],
        },
        writebacks: [{ kind: 'create_merge_request', project_id: 'group/repo' }],
        effects: [{ kind: 'send_im_message', target: 'default' }],
      }),
    })

    const { container } = renderExplorer()

    expect(await screen.findByText('协调数据查询')).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '运行记录' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /更多记录/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /查\s*询/ })).toBeInTheDocument()

    expect(await screen.findByText('DeveloperWorker')).toBeInTheDocument()
    fireEvent.mouseEnter(screen.getByRole('button', { name: '关于 角色' }))
    expect(await screen.findByText(/修改代码并创建 MR/)).toBeInTheDocument()
    expect(screen.getByText(/对 MR 进行代码审查/)).toBeInTheDocument()
    expect(screen.getByText('conversation-1')).toBeInTheDocument()
    expect(screen.getByText('4.0 分钟')).toBeInTheDocument()

    fireEvent.mouseEnter(screen.getByRole('button', { name: '关于 运行记录' }))
    expect(await screen.findByText(/一次运行 = 一次 Worker 执行/)).toBeInTheDocument()
    expect(screen.getByText(/对应一个 Agent Runner Execution/)).toBeInTheDocument()

    fireEvent.click(await screen.findByText('task-1'))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('角色 DeveloperWorker')).toBeInTheDocument()
    expect(within(dialog).getByText('系统 agent-runner')).toBeInTheDocument()
    expect(within(dialog).getByText(/耗时 4.0 分钟/)).toBeInTheDocument()
    expect(await screen.findByText('执行输入')).toBeInTheDocument()
    expect(screen.getByText('修复登录失败')).toBeInTheDocument()
    expect(screen.getByText('执行结果')).toBeInTheDocument()
    expect(screen.getByText('已修复并通过测试')).toBeInTheDocument()
    expect(screen.getByText('abc123456789')).toBeInTheDocument()
    expect(screen.getByText('12 tests passed')).toBeInTheDocument()
    expect(screen.getByText('写回意图')).toBeInTheDocument()
    expect(screen.getByText('create_merge_request')).toBeInTheDocument()
    expect(screen.getByText('副作用意图')).toBeInTheDocument()
    expect(screen.getByText('send_im_message')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '打开外部执行' })).toHaveAttribute('href', 'http://agent-runner.example/executions/conversation-1')
    expect(screen.getByText('原始 Run 数据')).toBeInTheDocument()
    expect(adminApi.getEntity).toHaveBeenCalledTimes(1)

    fireEvent.click(screen.getByRole('button', { name: /更多记录/ }))
    fireEvent.click(await screen.findByRole('menuitem', { name: '通知与副作用' }))
    expect(await screen.findByText('effect-1')).toBeInTheDocument()
    fireEvent.mouseEnter(screen.getByRole('button', { name: '关于 通知与副作用' }))
    expect(await screen.findByText(/IM 通知等非核心副作用/)).toBeInTheDocument()
    expect(screen.queryByText('查看 IM 通知等非核心副作用的执行、失败与重试情况。')).not.toBeInTheDocument()
    expect(screen.getByText('Effect 类型').closest('.ant-select')?.querySelector('[role="combobox"]')).toBeInTheDocument()
    expect(screen.getByText('执行状态').closest('.ant-select')?.querySelector('[role="combobox"]')).toBeInTheDocument()
    fireEvent.mouseDown(screen.getByText('Effect 类型').closest('.ant-select')!.querySelector('.ant-select-selector')!)
    expect(await screen.findByText('发送 IM 消息')).toBeInTheDocument()
    expect(screen.queryByText('更新外部任务')).not.toBeInTheDocument()
    expect(container.querySelector('.admin-table--wide')).toBeInTheDocument()
    expect(container.querySelector('.table-pagination')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /更多记录/ }))
    fireEvent.click(await screen.findByRole('menuitem', { name: '入站事件' }))
    fireEvent.mouseDown(screen.getByText('处理状态').closest('.ant-select')!.querySelector('.ant-select-selector')!)
    expect(await screen.findByText('已创建 Task')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /更多记录/ }))
    fireEvent.click(await screen.findByRole('menuitem', { name: '结果回传' }))
    fireEvent.mouseDown(screen.getByText('回传来源').closest('.ant-select')!.querySelector('.ant-select-selector')!)
    expect(await screen.findByText('MCP')).toBeInTheDocument()
    expect(screen.getByText('HTTP Callback')).toBeInTheDocument()
    expect(screen.getByText('Polling')).toBeInTheDocument()
  }, 15000) // 全量并行时该用例贴近 5s 默认上限，放宽避免负载敏感超时
  it('shows a dash for unstarted runs instead of their created time', async () => {
    vi.spyOn(adminApi, 'listEntities').mockResolvedValue({
      items: [{ run_id: 'run-2', task_id: 'task-2', status: 'pending', worker_role: 'ReviewerWorker', external_system: 'agent-runner', created_at: '2026-08-08T09:00:00Z' }],
      page: 1, page_size: 20, total: 1,
    })
    vi.spyOn(adminApi, 'getEntity').mockResolvedValue({ run_id: 'run-2' })

    renderExplorer()

    const row = (await screen.findByText('task-2')).closest('tr')!
    const cells = within(row).getAllByRole('cell')
    expect(cells[4]).toHaveTextContent('—')
    const created = new Date('2026-08-08T09:00:00Z').toLocaleString('zh-CN', { hour12: false })
    expect(within(row).queryByText(created)).not.toBeInTheDocument()
  })

  it('renders secondary entity ids as plain text and normalizes an invalid entity back into the url', async () => {
    vi.spyOn(adminApi, 'listEntities').mockImplementation(async (entity) => ({
      items: entity === 'runs' ? [] : [{ effect_id: 'effect-1', task_id: 'task-1', status: 'failed', kind: 'send_im_message', created_at: '2026-08-08T08:00:00Z' }],
      page: 1, page_size: 20, total: 1,
    }))
    vi.spyOn(adminApi, 'getEntity').mockResolvedValue({ effect_id: 'effect-1' })

    renderExplorer('/explorer?entity=bogus')

    expect(await screen.findByText('当前没有匹配的执行记录，试试清空筛选后重试。')).toBeInTheDocument()
    await waitFor(() => expect(screen.getByTestId('location-probe')).toHaveTextContent('entity=runs'))

    fireEvent.click(screen.getByRole('button', { name: /更多记录/ }))
    fireEvent.click(await screen.findByRole('menuitem', { name: '通知与副作用' }))
    expect(await screen.findByText('effect-1')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'effect-1' })).not.toBeInTheDocument()
  })
})

function LocationProbe() {
  const { search } = useLocation()
  return <span data-testid="location-probe">{search}</span>
}

function renderExplorer(path = '/explorer') {
  return render(<MemoryRouter initialEntries={[path]}><DataExplorerPage /><LocationProbe /></MemoryRouter>)
}
