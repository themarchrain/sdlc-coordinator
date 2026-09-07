import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { AdminApiError, adminApi } from '../api/client'
import { AuthenticationBoundary } from './AuthenticationBoundary'

afterEach(() => vi.restoreAllMocks())

function LocationProbe() {
  const location = useLocation()
  return <span data-testid="location">{location.pathname}{location.search}</span>
}

describe('AuthenticationBoundary', () => {
  it('redirects unauthenticated visitors to login and preserves their target', async () => {
    vi.spyOn(adminApi, 'session').mockRejectedValue(new AdminApiError(401, 'unauthenticated', '请先登录管理台'))

    render(
      <MemoryRouter initialEntries={['/tasks?run_id=run-1']}>
        <Routes>
          <Route path="/login" element={<LocationProbe />} />
          <Route path="*" element={<AuthenticationBoundary><LocationProbe /></AuthenticationBoundary>} />
        </Routes>
      </MemoryRouter>,
    )

    expect(await screen.findByTestId('location')).toHaveTextContent('/login?return_to=%2Ftasks%3Frun_id%3Drun-1')
  })

  it('renders protected content after session validation succeeds', async () => {
    vi.spyOn(adminApi, 'session').mockResolvedValue({ username: 'admin', expires_at: '2026-08-18T09:00:00Z' })

    render(<MemoryRouter><AuthenticationBoundary><span>后台内容</span></AuthenticationBoundary></MemoryRouter>)

    expect(await screen.findByText('后台内容')).toBeInTheDocument()
  })
})
