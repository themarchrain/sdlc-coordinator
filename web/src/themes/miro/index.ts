import type { ThemeDefinition, ThemeSwatches } from '../types'
import { miroAntdTheme } from './antd'
import { miroSlots } from './slots'

/** miro 主题五色示意条（外观面板展示）。primary 与 ink 同为近黑 —— Miro 的主 CTA 就是墨色药丸。 */
export const miroSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f7f8fa',
  ink: '#1c1c1e',
  primary: '#1c1c1e',
  accent: '#ffd02f',
}

/**
 * miro 主题 = Miro 设计语言。
 * 白亮轨 + Miro 黄品牌：纯白画布上，黑药丸主 CTA（#1C1C1E + rounded.full）
 * 是主导交互元素；签名 Miro 黄 #FFD02F 只落在 wordmark 级装饰（brand-mark
 * 便利贴、nav 指示、黄标签 chip），从不上 CTA 与大面；行内链接/聚焦 =
 * 动作蓝 #4262FF。粉彩便利贴色板（coral/teal/rose/yellow）只作 brand-mark
 * 三色贴与点缀。Roobert PRO 字重只有 400/500/600，负字距标题；海拔用
 * 白板 mockup 阴影族 rgba(5,0,56,x)，平面卡不投影。
 * CSS 令牌见 themes/css/miro.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = Canvas White（规格「Page background」）。
 */
export const miroTheme: ThemeDefinition = {
  id: 'miro',
  label: 'Miro',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: miroSwatches,
  antd: miroAntdTheme,
  slots: miroSlots,
}
