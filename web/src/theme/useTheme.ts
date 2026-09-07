import { useThemeContext } from './ThemeProvider'
import { THEMES } from '../themes/registry'
import type { ThemeDefinition } from '../themes/types'

/**
 * 取当前主题。无 Provider 时（存量测试直接渲染页面组件）回退 default 主题，
 * 保证页面组件在任意渲染环境下都能取到完整 slots。
 */
export function useTheme(): { themeId: string; theme: ThemeDefinition; setTheme: (id: string) => void } {
  const context = useThemeContext()
  if (context) return context
  return { themeId: THEMES.default.id, theme: THEMES.default, setTheme: () => undefined }
}
