import type { ThemeDefinition, ThemeSwatches } from '../types'
import { superhumanAntdTheme } from './antd'
import { superhumanSlots } from './slots'

/** superhuman 主题五色示意条（外观面板展示）。accent = violet-soft：紫光晕签名色。 */
export const superhumanSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#fafaf8',
  ink: '#292827',
  primary: '#1b1938',
  accent: '#c9b4fa',
}

/**
 * superhuman 主题 = Superhuman 营销语言。
 * 三画布系统的亮色投影：白画布 #ffffff + 暖墨 #292827（never pure black）+
 * 近黑靛蓝 #1b1938 主 CTA（8px 圆角矩形）；正文黑白 stark、彩色不进数据面，
 * 紫光晕签名（violet-soft #c9b4fa）只落展示板类装饰位（brand-mark 光晕、
 * 登录 hero 的 violet-sky 大气渐变）；深青 #155555 作信息/成功族音。
 * 字族 = Super Sans VF 钦定替代 Inter Variable，子默认字重 460/540 暖感签名、
 * 展示位负字距与 0.96 紧凑行高（编辑密度）。
 * CSS 令牌见 themes/css/superhuman.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = #ffffff（规格「the body of every page flips to white」）。
 */
export const superhumanTheme: ThemeDefinition = {
  id: 'superhuman',
  label: 'Superhuman',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: superhumanSwatches,
  antd: superhumanAntdTheme,
  slots: superhumanSlots,
}
