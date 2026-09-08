import type { ThemeDefinition, ThemeSwatches } from '../types'
import { wiseAntdTheme } from './antd'
import { wiseSlots } from './slots'

/** wise 主题五色示意条（外观面板展示）：sage 画布 + 白卡 + 暖黑墨 + Wise 绿 + 深林绿。 */
export const wiseSwatches: ThemeSwatches = {
  canvas: '#e8ebe6',
  surface: '#ffffff',
  ink: '#0e0f0c',
  primary: '#9fe870',
  accent: '#163300',
}

/**
 * wise 主题 = Wise 设计语言。
 * 北欧金融杂志：sage 鼠尾草画布 #e8ebe6 与白卡靠表面对比分层（Level 2 海拔，
 * 「the surface contrast IS the elevation」），暖橄榄近黑墨 #0e0f0c，唯一品牌色 =
 * 柠檬绿 CTA 专色 #9fe870——绿面永远压近黑字（on-primary），hover/active 提亮到
 * #cdffad 的签名反转；规格明言品牌绿绝不兼成功色，成功语义走 positive #2ead4b。
 * 24px 圆角是按钮与卡的签名（「never sharp corners」），输入/三级按钮保留
 * 1px ink 边（Level 1 海拔）；字重两极：display 900（Wise Sans → Inter 栈近似）/
 * 其余 600。CSS 令牌见 themes/css/wise.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = #e8ebe6（sage 页画布，即本主题 --canvas）。
 */
export const wiseTheme: ThemeDefinition = {
  id: 'wise',
  label: 'Wise',
  family: 'brand',
  isDark: false,
  chromeColor: '#e8ebe6',
  swatches: wiseSwatches,
  antd: wiseAntdTheme,
  slots: wiseSlots,
}
