import type { ThemeDefinition, ThemeSwatches } from '../types'
import { discordAntdTheme } from './antd'
import { discordSlots } from './slots'

/** discord 主题五色示意条（外观面板展示）：深靛蓝画布 + 白墨 + Blurple/Magenta 双强调。 */
export const discordSwatches: ThemeSwatches = {
  canvas: '#0a0d3a',
  surface: '#1e2353',
  ink: '#ffffff',
  primary: '#5865f2',
  accent: '#ec48bd',
}

/**
 * discord 主题 = Discord 设计语言（暗色）。
 * 深靛蓝画布 #0a0d3a + Blurple #5865f2 主操作 + green #35ed7e 高意向 CTA +
 * magenta #ec48bd 渐变强调（品牌渐变见 css/discord.css 作用域片段）；
 * 重全大写展示字（字重 800）+ 圆润药丸形态（pill 50px / 面板 40px）。
 * CSS 令牌见 themes/css/discord.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor 取页面画布深靛蓝（首屏防白闪映射同值）。
 */
export const discordTheme: ThemeDefinition = {
  id: 'discord',
  label: 'Discord',
  family: 'dark',
  isDark: true,
  chromeColor: '#0a0d3a',
  swatches: discordSwatches,
  antd: discordAntdTheme,
  slots: discordSlots,
}
