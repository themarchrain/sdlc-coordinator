import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'

import { getTheme, THEMES, THEME_STORAGE_KEY, type ThemeDefinition } from '../themes/registry'

interface ThemeContextValue {
  themeId: string
  theme: ThemeDefinition
  setTheme: (id: string) => void
}

const ThemeContext = createContext<ThemeContextValue | undefined>(undefined)

/** 读 localStorage 中的主题 id；非法/缺失值回退 default（与 index.html 首屏脚本同逻辑）。 */
function readStoredThemeId(): string {
  try {
    return getTheme(localStorage.getItem(THEME_STORAGE_KEY)).id
  } catch {
    return THEMES.default.id
  }
}

/** 把主题应用到 DOM：data-theme 属性 + meta theme-color 浏览器 chrome 预设色。 */
function applyThemeToDocument(theme: ThemeDefinition) {
  document.documentElement.dataset.theme = theme.id
  const meta = document.querySelector('meta[name="theme-color"]')
  if (meta) meta.setAttribute('content', theme.chromeColor)
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [themeId, setThemeId] = useState(readStoredThemeId)
  const theme = getTheme(themeId)

  // 首屏内联脚本已设置过 data-theme；这里在 React 侧兜底同步（测试环境无该脚本）。
  useEffect(() => {
    applyThemeToDocument(theme)
  }, [theme])

  const setTheme = useCallback((id: string) => {
    const next = getTheme(id)
    try {
      localStorage.setItem(THEME_STORAGE_KEY, next.id)
    } catch {
      // localStorage 不可用（隐私模式等）时仅影响持久化，不影响本次会话切换。
    }
    applyThemeToDocument(next)
    setThemeId(next.id)
  }, [])

  const value = useMemo(() => ({ themeId: theme.id, theme, setTheme }), [theme, setTheme])

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export function useThemeContext() {
  return useContext(ThemeContext)
}
