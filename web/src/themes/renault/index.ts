import type { ThemeDefinition, ThemeSwatches } from '../types'
import { renaultAntdTheme } from './antd'
import { renaultSlots } from './slots'

/** renault 主题五色示意条（外观面板展示）：白画布 + 黑白 stark + Sunlight Yellow。 */
export const renaultSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f7f7f7',
  ink: '#000000',
  primary: '#ffed00',
  accent: '#111111', // surface-deep：黑白黄三色系统里的黑色叙事面（第二结构色）
}

/**
 * renault 主题 = Renault 设计语言。
 * 黑白 stark 对比：白画布目录模式（默认亮轨）+ 黑色叙事面（json/tooltip/登录
 * showcase 局部取材 surface-dark #000000）+ 唯一品牌强调 Sunlight Yellow #FFED00
 * （主 CTA / yeni 徽标 / 导航指示点，黄底永远配黑字）。全站 NouvelR 展示字
 * （授权族 → 规格替代栈 Inter Tight/Manrope），display 700 @ 0.95 紧行高、
 * 正文 400、无斜体；方角几何（按钮 rounded.xs 2px、卡面 rounded.none）；
 * 海拔 = 全幅色块切换（白→黑→黄）而非阴影，仅 sticky 浮层留 2-4px/10% 微影。
 * brand-mark = 2021 菱形标表达：白底墨描边方块旋转 45° + 黄色 yeni 角点。
 * CSS 令牌见 themes/css/renault.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = #FFFFFF（规格「white catalogue mode」画布）。
 */
export const renaultTheme: ThemeDefinition = {
  id: 'renault',
  label: 'Renault',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: renaultSwatches,
  antd: renaultAntdTheme,
  slots: renaultSlots,
}
