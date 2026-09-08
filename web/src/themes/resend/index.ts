import type { ThemeDefinition, ThemeSwatches } from '../types'
import { resendAntdTheme } from './antd'
import { resendSlots } from './slots'

/**
 * resend 主题五色示意条（外观面板展示）：真黑画布 + 近白墨 + 白主 CTA + 蓝强调。
 * primary 与 ink 同为 #fcfdff 是规格事实：白既是文字也是品牌强调
 * （规格 Don't「White is the brand on black — accents are atmospheric only」）。
 */
export const resendSwatches: ThemeSwatches = {
  canvas: '#000000',
  surface: '#0a0a0c',
  ink: '#fcfdff',
  primary: '#fcfdff',
  accent: '#3b9eff',
}

/**
 * resend 主题 = Resend 设计语言（暗色）。
 * 真黑画布 #000000 + 表面阶梯 #0a0a0c/#101012/#06060a（深度全靠半透白发丝线
 * 6%/14%/4%，零投影）；白 #fcfdff 是品牌强调（白底黑字 CTA = 画布上最亮像素），
 * 蓝 #3b9eff 链接/信息、绿 #11ff99 成功状态点；六 accent 只作低透明度大气辉光
 * 与行内高亮。排版 = 编辑部衬线 Domaine Display 显示字（骨架 --font-display，
 * 应用见 resend.css 片段）+ Inter/ABC Favorit UI + Geist Mono 代码；圆角词汇
 * 8/12/9999，正文 400、标题至多 500。CSS 令牌见 themes/css/resend.css；
 * antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = --canvas（#000000；首屏防白闪映射同值，规格「True black, never near-black」）。
 */
export const resendTheme: ThemeDefinition = {
  id: 'resend',
  label: 'Resend',
  family: 'dark',
  isDark: true,
  chromeColor: '#000000',
  swatches: resendSwatches,
  antd: resendAntdTheme,
  slots: resendSlots,
}
