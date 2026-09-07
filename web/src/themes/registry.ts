import type { ThemeDefinition } from './types'

export type { ThemeDefinition } from './types'

import { defaultTheme } from './default'
import { appleTheme } from './apple'
import { claudeTheme } from './claude'
import { minimaxTheme } from './minimax'
import { supabaseTheme } from './supabase'
import { vercelTheme } from './vercel'
import { xaiTheme } from './xai'
import { cursorTheme } from './cursor'
import { ollamaTheme } from './ollama'
import { voltagentTheme } from './voltagent'

/** localStorage 持久化 key（与 web/index.html 首屏脚本、ThemeProvider 同 key）。 */
export const THEME_STORAGE_KEY = 'coordinator-theme'

/** 回退主题 id。 */
export const DEFAULT_THEME_ID = 'default'

/** 有序主题清单：外观面板按此顺序渲染主题卡。 */
export const THEME_LIST: ThemeDefinition[] = [
  defaultTheme,
  appleTheme,
  claudeTheme,
  minimaxTheme,
  supabaseTheme,
  vercelTheme,
  xaiTheme,
  cursorTheme,
  ollamaTheme,
  voltagentTheme,
]

/** id → ThemeDefinition 索引。 */
export const THEMES: Record<string, ThemeDefinition> = Object.fromEntries(THEME_LIST.map((theme) => [theme.id, theme]))

/** 取主题；非法/缺失 id 一律回退 default（与 index.html 首屏脚本同一逻辑）。 */
export function getTheme(id: string | null | undefined): ThemeDefinition {
  return (id && THEMES[id]) || THEMES[DEFAULT_THEME_ID]
}
