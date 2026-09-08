import type { ThemeDefinition, ThemeSwatches } from '../types'
import { airtableAntdTheme } from './antd'
import { airtableSlots } from './slots'

/**
 * airtable 主题五色示意条（外观面板展示）。
 * ink 与 primary 同值是规格原文：「Ink (#181d26): Same hex as {colors.primary}
 * because they are the same role expressed at type and button layers」
 * —— 墨既是标题色也是主按钮色；accent 取链接蓝 #1B61C9（最可辨识的辅助色）。
 */
export const airtableSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f8fafc',
  ink: '#181d26',
  primary: '#181d26',
  accent: '#1b61c9',
}

/**
 * airtable 主题 = Airtable 设计语言。
 * 编辑式白画布：白底 + 近黑墨 + 近黑主 CTA（黑即主色，蓝 #1B61C9 只做链接），
 * 无渐变/无氛围底纹，靠 hairline 发丝线、留白与签名卡色（coral/cream/peach 等）
 * 制造品牌电压；字重只有 400/500，圆角 6/10/12 三级克制。
 * 品牌时刻：brand-mark = 深墨底上的三根 pastel 竖条（articles 彩虹条纹 hero 的缩影），
 * 登录 showcase = surface-dark 深墨 hero 卡。CSS 令牌见 themes/css/airtable.css；
 * antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = #ffffff（规格「canvas 白是每一页的底」）。
 */
export const airtableTheme: ThemeDefinition = {
  id: 'airtable',
  label: 'Airtable',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: airtableSwatches,
  antd: airtableAntdTheme,
  slots: airtableSlots,
}
