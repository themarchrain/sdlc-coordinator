import type { ThemeDefinition, ThemeSwatches } from '../types'
import { xaiAntdTheme } from './antd'
import { xaiSlots } from './slots'

/** xai 主题五色示意条（外观面板展示）：暗底 + 白药丸 + sunset 点缀。 */
export const xaiSwatches: ThemeSwatches = {
  canvas: '#0a0a0a',
  surface: '#191919',
  ink: '#ffffff',
  primary: '#ffffff',
  accent: '#ff7a17',
}

/**
 * xai 主题 = xAI 设计语言（design/xai，暗色）。
 * CSS 令牌见 themes/css/xai.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor 取源 :root --canvas（近黑 #0a0a0a；index.html 首屏防白闪映射同值）。
 */
export const xaiTheme: ThemeDefinition = {
  id: 'xai',
  label: 'xAI',
  family: 'dark',
  isDark: true,
  chromeColor: '#0a0a0a',
  swatches: xaiSwatches,
  antd: xaiAntdTheme,
  slots: xaiSlots,
}
