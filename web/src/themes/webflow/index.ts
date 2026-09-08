import type { ThemeDefinition, ThemeSwatches } from '../types'
import { webflowAntdTheme } from './antd'
import { webflowSlots } from './slots'

/** webflow 主题五色示意条（外观面板展示）。 */
export const webflowSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f5f5f5',
  ink: '#080808',
  primary: '#080808',
  accent: '#7a3dff',
}

/**
 * webflow 主题 = Webflow 设计语言。
 * 慷慨白画布 + 近黑转换色 Ink Black（每个主 CTA、标题、wordmark），
 * 点缀五停 chromatic 类别系统（紫/粉/蓝/橙/绿）——只作整面填充不作按钮底，
 * 管理台落在 brand-mark 三条、趋势紫与 badge 蓝族；WF Visual Sans 语汇
 * （字重天花板 600、display 负字距、大写 eyebrow、caption 550）；
 * 4px 按钮 / 8px 卡片的 tight engineered 圆角，海拔 = 五停分层投影。
 * CSS 令牌见 themes/css/webflow.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = 白画布（规格「the default page background」）。
 */
export const webflowTheme: ThemeDefinition = {
  id: 'webflow',
  label: 'Webflow',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: webflowSwatches,
  antd: webflowAntdTheme,
  slots: webflowSlots,
}
