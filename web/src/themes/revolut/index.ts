import type { ThemeDefinition, ThemeSwatches } from '../types'
import { revolutAntdTheme } from './antd'
import { revolutSlots } from './slots'

/** revolut 主题五色示意条（外观面板展示）：真黑画布 + 白墨 + cobalt + teal。 */
export const revolutSwatches: ThemeSwatches = {
  canvas: '#000000',
  surface: '#16181a',
  ink: '#ffffff',
  primary: '#494fdf',
  accent: '#00a87e',
}

/**
 * revolut 主题 = Revolut 设计语言（暗色）。
 * 真黑叙事画布 #000000（规格 Don't「never near-black」）+ 两级暗面阶梯
 * （内嵌面 #0a0a0a / 提亮卡 #16181a，无投影，海拔全靠明度切换）+
 * cobalt-violet #494fdf 作「deliberate stamp」（主操作/特色面/品牌图形）+
 * 宽产品色盘只进图表与状态。Aeonik Pro 500 展示字 + Inter 400/600 正文、
 * 全按钮药丸化、卡 20px、控件 12px；白药丸（黑画布上最响的 CTA）保留给登录页。
 * CSS 令牌见 themes/css/revolut.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = --canvas（#000000 真黑；首屏防白闪映射同值）。
 */
export const revolutTheme: ThemeDefinition = {
  id: 'revolut',
  label: 'Revolut',
  family: 'dark',
  isDark: true,
  chromeColor: '#000000',
  swatches: revolutSwatches,
  antd: revolutAntdTheme,
  slots: revolutSlots,
}
