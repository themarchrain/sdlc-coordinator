import type { ThemeDefinition, ThemeSwatches } from '../types'
import { lamborghiniAntdTheme } from './antd'
import { lamborghiniSlots } from './slots'

/** lamborghini 主题五色示意条（外观面板展示）：真黑茧房 + 白字 + 金/青强调。 */
export const lamborghiniSwatches: ThemeSwatches = {
  canvas: '#000000',
  surface: '#202020',
  ink: '#ffffff',
  primary: '#ffc000',
  accent: '#29abe2',
}

/**
 * lamborghini 主题 = Lamborghini 设计语言（暗色）。
 * 真黑画布 #000000（规格「true, uncompromising black」）+ Charcoal #202020 提升面
 * + Dark Iron #181818 内嵌面（海拔 = 表面变亮，零阴影）+ 白字 + Giallo 金 #FFC000
 * 唯一品牌强调（仅主 CTA，金底黑字，hover 转 Dark Gold #917300）+ Cyan Pulse
 * #29ABE2 信息高亮。签名形态：零圆角、全大写显示排版、字重 400 主导、
 * ghost 按钮白边 50% hover 转 Teal #1EAEDB、12° 斜切终端几何。
 * CSS 令牌见 themes/css/lamborghini.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = --canvas（#000000；首屏防白闪映射同值，规格「Absolute Black — dominant surface color」）。
 */
export const lamborghiniTheme: ThemeDefinition = {
  id: 'lamborghini',
  label: 'Lamborghini',
  family: 'dark',
  isDark: true,
  chromeColor: '#000000',
  swatches: lamborghiniSwatches,
  antd: lamborghiniAntdTheme,
  slots: lamborghiniSlots,
}
