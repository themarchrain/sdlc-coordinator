import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import { portalApi, type PortalSessionPayload } from '../api/portal'
import { App } from './App'

afterEach(() => vi.restoreAllMocks())

const superAdminSession: PortalSessionPayload = {
  user: { id: 1, username: 'root', role: 'super_admin', must_change_password: false },
  scope: { kind: 'super_admin', role: 'super_admin' },
}

const emptyTasks = { items: [], total: 0, page: 1, page_size: 10 }

// 业务页面对无支持访问的超管重定向——后端 API 已收窄为
// support_only，前端同步把五类业务页引导回运行总览；支持访问中保持可用。
describe('super admin business page gate', () => {
  it('redirects business pages to the overview when support access is inactive', async () => {
    vi.spyOn(portalApi, 'session').mockResolvedValue(superAdminSession)
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    const tasksSpy = vi.spyOn(adminApi, 'listTasks').mockResolvedValue(emptyTasks)

    render(<MemoryRouter initialEntries={['/tasks']}><App /></MemoryRouter>)

    const overviewLink = await screen.findByRole('link', { name: /运行总览/ })
    await waitFor(() => expect(overviewLink).toHaveAttribute('aria-current', 'page'))
    expect(tasksSpy).not.toHaveBeenCalled()
  })

  it('keeps business pages usable during active support access', async () => {
    vi.spyOn(portalApi, 'session').mockResolvedValue({
      ...superAdminSession,
      support: { active: true, reason: '夜班验收', entered_at: '2026-09-07T00:00:00Z' },
    })
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    const tasksSpy = vi.spyOn(adminApi, 'listTasks').mockResolvedValue(emptyTasks)

    render(<MemoryRouter initialEntries={['/tasks']}><App /></MemoryRouter>)

    expect(await screen.findByRole('link', { name: /任务查询/ })).toHaveAttribute('aria-current', 'page')
    expect(tasksSpy).toHaveBeenCalled()
  })
})
