import { RobotOutlined } from '@ant-design/icons'

import { useTheme } from '../theme/useTheme'

export function AgentIcon({ provider }: { provider?: string | null }): JSX.Element | null {
  const { theme } = useTheme()
  if (!provider) return null
  if (provider === 'codex-cli') return <span className="agent-icon agent-icon--codex" aria-hidden="true" style={{ color: theme.isDark ? '#FFFFFF' : '#0D0D0D' }} />
  if (provider === 'claude-cli') return <span className="agent-icon agent-icon--claude" aria-hidden="true" style={{ color: theme.isDark ? '#E38963' : '#D97757' }} />
  return <RobotOutlined className="agent-icon agent-icon--fallback" aria-hidden="true" />
}
