import type { ThemeDefinition, ThemeSwatches } from '../types'
import { voltagentAntdTheme } from './antd'
import { voltagentSlots } from './slots'

/** voltagent 主题五色示意条（外观面板展示）：暗底 + 电光绿双阶。 */
export const voltagentSwatches: ThemeSwatches = {
  canvas: '#101010',
  surface: '#1a1a1a',
  ink: '#ffffff',
  primary: '#00d992',
  accent: '#10b981',
}

/**
 * voltagent 主题 = Voltagent 设计语言（design/voltagent，暗色）。
 * CSS 令牌见 themes/css/voltagent.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor 取源 index.html 的 meta theme-color（唯一暗面画布 #101010；
 * index.html 首屏防白闪映射已含同值）。
 */
export const voltagentTheme: ThemeDefinition = {
  id: 'voltagent',
  label: 'Voltagent',
  family: 'dark',
  isDark: true,
  chromeColor: '#101010',
  swatches: voltagentSwatches,
  antd: voltagentAntdTheme,
  slots: voltagentSlots,
}
