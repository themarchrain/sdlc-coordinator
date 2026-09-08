import type { ThemeDefinition, ThemeSwatches } from '../types'
import { spacexAntdTheme } from './antd'
import { spacexSlots } from './slots'

/** spacex 主题五色示意条（外观面板展示）：纯黑画布 + 白墨 + 白信号 + 冷灰发丝线。 */
export const spacexSwatches: ThemeSwatches = {
  canvas: '#000000',
  surface: '#0a0a0a',
  ink: '#ffffff',
  primary: '#ffffff',
  accent: '#3a3a3f',
}

/**
 * spacex 主题 = SpaceX 设计语言（暗色）。
 * 勘误定调：文档实证营销站为 pure black canvas（canvas-night #000000）→ 暗色主题。
 * 极简 austere 黑白：纯黑画布 + #0a0a0a 微升面板（1px #3a3a3f 发丝线）、
 * 大写 D-DIN 展示字（tight leading + 正字距宽 tracking）、ghost 描边药丸 CTA、
 * 全屏摄影 hero 是唯一装饰（管理台无法承载摄影 → 按角色推导为
 * 黑白反转 + 黑→#0a0a0a 提升阶梯的暗面 chrome，零阴影零渐变）。
 * 白是唯一交互信号（主 CTA 白底黑字、白链、白 focus）。
 * CSS 令牌见 themes/css/spacex.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = --canvas（#000000；首屏防白闪映射同值）。
 */
export const spacexTheme: ThemeDefinition = {
  id: 'spacex',
  label: 'SpaceX',
  family: 'dark',
  isDark: true,
  chromeColor: '#000000',
  swatches: spacexSwatches,
  antd: spacexAntdTheme,
  slots: spacexSlots,
}
