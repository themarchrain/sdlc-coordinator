import type { ThemeDefinition, ThemeSwatches } from '../types'
import { cohereAntdTheme } from './antd'
import { cohereSlots } from './slots'

/** cohere 主题五色示意条（外观面板展示）。 */
export const cohereSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#eeece7',
  ink: '#212121',
  primary: '#17171c',
  accent: '#ff7759',
}

/**
 * cohere 主题 = Cohere 设计语言。
 * 白画布编辑部 + 暗色产品带：near-black #17171C 药丸 CTA 承担全部主操作
 * （主色即近黑，不是彩色），action-blue #1863DC 是链接/分页的数据墨水，
 * soft-stone #EEECE7 暖面与 coral #FF7759 小记号提供暖黄绿温度；
 * 近全 400/500 字重、圆角刻度 4/8/16/22 + 药丸、近零海拔（发丝线分隔）。
 * CSS 令牌见 themes/css/cohere.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = Canvas White（规格「dominant page background」）。
 */
export const cohereTheme: ThemeDefinition = {
  id: 'cohere',
  label: 'Cohere',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: cohereSwatches,
  antd: cohereAntdTheme,
  slots: cohereSlots,
}
