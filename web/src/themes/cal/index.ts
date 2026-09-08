import type { ThemeDefinition, ThemeSwatches } from '../types'
import { calAntdTheme } from './antd'
import { calSlots } from './slots'

/** cal 主题五色示意条（外观面板展示）。primary 与 ink 同为近黑 —— 品牌真相（黑 CTA）。 */
export const calSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f5f5f5',
  ink: '#111111',
  primary: '#111111',
  accent: '#3b82f6', // brand-accent 蓝：稀缺内联强调（链接 hover / 徽章高亮）
}

/**
 * cal 主题 = Cal.com 设计语言（slug cal）。
 * 友好现代 SaaS：白画布 + 近黑主 CTA #111111（单色动作层，蓝 #3B82F6 仅稀缺内联）；
 * Cal Sans 展示字族 600 + 负字距（Inter 替代）、灰面三档 #F8F9FA/#F5F5F5/#E5E7EB、
 * 卡片圆角 8/12/16 分档、药丸套药丸导航（软包裹内白药丸）、
 * 产品 UI 碎片白卡（hairline + 双档微投影）；深色面 #101010 只属于
 * 页脚与 featured 逆转 —— 映射到 json 深块与登录 signal。
 * CSS 令牌见 themes/css/cal.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = 白画布（规格「anchored on white canvas」）。
 */
export const calTheme: ThemeDefinition = {
  id: 'cal',
  label: 'Cal.com',
  family: 'minimal',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: calSwatches,
  antd: calAntdTheme,
  slots: calSlots,
}
