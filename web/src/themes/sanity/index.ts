import type { ThemeDefinition, ThemeSwatches } from '../types'
import { sanityAntdTheme } from './antd'
import { sanitySlots } from './slots'

/** sanity 主题五色示意条（外观面板展示）：近黑画布 + 白字 + 白色药丸 CTA + coral 强调。 */
export const sanitySwatches: ThemeSwatches = {
  canvas: '#0b0b0b',
  surface: '#212121',
  ink: '#ffffff',
  primary: '#ffffff',
  accent: '#f36458',
}

/**
 * sanity 主题 = Sanity 设计语言（暗色）。
 * 暗色优先的编辑部气质：近黑画布 #0b0b0b + 卡阶 #212121（单步提升 + 1px 发丝边）、
 * 白色药丸 CTA 黑字（主动作即极性反转）、coral-red #f36458 每屏至多一次的稀缺强调、
 * IBM Plex Mono 眉题承担技术可信度、超大 display 常规字重 + 收紧负字距、
 * 营销药丸与应用件 3–6px 直角的混合圆角语言；深色卡坐平无影，深度靠极性切换。
 * 画布明暗勘误：站内目录对 Sanity 的亮色描述与文档实证相反——规格 canvas
 * #0b0b0b 为近黑且「dark-first」（亮色仅 pricing 分节反转），按文档定暗色。
 * CSS 令牌见 themes/css/sanity.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor 取 --canvas（#0b0b0b；首屏防白闪映射同值）。
 */
export const sanityTheme: ThemeDefinition = {
  id: 'sanity',
  label: 'Sanity',
  family: 'dark',
  isDark: true,
  chromeColor: '#0b0b0b',
  swatches: sanitySwatches,
  antd: sanityAntdTheme,
  slots: sanitySlots,
}
