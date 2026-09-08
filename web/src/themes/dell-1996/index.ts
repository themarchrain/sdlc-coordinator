import type { ThemeDefinition, ThemeSwatches } from '../types'
import { dell1996ThemeConfig } from './antd'
import { dell1996Slots } from './slots'

/** dell-1996 主题五色示意条（外观面板展示）：帧内纯白 + 帧墨 + Dell red。 */
export const dell1996Swatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#ffffff',
  ink: '#000000',
  primary: '#000000',
  accent: '#e91d2a',
}

/**
 * dell-1996 主题 = Dell.com 1996 目录式设计语言。
 * 亮色主题：帧内纯白画布、1px 纯黑发丝线承全部层级、缎带卡嵌面色族、
 * 双字体层级（Helvetica UI / Times 正文 / Arial Black 展示数字）、全场方角。
 * 主操作色 = 帧墨（规格 button-primary 黑底白字）；Dell red 保留给
 * 危险/报错与导航指示（规格 Do：红仅最高注意级）。
 * chromeColor = 帧墨 #000000（黑色 page-frame / top-banner 即浏览器 chrome 色；
 * default 主题同为亮主题配深 chromeColor 之先例）。
 * CSS 令牌见 themes/css/dell-1996.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 */
export const dell1996Theme: ThemeDefinition = {
  id: 'dell-1996',
  label: 'Dell 1996',
  family: 'retro',
  isDark: false,
  chromeColor: '#000000',
  swatches: dell1996Swatches,
  antd: dell1996ThemeConfig,
  slots: dell1996Slots,
}
