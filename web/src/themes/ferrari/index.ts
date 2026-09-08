import type { ThemeDefinition, ThemeSwatches } from '../types'
import { ferrariAntdTheme } from './antd'
import { ferrariSlots } from './slots'

/** ferrari 主题五色示意条（外观面板展示）：近黑微暖画布 + 纯白 + Rosso + 焦点黄。 */
export const ferrariSwatches: ThemeSwatches = {
  canvas: '#181818',
  surface: '#303030',
  ink: '#ffffff',
  primary: '#da291c',
  accent: '#f6e500',
}

/**
 * ferrari 主题 = Ferrari 设计语言（暗色）。
 * cinematic editorial：近黑微暖画布 #181818（永不纯黑）+ 纯白 display（恒 500
 * 不加粗、负字距）+ 唯一品牌电压 Rosso Corsa #da291c（只落主 CTA / brand-mark
 * Cavallino / 导航指示条 race-position highlight）+ 锐角哲学（CTA/卡/面板全 0px，
 * 药丸只归徽章）+ 大写宽字距 CTA/导航 + 发丝线与亮度阶梯承担海拔（单一 soft 阴影层）。
 * 焦点环 = Hypersail Yellow（规格全局 focus-ring 色）。
 * CSS 令牌见 themes/css/ferrari.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = --canvas（#181818 近黑微暖；首屏防白闪映射同值）。
 */
export const ferrariTheme: ThemeDefinition = {
  id: 'ferrari',
  label: 'Ferrari',
  family: 'dark',
  isDark: true,
  chromeColor: '#181818',
  swatches: ferrariSwatches,
  antd: ferrariAntdTheme,
  slots: ferrariSlots,
}
