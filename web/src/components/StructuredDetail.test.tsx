import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { TaskContextDetail } from './StructuredDetail'

describe('TaskContextDetail', () => {
  it('renders review merge request context', () => {
    render(<TaskContextDetail value={{
      merge_request: {
        iid: 24,
        title: '修复登录',
        source_branch: 'fix/login',
        target_branch: 'master',
        head_sha: 'abc123',
        author: { name: 'Alice', username: 'alice', email: 'alice@example.com' },
        url: 'https://gitlab.example/group/repo/-/merge_requests/24',
      },
      project: { id: '15594', path: 'group/repo' },
    }} />)

    expect(screen.getByText('MR 信息')).toBeInTheDocument()
    expect(screen.getByText('fix/login')).toBeInTheDocument()
    expect(screen.getByText(/Alice.*alice/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '打开 MR' })).toBeInTheDocument()
  })

  it('renders rework request and generated task lineage', () => {
    render(<TaskContextDetail value={{
      merge_request: { iid: 23, title: '接续修改', source_branch: 'fix/rework', target_branch: 'master' },
      origin_issue: { iid: 5, title: '原始需求' },
      rework_request: { context: '补充边界测试', note_id: 172223, requester: { username: 'reviewer' } },
      parent_task_id: 'task-parent',
      parent_run_id: 'run-parent',
      next_task_index: 1,
      allowed_repos: { writable: ['group/repo-a'], readable: ['group/repo-b'] },
    }} />)

    expect(screen.getByText('接续开发请求')).toBeInTheDocument()
    expect(screen.getByText('补充边界测试')).toBeInTheDocument()
    expect(screen.getByText('派生任务信息')).toBeInTheDocument()
    expect(screen.getByText('task-parent')).toBeInTheDocument()
    expect(screen.getByText('group/repo-a')).toBeInTheDocument()
  })
})
