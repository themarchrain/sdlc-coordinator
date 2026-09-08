import type { ThemeDefinition, ThemeSwatches } from '../types'
import { nikeAntdTheme } from './antd'
import { nikeSlots } from './slots'

/** nike 主题五色示意条（外观面板展示）。 */
export const nikeSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f5f5f5',
  ink: '#111111',
  primary: '#111111',
  accent: '#d30005',
}

/**
 * nike 主题 = Nike 设计语言。
 * 摄影优先 + 极端排版对比：纯黑/纯白/单一软灰的近单色 chrome，Nike Black #111111
 * 是唯一主色（主 CTA 与 ink 同源，故 primary = ink）；色只留给信号（sale 红 /
 * success 绿 / info 蓝）。形态 = CTA 全 pill、容器全方角、零投影、1px 发丝线、
 * 巨型全大写 condensed 展示排版（Futura ND 的 Arial Narrow/Impact 系统栈近似）。
 * CSS 令牌见 themes/css/nike.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = Pure White（规格 canvas，页面底色）。
 */
export const nikeTheme: ThemeDefinition = {
  id: 'nike',
  label: 'Nike',
  family: 'minimal',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: nikeSwatches,
  antd: nikeAntdTheme,
  slots: nikeSlots,
}
