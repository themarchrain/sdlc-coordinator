import type { ThemeDefinition, ThemeSwatches } from '../types'
import { warpAntdTheme } from './antd'
import { warpSlots } from './slots'

/** warp 主题五色示意条（外观面板展示）：暖棕暗画布 + 暖白一族 + 暖银阶。 */
export const warpSwatches: ThemeSwatches = {
  canvas: '#2b2622',
  surface: '#383330',
  ink: '#f7f5f0',
  primary: '#f7f5f0',
  accent: '#aea69c',
}

/**
 * warp 主题 = Warp 设计语言（暗色）。
 * 暖近炭画布（#2b2622，比纯黑更棕的 Rust 终端血统）+ 唯一的「颜色」是暖白
 * #f7f5f0（同时充当默认文字与主 CTA 填充，无彩色品牌强调）；卡 = canvas-soft
 * #383330 + 发丝线 #3f3a36，海拔靠面阶对比不发投影；极紧圆角（按钮 3px / 卡 4px）；
 * 字重只有 400/500，Inter 叙事 + DM Mono 代码 + Instrument Serif 罕见斜体编辑瞬间。
 * CSS 令牌见 themes/css/warp.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = --canvas（#2b2622；暗色首屏防白闪映射同值）。
 */
export const warpTheme: ThemeDefinition = {
  id: 'warp',
  label: 'Warp',
  family: 'dark',
  isDark: true,
  chromeColor: '#2b2622',
  swatches: warpSwatches,
  antd: warpAntdTheme,
  slots: warpSlots,
}
