export function agentProviderLabel(provider?: string | null): string {
  if (provider === 'codex-cli') return 'Codex'
  if (provider === 'claude-cli') return 'Claude Code'
  if (provider === 'pi-cli') return 'Pi'
  return provider ?? ''
}
