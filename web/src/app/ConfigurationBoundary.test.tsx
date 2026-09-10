import { render, screen } from '@testing-library/react'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import { ConfigurationBoundary } from './ConfigurationBoundary'
import { WorkspaceSessionProvider } from './workspaceSession'

afterEach(() => vi.restoreAllMocks())

function LocationProbe() {
  const location = useLocation()
  return <span data-testid="location">{location.pathname}</span>
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
})
