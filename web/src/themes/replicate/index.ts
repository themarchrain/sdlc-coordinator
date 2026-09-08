import type { ThemeDefinition, ThemeSwatches } from '../types'
import { replicateAntdTheme } from './antd'
import { replicateSlots } from './slots'

/** replicate 主题五色示意条（外观面板展示）。accent = hero-glow（规格 hero mesh 的浅橙签名）。 */
export const replicateSwatches: ThemeSwatches = {
  canvas: '#f9f7f3',
  surface: '#ffffff',
  ink: '#202020',
  primary: '#ea2804',
  accent: '#ff6a3d',
}

/**
 * replicate 主题 = Replicate 设计语言。
 * 「AI 实验室笔记 × 印刷杂志」：暖白 cream 画布 #F9F7F3（页面级永不纯白），
 * 白只上卡/输入/浮层；热橙 #EA2804 是稀缺戳记（主 CTA / hero / 链接三职）；
 * 暗代码井 #202020 如印刷拉页嵌在 cream 上；三族字各守其位 ——
 * rb-freigeist-neue 展示巨标（替身 Bricolage Grotesque）、basier-square UI/正文
 * （替身 Inter）、JetBrains Mono 代码；每个交互面全药丸（rounded.full），
 * 内容卡停在 10/16；海拔靠色彩分层（cream→bone→dark 反转）不靠影。
 * CSS 令牌见 themes/css/replicate.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = Canvas cream（规格「the default page background」）。
 */
export const replicateTheme: ThemeDefinition = {
  id: 'replicate',
  label: 'Replicate',
  family: 'minimal',
  isDark: false,
  chromeColor: '#f9f7f3',
  swatches: replicateSwatches,
  antd: replicateAntdTheme,
  slots: replicateSlots,
}
