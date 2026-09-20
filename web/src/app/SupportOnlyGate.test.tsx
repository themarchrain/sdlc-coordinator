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
// support_only，前端同步把业务页引导回首页；支持访问中保持可用。
describe('super admin business page gate', () => {
  it('redirects business pages to the home overview when support access is inactive', async () => {
    vi.spyOn(portalApi, 'session').mockResolvedValue(superAdminSession)
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    vi.spyOn(adminApi, 'listRepositories').mockResolvedValue({ items: [] })
    vi.spyOn(portalApi, 'superListTenants').mockResolvedValue({ tenants: [] })
    const tasksSpy = vi.spyOn(adminApi, 'listTasks').mockResolvedValue(emptyTasks)

    render(<MemoryRouter initialEntries={['/tasks']}><App /></MemoryRouter>)

    const homeLink = await screen.findByRole('link', { name: /系统总览/ })
    await waitFor(() => expect(homeLink).toHaveAttribute('aria-current', 'page'))
    expect(tasksSpy).not.toHaveBeenCalled()
  })

  it('redirects agent observability, operations and connectors deep links for solo super admins', async () => {
    vi.spyOn(portalApi, 'session').mockResolvedValue(superAdminSession)
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    vi.spyOn(adminApi, 'listRepositories').mockResolvedValue({ items: [] })
    vi.spyOn(portalApi, 'superListTenants').mockResolvedValue({ tenants: [] })
    const agentSummarySpy = vi.spyOn(adminApi, 'agentExecutionSummary').mockResolvedValue({
      total_executions: 0, status_counts: {}, average_duration_seconds: 0, p50_duration_seconds: 0,
      p90_duration_seconds: 0, input_tokens: 0, cached_input_tokens: 0, output_tokens: 0,
      cache_hit_ratio: 0, turn_count: 0, error_count: 0, timeout_count: 0, generated_at: '2026-09-01T00:00:00Z',
    })
    const entitiesSpy = vi.spyOn(adminApi, 'listEntities').mockResolvedValue({ items: [], total: 0, page: 1, page_size: 10 })
    const connectorsSpy = vi.spyOn(adminApi, 'listConnectors').mockResolvedValue({ items: [], total: 0, page: 1, page_size: 10 })

    for (const path of ['/agents', '/operations', '/connectors']) {
      const { unmount } = render(<MemoryRouter initialEntries={[path]}><App /></MemoryRouter>)
      const homeLink = await screen.findByRole('link', { name: /系统总览/ })
      await waitFor(() => expect(homeLink).toHaveAttribute('aria-current', 'page'))
      unmount()
    }

    expect(agentSummarySpy).not.toHaveBeenCalled()
    expect(entitiesSpy).not.toHaveBeenCalled()
    expect(connectorsSpy).not.toHaveBeenCalled()
  })

  it('keeps business pages usable during active support access', async () => {
    vi.spyOn(portalApi, 'session').mockResolvedValue({
      ...superAdminSession,
      support: { active: true, reason: '夜班验收', entered_at: '2026-09-07T00:00:00Z' },
    })
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    vi.spyOn(adminApi, 'listRepositories').mockResolvedValue({ items: [] })
    const tasksSpy = vi.spyOn(adminApi, 'listTasks').mockResolvedValue(emptyTasks)

    render(<MemoryRouter initialEntries={['/tasks']}><App /></MemoryRouter>)

    expect(await screen.findByRole('link', { name: /任务查询/ })).toHaveAttribute('aria-current', 'page')
    expect(tasksSpy).toHaveBeenCalled()
  })
})
