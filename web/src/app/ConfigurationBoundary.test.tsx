import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, useLocation, useNavigate } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import { ConfigurationBoundary } from './ConfigurationBoundary'
import { useConfigurationStatus } from './configurationStatus'
import { WorkspaceSessionProvider } from './workspaceSession'

afterEach(() => vi.restoreAllMocks())

function LocationProbe() {
  const location = useLocation()
  return <span data-testid="location">{location.pathname}</span>
}

function SyncHarness() {
  const location = useLocation()
  const navigate = useNavigate()
  const { sync } = useConfigurationStatus()
  return (
    <div>
      <span data-testid="location">{location.pathname}</span>
      <button onClick={() => sync({ mode: 'ready', revision: 2, source: 'ui' })}>sync-ready</button>
      <button onClick={() => sync({ mode: 'ready', revision: 2, source: 'ui' }, { fromRestart: true })}>sync-restart</button>
      <button onClick={() => navigate('/tasks')}>go-tasks</button>
      <button onClick={() => navigate('/setup')}>go-setup</button>
    </div>
  )
}

describe('ConfigurationBoundary', () => {
  it('redirects setup_pending deployments to the setup wizard', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'setup_pending', revision: 0 })

    render(
      <MemoryRouter initialEntries={['/tasks']}>
        <WorkspaceSessionProvider value={{ session: null, role: 'super_admin', superAdmin: true }}>
          <ConfigurationBoundary><LocationProbe /></ConfigurationBoundary>
        </WorkspaceSessionProvider>
      </MemoryRouter>,
    )

    expect(await screen.findByTestId('location')).toHaveTextContent('/setup')
  })

  it('redirects an initialized deployment away from setup', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 3 })

    render(
      <MemoryRouter initialEntries={['/setup']}>
        <WorkspaceSessionProvider value={{ session: null, role: 'super_admin', superAdmin: true }}>
          <ConfigurationBoundary><LocationProbe /></ConfigurationBoundary>
        </WorkspaceSessionProvider>
      </MemoryRouter>,
    )

    expect(await screen.findByTestId('location')).toHaveTextContent('/settings')
  })

  it('allows the person directory during setup pending mode', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'setup_pending', revision: 0 })

    render(
      <MemoryRouter initialEntries={['/people']}>
        <ConfigurationBoundary><LocationProbe /></ConfigurationBoundary>
      </MemoryRouter>,
    )

    expect(await screen.findByTestId('location')).toHaveTextContent('/people')
  })

  it('does not send members into the setup wizard', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'setup_pending', revision: 0 })

    render(
      <MemoryRouter initialEntries={['/tasks']}>
        <WorkspaceSessionProvider value={{ session: null, role: 'member', superAdmin: false }}>
          <ConfigurationBoundary><LocationProbe /></ConfigurationBoundary>
        </WorkspaceSessionProvider>
      </MemoryRouter>,
    )

    expect(await screen.findByTestId('location')).toHaveTextContent('/tasks')
  })

  it('applies a synced status to later navigation without a reload', async () => {
    // 复现 02-admin-setup-stale-configuration-boundary：配置页通过 sync
    // 回写共享状态后，边界必须按新状态放行后续导航，而不是继续按
    // 挂载时的 setup_pending 把路由弹回 /setup。
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'setup_pending', revision: 0 })

    render(
      <MemoryRouter initialEntries={['/setup']}>
        <WorkspaceSessionProvider value={{ session: null, role: 'super_admin', superAdmin: true }}>
          <ConfigurationBoundary><SyncHarness /></ConfigurationBoundary>
        </WorkspaceSessionProvider>
      </MemoryRouter>,
    )

    expect(await screen.findByTestId('location')).toHaveTextContent('/setup')

    fireEvent.click(screen.getByRole('button', { name: 'sync-ready' }))
    await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/settings'))

    fireEvent.click(screen.getByRole('button', { name: 'go-tasks' }))
    await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/tasks'))
  })

  it('lands on home when a synced status comes from the restart flow', async () => {
    // 重启完成（fromRestart）时落地路由归边界接管送首页；
    // 标记消费一次后失效，之后再进 /setup 仍送系统配置页。
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'setup_pending', revision: 0 })

    render(
      <MemoryRouter initialEntries={['/setup']}>
        <WorkspaceSessionProvider value={{ session: null, role: 'super_admin', superAdmin: true }}>
          <ConfigurationBoundary><SyncHarness /></ConfigurationBoundary>
        </WorkspaceSessionProvider>
      </MemoryRouter>,
    )

    expect(await screen.findByTestId('location')).toHaveTextContent('/setup')

    fireEvent.click(screen.getByRole('button', { name: 'sync-restart' }))
    await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/'))
    expect(screen.queryByTestId('location')).not.toHaveTextContent('/settings')

    fireEvent.click(screen.getByRole('button', { name: 'go-tasks' }))
    await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/tasks'))

    fireEvent.click(screen.getByRole('button', { name: 'go-setup' }))
    await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/settings'))
  })
})
