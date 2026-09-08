import type { ThemeDefinition, ThemeSwatches } from '../types'
import { runwayAntdTheme } from './antd'
import { runwaySlots } from './slots'

/** runway 主题五色示意条（外观面板展示）。 */
export const runwaySwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#e7eaf0',
  ink: '#030303',
  primary: '#000000',
  accent: '#676f7b',
}

/**
 * runway 主题 = Runway 设计语言。
 * 电影节场刊式编辑极简：纸白画布 + 近黑墨，黑色药丸是唯一主行动语言，
 * 五级灰阶（ink/graphite/slate/stone/hairline）承担全部层级，无第二交互色；
 * 发丝线分隔、零阴影零光晕、方角格子 + 药丸按钮的严格圆角二分、
 * 标题一律 400 常规字重 + 负字距（abcNormal → Inter 替代）。
 * CSS 令牌见 themes/css/runway.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = canvas #ffffff（亮色画布勘误：规格 画布为 #ffffff 纸白）。
 */
export const runwayTheme: ThemeDefinition = {
  id: 'runway',
  label: 'Runway',
  family: 'minimal',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: runwaySwatches,
  antd: runwayAntdTheme,
  slots: runwaySlots,
}
