import type { ThemeDefinition, ThemeSwatches } from '../types'
import { raycastAntdTheme } from './antd'
import { raycastSlots } from './slots'

/** raycast 主题五色示意条（外观面板展示）：近黑面阶 + 近白墨 + 白主色 + 签名红。 */
export const raycastSwatches: ThemeSwatches = {
  canvas: '#07080a',
  surface: '#0d0d0d',
  ink: '#f4f4f6',
  primary: '#ffffff',
  accent: '#ff5757',
}

/**
 * raycast 主题 = Raycast 设计语言（暗色）。
 * 营销页即产品截图：全站连续暗模式，面阶梯 #07080a → #0d0d0d → #101111 → 卡 #121212，
 * 发丝线 #242728，海拔全靠面色阶梯（零投影）；白色 CTA 锚定一切主操作（白底纯黑字）；
 * Inter ss03 特征集是排版签名；签名装饰 = 顶部红渐变字标带（#ff5757 → #a1131a），
 * 饱和点缀（HN 黄/Slack 红/Mac 绿/info 蓝）只落状态语义与产品插画类元素。
 * CSS 令牌见 themes/css/raycast.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor 取 --canvas（#07080a；首屏防白闪映射同值）。
 */
export const raycastTheme: ThemeDefinition = {
  id: 'raycast',
  label: 'Raycast',
  family: 'dark',
  isDark: true,
  chromeColor: '#07080a',
  swatches: raycastSwatches,
  antd: raycastAntdTheme,
  slots: raycastSlots,
}
