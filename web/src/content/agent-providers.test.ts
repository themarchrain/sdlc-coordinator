import { describe, expect, it } from 'vitest'

import { agentProviderLabel } from './agent-providers'

describe('agentProviderLabel', () => {
  it('maps the two known provider codes to their product names', () => {
    expect(agentProviderLabel('codex-cli')).toBe('Codex')
    expect(agentProviderLabel('claude-cli')).toBe('Claude Code')
  })

  it('returns unknown non-empty codes unchanged', () => {
    for (const provider of ['future-cli', 'constructor', '__proto__', 'Claude-CLI', ' codex-cli', 'claude-code', 'codex']) {
      expect(agentProviderLabel(provider)).toBe(provider)
    }
  })

  it('returns an empty string for missing providers', () => {
    expect(agentProviderLabel('')).toBe('')
    expect(agentProviderLabel(undefined)).toBe('')
    expect(agentProviderLabel(null)).toBe('')
    expect(agentProviderLabel()).toBe('')
  })
})
