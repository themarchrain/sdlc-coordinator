import type { ThemeDefinition, ThemeSwatches } from '../types'
import { shopifyAntdTheme } from './antd'
import { shopifySlots } from './slots'

/** shopify 主题五色示意条（外观面板展示）：乳白事务轨 + 白卡 + 墨 + aloe。 */
export const shopifySwatches: ThemeSwatches = {
  canvas: '#fbfbf5',
  surface: '#ffffff',
  ink: '#000000',
  primary: '#000000',
  accent: '#c1fbd4',
}

/**
 * shopify 主题 = Shopifi 双轨设计语言的事务轨（设计语言；
 * 文档 name 写作 Shopifi 为原文笔误，label 用 Shopify）。
 * 规格明言双轨：营销轨 = 近黑 canvas-night #000000 + 满铺摄影 + 330 细字重巨型 display
 * + 白描边黑药丸 CTA；事务轨（pricing/signup/dashboards）= 乳白画布（卡 #ffffff /
 * 页底 cream #fbfbf5）+ pastel aloe #c1fbd4 / pistachio #d4f9e0 绿只作事务成功面
 * + 同款黑色实底药丸词汇 + Inter 正文 + 全站 ss03 字形集。
 * 管理台即 dashboards → 默认取事务轨（isDark=false，canvas 走 light/cream）；
 * night 轨只作登录 showcase / json 卡 / tooltip 的局部取材面（规格「never blended」
 * → 不混画布、只借装饰局部）。骨架表达不了的形态（药丸导航、330 细字重数值、
 * ss03、黑底展示板）见 themes/css/shopify.css 块后片段。
 * CSS 令牌见 themes/css/shopify.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = #fbfbf5（事务轨 pricing 页画布 canvas-cream，即本主题 --canvas）。
 */
export const shopifyTheme: ThemeDefinition = {
  id: 'shopify',
  label: 'Shopify',
  family: 'brand',
  isDark: false,
  chromeColor: '#fbfbf5',
  swatches: shopifySwatches,
  antd: shopifyAntdTheme,
  slots: shopifySlots,
}
