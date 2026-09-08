import type { ThemeDefinition, ThemeSwatches } from '../types'
import { ibmAntdTheme } from './antd'
import { ibmSlots } from './slots'

/** ibm 主题五色示意条（外观面板展示）。accent = ink-muted：IBM 无第二彩色，示条以灰阶收尾。 */
export const ibmSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f4f4f4',
  ink: '#161616',
  primary: '#0f62fe',
  accent: '#525252',
}

/**
 * ibm 主题 = IBM / Carbon Design System。
 * 云时代的旧世界企业庄重感：白画布 + 炭墨 + 唯一强调色 IBM Blue #0F62FE；
 * 方正几何（圆角 0、无药丸）、1px 发丝线与灰面切换分层（零阴影）、
 * IBM Plex 字族三角色分工（Sans 正文 / Mono 代码 = 产品面 / Serif 数值展示位）、
 * 展示字号 300 轻 字重签名、body 0.16px 正字距精密细节、
 * Carbon productive 动效节奏（70/150/240ms）。
 * CSS 令牌见 themes/css/ibm.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = #ffffff（规格「dominant surface is canvas pure white」）。
 */
export const ibmTheme: ThemeDefinition = {
  id: 'ibm',
  label: 'IBM',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: ibmSwatches,
  antd: ibmAntdTheme,
  slots: ibmSlots,
}
