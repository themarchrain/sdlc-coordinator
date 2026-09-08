import type { ThemeDefinition, ThemeSwatches } from '../types'
import { bmwMAntdTheme } from './antd'
import { bmwMSlots } from './slots'

/** bmw-m 主题五色示意条（外观面板展示）：纯黑画布 + 白主色 + M 红点缀。 */
export const bmwMSwatches: ThemeSwatches = {
  canvas: '#000000',
  surface: '#1a1a1a',
  ink: '#ffffff',
  primary: '#ffffff',
  accent: '#e22718',
}

/**
 * bmw-m 主题 = BMW M 设计语言（暗色）。
 * 纯黑画布 #000000 + 白墨 + M 三色条纹（#0066b1/#1c69d4/#e22718）作品牌记号；
 * 主操作 = 白（规格 primary #ffffff），矩形 0 圆角、无投影、发丝线分层。
 * CSS 令牌见 themes/css/bmw-m.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor 取页面画布纯黑（首屏防白闪映射同值）。
 */
export const bmwMTheme: ThemeDefinition = {
  id: 'bmw-m',
  label: 'BMW M',
  family: 'dark',
  isDark: true,
  chromeColor: '#000000',
  swatches: bmwMSwatches,
  antd: bmwMAntdTheme,
  slots: bmwMSlots,
}
