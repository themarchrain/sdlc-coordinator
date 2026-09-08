import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { PortalApiError, portalApi } from '../api/portal'
import { AuthenticationBoundary } from './AuthenticationBoundary'

afterEach(() => vi.restoreAllMocks())

function LocationProbe() {
  const location = useLocation()
  return <span data-testid="location">{location.pathname}{location.search}</span>
}

describe('AuthenticationBoundary', () => {
  it('redirects unauthenticated visitors to portal login and preserves their target', async () => {
    vi.spyOn(portalApi, 'session').mockRejectedValue(new PortalApiError(401, 'unauthenticated', '请先登录'))

    render(
      <MemoryRouter initialEntries={['/tasks?run_id=run-1']}>
        <Routes>
          <Route path="/portal/login" element={<LocationProbe />} />
          <Route path="*" element={<AuthenticationBoundary><LocationProbe /></AuthenticationBoundary>} />
        </Routes>
      </MemoryRouter>,
    )

    expect(await screen.findByTestId('location')).toHaveTextContent('/portal/login?return_to=%2Ftasks%3Frun_id%3Drun-1')
  })

  it('renders protected content after portal session validation succeeds', async () => {
    vi.spyOn(portalApi, 'session').mockResolvedValue({
      user: { id: 9, username: 'root', role: 'super_admin', must_change_password: false },
      scope: { kind: 'super_admin', role: 'super_admin' },
    })

    render(<MemoryRouter><AuthenticationBoundary><span>后台内容</span></AuthenticationBoundary></MemoryRouter>)

    expect(await screen.findByText('后台内容')).toBeInTheDocument()
  })
})
