import type { ThemeDefinition, ThemeSwatches } from '../types'
import { minimaxAntdTheme } from './antd'
import { minimaxSlots } from './slots'

/** MiniMax 主题五色示意条（外观面板展示）。 */
export const minimaxSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f7f8fa',
  ink: '#222222',
  primary: '#0a0a0a',
  accent: '#ff5530',
}

/**
 * MiniMax 主题 = 白画布 + 近黑主色药丸 + 灰阶文字链 + 品牌渐变身份面。
 * CSS 令牌见 themes/css/minimax.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 */
export const minimaxTheme: ThemeDefinition = {
  id: 'minimax',
  label: 'MiniMax',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: minimaxSwatches,
  antd: minimaxAntdTheme,
  slots: minimaxSlots,
}
