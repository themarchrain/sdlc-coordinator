import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it } from 'vitest'

import { MemberHomePage } from './MemberHomePage'

describe('MemberHomePage', () => {
  it('offers read-only shortcuts to tasks, explorer and agents', () => {
    render(<MemoryRouter><MemberHomePage /></MemoryRouter>)
    expect(screen.getByText('成员工作台')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '任务查询' })).toHaveAttribute('href', '/tasks')
    expect(screen.getByRole('link', { name: '数据查询' })).toHaveAttribute('href', '/explorer')
    expect(screen.getByRole('link', { name: 'Agent 运行' })).toHaveAttribute('href', '/agents')
    expect(screen.queryByText('仓库管理')).not.toBeInTheDocument()
  })
})
