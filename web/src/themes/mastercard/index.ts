import type { ThemeDefinition, ThemeSwatches } from '../types'
import { mastercardAntdTheme } from './antd'
import { mastercardSlots } from './slots'

/** mastercard 主题五色示意条（外观面板展示）。 */
export const mastercardSwatches: ThemeSwatches = {
  canvas: '#f3f0ee',
  surface: '#fcfbfa',
  ink: '#141413',
  primary: '#141413',
  accent: '#cf4500',
}

/**
 * mastercard 主题 = Mastercard 设计语言
 * 。
 * 暖石报纸编辑风：putty-cream 画布 #F3F0EE + 三层暖面（canvas → lifted cream
 * → white 浮层）+ 暖黑 Ink #141413 兼任主 CTA 与暗面（CTA 文字用奶油非纯白）；
 * 超圆角母题（按钮 20 / hero 40 / 卡全药丸 999+，8–12 中段刻意缺席）；
 * 圆形轨道 + 信号橙 #F37338 弧线装饰、眉题小橙点、MarkForMC 500/-2% 标题
 * 与 450 正文半档字重；阴影 = 大弥散低透明度「呼吸垫」。
 * 品牌红 #EB001B / 黄 #F79E1B 只出现在 brand-mark logo 位，禁作 UI 色。
 * CSS 令牌见 themes/css/mastercard.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = Canvas Cream（「Background: Canvas Cream (#F3F0EE)」）。
 */
export const mastercardTheme: ThemeDefinition = {
  id: 'mastercard',
  label: 'Mastercard',
  family: 'warm',
  isDark: false,
  chromeColor: '#f3f0ee',
  swatches: mastercardSwatches,
  antd: mastercardAntdTheme,
  slots: mastercardSlots,
}
