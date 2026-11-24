import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { ThemeProvider } from '../theme/ThemeProvider'
import { useTheme } from '../theme/useTheme'
import { AgentIcon } from './AgentIcon'

function ThemeSwitch() {
  const { setTheme } = useTheme()
  return <>
    <button type="button" onClick={() => setTheme('framer')}>to-framer</button>
    <button type="button" onClick={() => setTheme('default')}>to-default</button>
  </>
}

describe('AgentIcon', () => {
  beforeEach(() => {
    delete document.documentElement.dataset.theme
  })

  afterEach(() => {
    delete document.documentElement.dataset.theme
  })

  it('renders the pi brand mask without falling back to a glyph', () => {
    const { container } = render(<AgentIcon provider="pi-cli" />)

    expect(container.querySelector('.agent-icon--pi')).toHaveAttribute('aria-hidden', 'true')
    expect(container.querySelector('.agent-icon--pi')).toHaveStyle({ width: '14px', height: '14px', color: '#111111' })
    expect(container.querySelector('.agent-icon--fallback')).toBeNull()
  })

  it('renders the exact provider brands as decorative icons', () => {
    const { container } = render(<><AgentIcon provider="codex-cli" /><AgentIcon provider="claude-cli" /></>)

    const codex = container.querySelector('.agent-icon--codex')
    const claude = container.querySelector('.agent-icon--claude')
    expect(codex).toHaveAttribute('aria-hidden', 'true')
    expect(claude).toHaveAttribute('aria-hidden', 'true')
    expect(codex).toHaveStyle({ width: '14px', height: '14px' })
    expect(claude).toHaveStyle({ width: '14px', height: '14px' })
    expect(claude).toHaveStyle({ color: '#D97757' })
    expect(container.querySelectorAll('button, a, [tabindex]')).toHaveLength(0)
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })

  it('renders a robot only for unknown nonempty providers', () => {
    for (const provider of ['future-cli', 'constructor', 'Claude-CLI']) {
      const { container } = render(<AgentIcon provider={provider} />)
      expect(container.querySelector('.agent-icon--fallback')).toBeInTheDocument()
      expect(container.querySelector('.agent-icon--fallback')).toHaveAttribute('aria-hidden', 'true')
      expect(container.querySelector('.agent-icon--codex')).toBeNull()
      expect(container.querySelector('.agent-icon--claude')).toBeNull()
    }
  })

  it('renders nothing for absent providers', () => {
    for (const provider of ['', undefined, null]) {
      const { container } = render(<AgentIcon provider={provider} />)
      expect(container).toBeEmptyDOMElement()
    }
  })

  it('uses the default theme without a provider and reacts to theme changes', () => {
    const plain = render(<><AgentIcon provider="codex-cli" /><AgentIcon provider="claude-cli" /></>)
    expect(plain.container.querySelector('.agent-icon--codex')).toHaveStyle({ color: '#0D0D0D' })
    expect(plain.container.querySelector('.agent-icon--claude')).toHaveStyle({ color: '#D97757' })

    const { container, getByText } = render(<ThemeProvider><AgentIcon provider="codex-cli" /><AgentIcon provider="claude-cli" /><ThemeSwitch /></ThemeProvider>)
    expect(container.querySelector('.agent-icon--codex')).toHaveStyle({ color: '#0D0D0D' })

    fireEvent.click(getByText('to-framer'))
    expect(container.querySelector('.agent-icon--codex')).toHaveStyle({ color: '#FFFFFF' })
    expect(container.querySelector('.agent-icon--claude')).toHaveStyle({ color: '#E38963' })

    fireEvent.click(getByText('to-default'))
    expect(container.querySelector('.agent-icon--codex')).toHaveStyle({ color: '#0D0D0D' })
    expect(container.querySelector('.agent-icon--claude')).toHaveStyle({ color: '#D97757' })
  })
})
