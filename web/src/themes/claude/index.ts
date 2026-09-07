import type { ThemeDefinition, ThemeSwatches } from '../types'
import { claudeAntdTheme } from './antd'
import { claudeSlots } from './slots'

/** claude 主题五色示意条（外观面板展示）。 */
export const claudeSwatches: ThemeSwatches = {
  canvas: '#faf9f5',
  surface: '#f5f0e8',
  ink: '#141413',
  primary: '#cc785c',
  accent: '#5db8a6',
}

/**
 * claude 主题 = Claude 设计语言（暖调编辑风）。
 * CSS 令牌见 themes/css/claude.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor 取 claude 源 index.html 的 meta[name=theme-color]。
 */
export const claudeTheme: ThemeDefinition = {
  id: 'claude',
  label: 'Claude',
  isDark: false,
  chromeColor: '#faf9f5',
  swatches: claudeSwatches,
  antd: claudeAntdTheme,
  slots: claudeSlots,
}
