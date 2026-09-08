import type { ThemeDefinition, ThemeSwatches } from '../types'
import { bmwAntdTheme } from './antd'
import { bmwSlots } from './slots'

/** bmw 主题五色示意条（外观面板展示）：白画布 + BMW blue + 暗军蓝 hero 面。 */
export const bmwSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#fafafa',
  ink: '#262626',
  primary: '#1c69d4',
  accent: '#1a2129',
}

/**
 * bmw 主题 = BMW corporate 设计语言（
 * 亮轨 #ffffff 画布 —— 与已内置的 bmw-m 暗色主题是同一品牌的两个方言）。
 * 克制的 corporate-automotive：白画布 + 近黑墨 + BMW corporate blue #1c69d4
 * 唯一主操作色；暗军蓝 #1a2129 只作 hero 带（登录 showcase / json 深面 / tooltip）；
 * 矩形 0px 圆角、全站无投影（深度 = 色块对比）、BMW Type Next 700/300 字重对比
 * （Inter 替身）、大写机加工「LEARN MORE」标签；M 三色条纹不进本主题。
 * CSS 令牌见 themes/css/bmw.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = 规格画布 #ffffff（规格「dominant background color」）。
 */
export const bmwTheme: ThemeDefinition = {
  id: 'bmw',
  label: 'BMW',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: bmwSwatches,
  antd: bmwAntdTheme,
  slots: bmwSlots,
}
