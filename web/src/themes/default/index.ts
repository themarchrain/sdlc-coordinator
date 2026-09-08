import type { ThemeDefinition, ThemeSwatches } from '../types'
import { defaultAntdTheme } from './antd'
import { defaultSlots } from './slots'

/** default 主题五色示意条（外观面板展示）。 */
export const defaultSwatches: ThemeSwatches = {
  canvas: '#f5f5f5',
  surface: '#ffffff',
  ink: '#152033',
  primary: '#2f6bff',
  accent: '#0f9f8f',
}

/**
 * default 主题 = develop 现行视觉，像素级不变。
 * CSS 令牌见 themes/css/default.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 */
export const defaultTheme: ThemeDefinition = {
  id: 'default',
  label: '默认',
  family: 'system',
  isDark: false,
  chromeColor: '#152033',
  swatches: defaultSwatches,
  antd: defaultAntdTheme,
  slots: defaultSlots,
}
