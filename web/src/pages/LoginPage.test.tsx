import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import { LoginPage } from './LoginPage'

afterEach(() => vi.restoreAllMocks())

function LocationProbe() {
  const location = useLocation()
  return <span data-testid="location">{location.pathname}</span>
}

describe('LoginPage', () => {
  it('submits credentials and returns to the preserved admin target', async () => {
    const user = userEvent.setup()
    vi.spyOn(adminApi, 'login').mockResolvedValue({ username: 'admin' })

    render(
      <MemoryRouter initialEntries={['/login?return_to=%2Fpeople']}>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/people" element={<LocationProbe />} />
        </Routes>
      </MemoryRouter>,
    )

    await user.type(screen.getByLabelText('账号'), 'admin')
    await user.type(screen.getByLabelText('密码'), 'admin123')
    await user.click(screen.getByRole('button', { name: '登录管理台' }))

    expect(adminApi.login).toHaveBeenCalledWith({ username: 'admin', password: 'admin123' })
    expect(await screen.findByTestId('location')).toHaveTextContent('/people')
  })
})
