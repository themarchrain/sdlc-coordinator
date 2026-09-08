import type { ThemeDefinition, ThemeSwatches } from '../types'
import { appleAntdTheme } from './antd'
import { appleSlots } from './slots'

/** apple 主题五色示意条（外观面板展示）。 */
export const appleSwatches: ThemeSwatches = {
  canvas: '#f5f5f7',
  surface: '#ffffff',
  ink: '#1d1d1f',
  primary: '#0066cc',
  accent: '#34c759',
}

/**
 * apple 主题 = Apple 设计语言（design/apple）。
 * CSS 令牌见 themes/css/apple.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor 取 apple 源 index.html 的 meta theme-color（珍珠灰）。
 */
export const appleTheme: ThemeDefinition = {
  id: 'apple',
  label: 'Apple',
  family: 'minimal',
  isDark: false,
  chromeColor: '#f5f5f7',
  swatches: appleSwatches,
  antd: appleAntdTheme,
  slots: appleSlots,
}
