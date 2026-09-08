import type { ThemeDefinition, ThemeSwatches } from '../types'
import { mintlifyAntdTheme } from './antd'
import { mintlifySlots } from './slots'

/** mintlify 主题五色示意条（外观面板展示）。 */
export const mintlifySwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f7f7f7',
  ink: '#0a0a0a',
  primary: '#0a0a0a',
  accent: '#00d4a4',
}

/**
 * mintlify 主题 = Mintlify 设计语言。
 * 双模态美学：营销面的大气渐变 hero（天空-奶油 / 深青-薄荷）+ 开发者级密集文档面，
 * 本主题取亮轨（Canvas White #FFFFFF）。「黑药丸 CTA + 近黑墨」承担全部交互，
 * Mint 绿 #00D4A4 只落在强调 CTA、激活指示与 focus 信号；Inter 散文 + Geist Mono
 * 代码二部制；按钮徽章一律全药丸、卡片一律 12px；平面为主、发丝线分隔，
 * 海拔只给浮层（Level 2/3 投影）。渐变装饰只进展示位：登录 showcase =
 * startups hero 深青→薄荷渐变 + 薄荷辉光，眉题 kicker = 薄荷→引用蓝渐变字。
 * CSS 令牌见 themes/css/mintlify.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = Canvas White（规格「primary page and card background」）。
 */
export const mintlifyTheme: ThemeDefinition = {
  id: 'mintlify',
  label: 'Mintlify',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: mintlifySwatches,
  antd: mintlifyAntdTheme,
  slots: mintlifySlots,
}
