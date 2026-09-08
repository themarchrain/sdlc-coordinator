import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { describe, expect, it } from 'vitest'

import { LoginPage } from './LoginPage'

function LocationProbe() {
  const location = useLocation()
  return <span data-testid="location">{location.pathname}{location.search}</span>
}

describe('LoginPage', () => {
  it('redirects the retired admin login to the portal login and preserves return_to', () => {
    render(
      <MemoryRouter initialEntries={['/login?return_to=%2Fpeople']}>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/portal/login" element={<LocationProbe />} />
        </Routes>
      </MemoryRouter>,
    )

    expect(screen.getByTestId('location')).toHaveTextContent('/portal/login?return_to=%2Fpeople')
  })
})
