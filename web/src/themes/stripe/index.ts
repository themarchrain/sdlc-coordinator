import type { ThemeDefinition, ThemeSwatches } from '../types'
import { stripeAntdTheme } from './antd'
import { stripeSlots } from './slots'

/** stripe 主题五色示意条（外观面板展示）：白画布 + 海军蓝墨 + 靛蓝主色 + ruby 点缀。 */
export const stripeSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f6f9fc',
  ink: '#0d253d',
  primary: '#533afd',
  accent: '#ea2261',
}

/**
 * stripe 主题 = Stripe 设计语言（亮色）。
 * 白画布 + 冷调 soft #f6f9fc + 深海军蓝墨 #0d253d；品牌靛蓝 #533afd 是唯一填充 CTA
 * （一屏一枚药丸）；排版签名 = sohne 系 300 细字重 + 负字距 display、tnum 表格数字；
 * 深度靠渐变 mesh 氛围（奶油/橙/薰衣草/靛蓝/ruby 横向洗过 hero 上三分之一）
 * 与蓝调投影，不靠装饰阴影；语义三色按「渐变停靠点之外不加新点缀色」
 * 的禁令推导为克制绿/lemon/ruby。
 * CSS 令牌见 themes/css/stripe.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor 取 --canvas（#ffffff；亮色主题防白闪映射即画布白）。
 */
export const stripeTheme: ThemeDefinition = {
  id: 'stripe',
  label: 'Stripe',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: stripeSwatches,
  antd: stripeAntdTheme,
  slots: stripeSlots,
}
