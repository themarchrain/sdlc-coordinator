import type { ThemeDefinition, ThemeSwatches } from '../types'
import { cursorAntdTheme } from './antd'
import { cursorSlots } from './slots'

/** cursor 主题五色示意条（外观面板展示）。 */
export const cursorSwatches: ThemeSwatches = {
  canvas: '#f7f7f4',
  surface: '#ffffff',
  ink: '#26251e',
  primary: '#f54e00',
  accent: '#c08532',
}

/**
 * cursor 主题 = Cursor 设计语言（design/cursor）。
 * CSS 令牌见 themes/css/cursor.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor 取 cursor 源 index.html 的 meta theme-color（奶油画布）。
 */
export const cursorTheme: ThemeDefinition = {
  id: 'cursor',
  label: 'Cursor',
  family: 'warm',
  isDark: false,
  chromeColor: '#f7f7f4',
  swatches: cursorSwatches,
  antd: cursorAntdTheme,
  slots: cursorSlots,
}
