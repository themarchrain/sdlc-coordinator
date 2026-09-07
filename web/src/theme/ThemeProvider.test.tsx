import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'

import { THEMES, THEME_STORAGE_KEY } from '../themes/registry'
import { ThemeProvider, useThemeContext } from './ThemeProvider'

function Probe() {
  const context = useThemeContext()
  if (!context) throw new Error('Probe must render inside ThemeProvider')
  return (
    <div>
      <span data-testid="theme-id">{context.themeId}</span>
      <button type="button" onClick={() => context.setTheme('apple')}>switch-apple</button>
      <button type="button" onClick={() => context.setTheme('bogus-theme')}>switch-bogus</button>
    </div>
  )
}

describe('ThemeProvider', () => {
  beforeEach(() => {
    localStorage.clear()
    delete document.documentElement.dataset.theme
    document.documentElement.style.background = ''
  })

  it('initializes to default and applies data-theme when storage is empty', () => {
    render(<ThemeProvider><Probe /></ThemeProvider>)

    expect(screen.getByTestId('theme-id').textContent).toBe('default')
    expect(document.documentElement.dataset.theme).toBe('default')
  })

  it('falls back to default when storage holds an invalid theme id', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'not-a-theme')

    render(<ThemeProvider><Probe /></ThemeProvider>)

    expect(screen.getByTestId('theme-id').textContent).toBe('default')
    expect(document.documentElement.dataset.theme).toBe('default')
  })

  it('restores a valid stored theme id on mount', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'vercel')

    render(<ThemeProvider><Probe /></ThemeProvider>)

    expect(screen.getByTestId('theme-id').textContent).toBe('vercel')
    expect(document.documentElement.dataset.theme).toBe('vercel')
  })

  it('setTheme updates data-theme, meta theme-color and localStorage together', () => {
    const meta = document.createElement('meta')
    meta.setAttribute('name', 'theme-color')
    meta.setAttribute('content', THEMES.default.chromeColor)
    document.head.appendChild(meta)

    render(<ThemeProvider><Probe /></ThemeProvider>)
    fireEvent.click(screen.getByText('switch-apple'))

    expect(screen.getByTestId('theme-id').textContent).toBe('apple')
    expect(document.documentElement.dataset.theme).toBe('apple')
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('apple')
    expect(meta.getAttribute('content')).toBe(THEMES.apple.chromeColor)
    meta.remove()
  })

  it('setTheme with an invalid id falls back to default instead of crashing', () => {
    render(<ThemeProvider><Probe /></ThemeProvider>)
    fireEvent.click(screen.getByText('switch-apple'))
    fireEvent.click(screen.getByText('switch-bogus'))

    expect(screen.getByTestId('theme-id').textContent).toBe('default')
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('default')
  })
})
