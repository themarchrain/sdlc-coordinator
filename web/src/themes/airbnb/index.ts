import type { ThemeDefinition, ThemeSwatches } from '../types'
import { airbnbAntdTheme } from './antd'
import { airbnbSlots } from './slots'

/** airbnb 主题五色示意条（外观面板展示）。 */
export const airbnbSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f7f7f7',
  ink: '#222222',
  primary: '#ff385c',
  accent: '#428bff',
}

/**
 * airbnb 主题 = Airbnb 设计语言。
 * 慷慨的消费市场：纯白画布 + 近黑墨 #222222（从不纯黑）+ 单一品牌电压 Rausch
 * #FF385C 只落在主 CTA/用量轨/头部指标；字重克制（展示 500–600，正文 400），
 * 摄影与留白扛视觉重量。全软角（按钮 8 / 卡 14 / 徽章·搜索球药丸），
 * 全系统仅一档阴影（rgba 三层字面值）；导航激活 = 墨字 + 墨下划线（product-tab），
 * 焦点 = 2px 墨边无光晕；Luxe 紫/Plus 品红按规格只属子品牌、不入主线。
 * CSS 令牌见 themes/css/airbnb.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = 纯白（规格「The base canvas is pure white」主导底）。
 */
export const airbnbTheme: ThemeDefinition = {
  id: 'airbnb',
  label: 'Airbnb',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: airbnbSwatches,
  antd: airbnbAntdTheme,
  slots: airbnbSlots,
}
