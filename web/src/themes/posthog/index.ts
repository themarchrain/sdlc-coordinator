import type { ThemeDefinition, ThemeSwatches } from '../types'
import { posthogAntdTheme } from './antd'
import { posthogSlots } from './slots'

/** posthog 主题五色示意条（外观面板展示）。 */
export const posthogSwatches: ThemeSwatches = {
  canvas: '#eeefe9',
  surface: '#ffffff',
  ink: '#23251d',
  primary: '#f7a501',
  accent: '#2c84e0',
}

/**
 * posthog 主题 = PostHog 产品分析品牌。
 * #eeefe9 暖灰白（cream）画布连绵到底、白卡薄橄榄边平铺（4-6px 圆角、零投影）；
 * 单一饱和黄橙 #f7a501 药丸 CTA 承载全部主操作（黄底配深橄榄字 #23251d，
 * 规格 on-primary 字面值）；IBM Plex Sans Variable 400/500/600/700/800 字重
 * 阶梯（眉题 uppercase、display 800 负字距）；四彩 soft 通告族
 * （blue/green/red/purple）只进 info/语义/图表位；深橄榄炭 #23251d 仅作
 * json 深码岛、tooltip 与登录 showcase（白卡内暗码块的沉面时刻）。
 * CSS 令牌见 themes/css/posthog.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = canvas（规格「warm cream page background」）。
 */
export const posthogTheme: ThemeDefinition = {
  id: 'posthog',
  label: 'PostHog',
  family: 'brand',
  isDark: false,
  chromeColor: '#eeefe9',
  swatches: posthogSwatches,
  antd: posthogAntdTheme,
  slots: posthogSlots,
}
