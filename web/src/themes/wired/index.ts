import type { ThemeDefinition, ThemeSwatches } from '../types'
import { wiredAntdTheme } from './antd'
import { wiredSlots } from './slots'

/** wired 主题五色示意条（外观面板展示）。 */
export const wiredSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f5f5f5',
  ink: '#000000',
  primary: '#000000',
  accent: '#057dbc',
}

/**
 * wired 主题 = WIRED 设计语言。
 * 编辑部黑白二重奏：白画布 + 纯黑 wordmark/CTA，唯一彩色是内文链接蓝 #057DBC；
 * 三族排版（WiredDisplay 展示衬线 / BreveText 正文衬线 / Apercu 结构 sans，
 * 「serif for narrative, sans for structure」）+ mono kicker 大写宽字距（杂志技术签）。
 * 方角几何（全站 0px，「square corners are non-negotiable」）、零软阴影
 * （海拔 = hairline #E0E0E0 与表面对比）、黑 footer 带的极性翻转。
 * CSS 令牌见 themes/css/wired.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = 画布白（规格「canvas #ffffff」）。
 */
export const wiredTheme: ThemeDefinition = {
  id: 'wired',
  label: 'WIRED',
  family: 'warm',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: wiredSwatches,
  antd: wiredAntdTheme,
  slots: wiredSlots,
}
