import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter, useSearchParams } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import { patchSearchParams } from '../app/searchParams'
import { TasksPage } from './TasksPage'

afterEach(() => vi.restoreAllMocks())

describe('TasksPage', () => {
  it('restores the status filter from the URL', async () => {
    const listTasks = vi.spyOn(adminApi, 'listTasks').mockResolvedValue({ items: [], page: 1, page_size: 20, total: 0 })

    renderTasks('/tasks?status=human_required')

    expect(await screen.findByText('需要人工')).toBeInTheDocument()
    expect(screen.queryByText('从内部 Task 定位当前 Run、错误原因和完整生命周期；列表不会加载大字段。')).not.toBeInTheDocument()
    expect(listTasks).toHaveBeenCalledWith(expect.objectContaining({ status: 'human_required' }))
  })

  it('opens a task drawer directly from task_id without losing list filters', async () => {
    vi.spyOn(adminApi, 'listTasks').mockResolvedValue({ items: [], page: 1, page_size: 20, total: 0 })
    vi.spyOn(adminApi, 'getTask').mockResolvedValue({ task_id: 'task-deep-link', status: 'running', role: 'DeveloperWorker', type: 'develop_from_issue' })
    vi.spyOn(adminApi, 'taskTimeline').mockResolvedValue({ items: [] })

    renderTasks('/tasks?status=running&task_id=task-deep-link')

    expect(await screen.findByText('task-deep-link')).toBeInTheDocument()
    expect(adminApi.getTask).toHaveBeenCalledWith('task-deep-link')
    expect(adminApi.taskTimeline).toHaveBeenCalledWith('task-deep-link')
    expect(adminApi.listTasks).toHaveBeenCalledWith(expect.objectContaining({ status: 'running' }))
  })

  it('renders typed task context and its SQLite-backed lifecycle on demand', async () => {
    vi.spyOn(adminApi, 'listTasks').mockResolvedValue({
      items: [{
        task_id: 'task-1', type: 'develop_from_issue', role: 'DeveloperWorker', status: 'human_required',
        source_repo: 'group/repo', target_repo: 'group/repo', current_run_id: 'run-1', last_error: '上下文不足',
        created_at: '2026-08-08T08:00:00Z', updated_at: '2026-08-08T08:10:00Z',
      }],
      page: 1, page_size: 20, total: 1,
    })
    vi.spyOn(adminApi, 'getTask').mockResolvedValue({
      task_id: 'task-1',
      type: 'develop_from_issue',
      role: 'DeveloperWorker',
      status: 'human_required',
      current_run_id: 'run-1',
      objective: '修复登录',
      acceptance_criteria: '登录成功且测试通过',
      context_json: JSON.stringify({
        issue: {
          iid: 12,
          title: '修复登录失败',
          description: '登录接口在特定账号下返回 500。',
          url: 'https://gitlab.example/group/repo/-/issues/12',
          assignees: [{ id: 7101, username: 'alice', name: 'Alice' }],
        },
        project: { id: '15594', path: 'group/repo' },
        repos: [{ name: 'repo', url: 'https://gitlab.example/group/repo.git', branch: 'main', access: 'writable' }],
        workspace_strategy: 'agent_managed_clone',
      }),
    })
    vi.spyOn(adminApi, 'taskTimeline').mockResolvedValue({ items: [
      { event_id: 'e1', occurred_at: '2026-08-08T08:00:00Z', category: 'task', action: 'created', status: 'pending', title: '任务已创建', summary: '', entity_type: 'task', entity_id: 'task-1' },
      { event_id: 'e2', occurred_at: '2026-08-08T08:10:00Z', category: 'run', action: 'blocked', status: 'human_required', title: '需要补充上下文', summary: '上下文不足', entity_type: 'run', entity_id: 'run-1' },
    ] })

    const { container } = renderTasks()
    expect(await screen.findByText('task-1')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /task-1/ })).not.toBeInTheDocument()
    fireEvent.click(screen.getByText('group/repo'))

    expect(await screen.findByText('修复登录')).toBeInTheDocument()
    expect(screen.getByText('Issue 信息')).toBeInTheDocument()
    expect(screen.getByText('修复登录失败')).toBeInTheDocument()
    expect(screen.getByText('登录接口在特定账号下返回 500。')).toBeInTheDocument()
    expect(screen.getByText(/Alice.*alice/)).toBeInTheDocument()
    expect(screen.getAllByText('group/repo').length).toBeGreaterThan(0)
    expect(screen.getByText('main')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '打开 Issue' })).toHaveAttribute('href', 'https://gitlab.example/group/repo/-/issues/12')
    expect(screen.getByText('原始任务数据')).toBeInTheDocument()
    expect(screen.getByText('任务已创建')).toBeInTheDocument()
    expect(screen.getByText('需要补充上下文')).toBeInTheDocument()
    expect(adminApi.getTask).toHaveBeenCalledWith('task-1')
    expect(adminApi.taskTimeline).toHaveBeenCalledWith('task-1')
    expect(container.querySelector('.admin-table--wide')).toBeInTheDocument()
    const taskRow = container.querySelector('.admin-table--row-click .ant-table-tbody > tr')!
    expect(window.getComputedStyle(taskRow).cursor).toBe('pointer')
    expect(container.querySelector('.table-pagination')).toBeInTheDocument()
  })

  it('shows a single repo path when source and target match', async () => {
    vi.spyOn(adminApi, 'listTasks').mockResolvedValue({
      items: [
        { task_id: 'task-same', type: 'review_mr', role: 'ReviewerWorker', status: 'succeeded', source_repo: 'alice/demo-repo', target_repo: 'alice/demo-repo', created_at: '2026-08-08T08:00:00Z', updated_at: '2026-08-08T08:10:00Z' },
        { task_id: 'task-diff', type: 'develop_from_issue', role: 'DeveloperWorker', status: 'succeeded', source_repo: 'source/repo', target_repo: 'target/repo', created_at: '2026-08-08T08:00:00Z', updated_at: '2026-08-08T08:10:00Z' },
      ],
      page: 1, page_size: 20, total: 2,
    })

    renderTasks()

    expect(await screen.findByText('task-same')).toBeInTheDocument()
    expect(screen.getAllByText('alice/demo-repo')).toHaveLength(1)
    expect(screen.queryByText(/→ alice\/test-hooks/)).not.toBeInTheDocument()
    expect(screen.getByText(/source\/repo/)).toBeInTheDocument()
    expect(screen.getByText('→ target/repo')).toBeInTheDocument()
  })

  it('uses selects for stable task filters', async () => {
    vi.spyOn(adminApi, 'listTasks').mockResolvedValue({ items: [], page: 1, page_size: 20, total: 0 })

    renderTasks()

    expect((await screen.findByText('任务状态')).closest('.ant-select')?.querySelector('[role="combobox"]')).toBeInTheDocument()
    expect(screen.getByText('Worker 角色').closest('.ant-select')?.querySelector('[role="combobox"]')).toBeInTheDocument()
    expect(screen.getByText('任务类型').closest('.ant-select')?.querySelector('[role="combobox"]')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('仓库路径')).not.toHaveAttribute('role', 'combobox')

    fireEvent.mouseDown(screen.getByText('任务状态').closest('.ant-select')!.querySelector('.ant-select-selector')!)
    expect(await screen.findByText('等待合并')).toBeInTheDocument()

    fireEvent.mouseDown(screen.getByText('任务类型').closest('.ant-select')!.querySelector('.ant-select-selector')!)
    expect(await screen.findByText('上游修复')).toBeInTheDocument()
  })

  it('passes repository_id from the URL and resets pagination when it changes', async () => {
    const listTasks = vi.spyOn(adminApi, 'listTasks').mockResolvedValue({
      items: [{
        task_id: 'task-1', type: 'develop_from_issue', role: 'DeveloperWorker', status: 'running',
        source_repo: 'group/repo', target_repo: 'group/repo', created_at: '2026-08-08T08:00:00Z', updated_at: '2026-08-08T08:10:00Z',
      }],
      page: 1, page_size: 10, total: 21,
    })

    renderTasks('/tasks?status=running&repository_id=repo_a')

    expect(await screen.findByText('task-1')).toBeInTheDocument()
    expect(listTasks).toHaveBeenCalledWith(expect.objectContaining({ status: 'running', repository_id: 'repo_a', page: 1 }))

    fireEvent.click(screen.getByTitle('2'))
    expect(listTasks).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2, repository_id: 'repo_a' }))

    fireEvent.click(screen.getByRole('button', { name: 'select-repo-b' }))
    expect(listTasks).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, repository_id: 'repo_b', status: 'running' }))
  })

  it('omits the status tag for timeline events without a status', async () => {
    vi.spyOn(adminApi, 'listTasks').mockResolvedValue({
      items: [{ task_id: 'task-statusless', type: 'develop_from_issue', role: 'DeveloperWorker', status: 'running', source_repo: 'group/repo', target_repo: 'group/repo', current_run_id: 'run-1', created_at: '2026-08-08T08:00:00Z', updated_at: '2026-08-08T08:10:00Z' }],
      page: 1, page_size: 20, total: 1,
    })
    vi.spyOn(adminApi, 'getTask').mockResolvedValue({ task_id: 'task-statusless', status: 'running', role: 'DeveloperWorker', type: 'develop_from_issue' })
    vi.spyOn(adminApi, 'taskTimeline').mockResolvedValue({ items: [
      { event_id: 'e1', occurred_at: '2026-08-08T08:00:00Z', category: 'task', action: 'created', status: '', title: 'Task 已创建', summary: '', entity_type: 'task', entity_id: 'task-statusless' },
      { event_id: 'e2', occurred_at: '2026-08-08T08:10:00Z', category: 'run', action: 'completed', status: 'succeeded', title: 'Run 已结束', summary: '', entity_type: 'run', entity_id: 'run-1' },
    ] })

    renderTasks()
    expect(await screen.findByText('task-statusless')).toBeInTheDocument()
    fireEvent.click(screen.getByText('group/repo'))

    expect(await screen.findByText('Task 已创建')).toBeInTheDocument()
    expect(screen.getByText('Run 已结束')).toBeInTheDocument()
    expect(screen.getByText('succeeded')).toBeInTheDocument()
    expect(screen.queryByText('unknown')).not.toBeInTheDocument()
  })
})

function RepositorySwitchProbe() {
  const [searchParams, setSearchParams] = useSearchParams()
  return <button type="button" onClick={() => setSearchParams(patchSearchParams(searchParams, { repository_id: 'repo_b' }))}>select-repo-b</button>
}

function renderTasks(path = '/tasks') {
  return render(<MemoryRouter initialEntries={[path]}><RepositorySwitchProbe /><TasksPage /></MemoryRouter>)
}
