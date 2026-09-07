import type { ThemeDefinition, ThemeSwatches } from '../types'
import { vercelAntdTheme } from './antd'
import { vercelSlots } from './slots'

/** vercel 主题五色示意条（外观面板展示）。 */
export const vercelSwatches: ThemeSwatches = {
  canvas: '#fafafa',
  surface: '#ffffff',
  ink: '#171717',
  primary: '#171717',
  accent: '#0070f3',
}

/**
 * vercel 主题 = Geist (Vercel) 设计语言（design/vercel）。
 * CSS 令牌见 themes/css/vercel.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor 取 vercel 源 index.html 的 meta theme-color（ink 黑）。
 */
export const vercelTheme: ThemeDefinition = {
  id: 'vercel',
  label: 'Vercel',
  isDark: false,
  chromeColor: '#171717',
  swatches: vercelSwatches,
  antd: vercelAntdTheme,
  slots: vercelSlots,
}
