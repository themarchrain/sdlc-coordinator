import type { ThemeDefinition, ThemeSwatches } from '../types'
import { uberAntdTheme } from './antd'
import { uberSlots } from './slots'

/** uber 主题五色示意条（外观面板展示）：白画布 + soft 灰面 + 纯黑主操作 + 一点链接蓝。 */
export const uberSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#efefef',
  ink: '#000000',
  primary: '#000000',
  accent: '#0000ee',
}

/**
 * uber 主题 = Uber 设计语言。
 * 黑白二重唱：白画布 #ffffff + canvas-soft #efefef 灰面 + 纯黑 #000000 作唯一
 * 转化色（黑药丸 CTA / 黑 footer / 黑 promo 带 = 极性翻转深度）；药丸 999px 是
 * 唯一交互几何签名，卡片 16px、输入 8px 灰填充；#0000ee 浏览器默认链接蓝是
 * 系统唯一彩；标题 UberMove 700 句首大写、按钮 500（Inter 替身）；
 * 无渐变无装饰，海拔只靠规格两档投影与极性翻转。
 * CSS 令牌见 themes/css/uber.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = 规格画布 #ffffff（规格「dominant background color」）。
 */
export const uberTheme: ThemeDefinition = {
  id: 'uber',
  label: 'Uber',
  family: 'minimal',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: uberSwatches,
  antd: uberAntdTheme,
  slots: uberSlots,
}
