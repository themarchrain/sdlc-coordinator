import type { ThemeDefinition, ThemeSwatches } from '../types'
import { pinterestAntdTheme } from './antd'
import { pinterestSlots } from './slots'

/** pinterest 主题五色示意条（外观面板展示）。 */
export const pinterestSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f6f6f3',
  ink: '#000000',
  primary: '#e60023',
  accent: '#7e238b',
}

/**
 * pinterest 主题 = Pinterest 设计语言。
 * 摄影优先：暖奶油中性 chrome（surface-card #f6f6f3）让路给内容，
 * Pinterest Red #e60023 是唯一高饱和色（主 CTA / 激活 tab / 品牌字，从不装饰）；
 * 双圆角语汇 16px + 32px + 药丸、卡面全平（唯一阴影 = 模态 16px ambient）、
 * Pin Sans（→ Inter 替代）层级全靠字重 400→600→700、负字距只给大标题。
 * CSS 令牌见 themes/css/pinterest.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = canvas #ffffff（白画布）。
 */
export const pinterestTheme: ThemeDefinition = {
  id: 'pinterest',
  label: 'Pinterest',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: pinterestSwatches,
  antd: pinterestAntdTheme,
  slots: pinterestSlots,
}
