import type { ThemeDefinition, ThemeSwatches } from '../types'
import { nvidiaAntdTheme } from './antd'
import { nvidiaSlots } from './slots'

/** nvidia 主题五色示意条（外观面板展示）。 */
export const nvidiaSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f7f7f7',
  ink: '#000000',
  primary: '#76b900',
  accent: '#0046a4',
}

/**
 * nvidia 主题 = NVIDIA 设计语言。
 * 双面轨道：黑 canvas 章（hero/footer/主导航 surface-dark）+ 纸白正文轨，
 * 唯一强调色 NVIDIA Green #76B900 独占 CTA/激活态/12px 角标方块；
 * 绿底黑字签名反转、全交互件 2px 锐角、字重两极 400/700、卡片零投影
 * 靠 hairline #CCCCCC 分隔；link-blue #0046A4 只做内联链接/数据墨水。
 * 规格「有暗面变体默认亮」——本主题取亮色正文轨为默认，黑面由 CSS
 * 令牌（canvas-dark/sidebar/json/登录章）承担。
 * CSS 令牌见 themes/css/nvidia.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = Canvas White（正文轨主导底，与 isDark=false 首屏一致）。
 */
export const nvidiaTheme: ThemeDefinition = {
  id: 'nvidia',
  label: 'NVIDIA',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: nvidiaSwatches,
  antd: nvidiaAntdTheme,
  slots: nvidiaSlots,
}
