import type { ThemeDefinition, ThemeSwatches } from '../types'
import { thevergeAntdTheme } from './antd'
import { thevergeSlots } from './slots'

/** theverge 主题五色示意条（外观面板展示）。 */
export const thevergeSwatches: ThemeSwatches = {
  canvas: '#131313',
  surface: '#2d2d2d',
  ink: '#ffffff',
  primary: '#3cffd0',
  accent: '#5200ff',
}

/**
 * theverge 主题 = The Verge 2024 设计语言。
 * 近黑 editorial 画布 + 双 hazard 强调（acid-mint / ultraviolet）+ 白发丝线零阴影海拔
 * + 药丸瓦片卡 + mono-uppercase kicker 签名；往管理台的迁移 = 双强调色体系、
 * 展示排版气势（Manuka 仅登录 showcase hero）、瓦片化卡片语言。
 * CSS 令牌见 themes/css/theverge.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = Canvas Black（「The default dark surface for the entire homepage」）。
 */
export const thevergeTheme: ThemeDefinition = {
  id: 'theverge',
  label: 'The Verge',
  family: 'dark',
  isDark: true,
  chromeColor: '#131313',
  swatches: thevergeSwatches,
  antd: thevergeAntdTheme,
  slots: thevergeSlots,
}
