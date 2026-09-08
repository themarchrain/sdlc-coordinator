import type { ThemeDefinition, ThemeSwatches } from '../types'
import { teslaAntdTheme } from './antd'
import { teslaSlots } from './slots'

/** tesla 主题五色示意条（外观面板展示）。 */
export const teslaSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f4f4f4',
  ink: '#171a20',
  primary: '#3e6ae1',
  accent: '#5c5e62',
}

/**
 * tesla 主题 = Tesla 设计语言。
 * 激进减法：白画布 + 近黑墨 + 唯一交互色 Electric Blue；近零装饰
 * （发丝线、零阴影、4px 微圆角、400/500 字重、0.33s 统一节奏）。
 * CSS 令牌见 themes/css/tesla.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = Pure White（规格「dominant background color」）。
 */
export const teslaTheme: ThemeDefinition = {
  id: 'tesla',
  label: 'Tesla',
  family: 'minimal',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: teslaSwatches,
  antd: teslaAntdTheme,
  slots: teslaSlots,
}
