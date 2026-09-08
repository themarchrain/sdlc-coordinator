import type { ThemeDefinition, ThemeSwatches } from '../types'
import { clayAntdTheme } from './antd'
import { claySlots } from './slots'

/** clay 主题五色示意条（外观面板展示）。primary 与 ink 同为近黑——
 *  Clay 的主 CTA 本就是「近黑墨」而非彩色（先例：nike/wired 同构）。 */
export const claySwatches: ThemeSwatches = {
  canvas: '#fffaf0',
  surface: '#f5f0e0',
  ink: '#0a0a0a',
  primary: '#0a0a0a',
  accent: '#ff4d8b',
}

/**
 * clay 主题 = Clay.com 设计语言。
 * 暖奶油 #FFFAF0 画布 + 近黑 #0A0A0A 墨与主 CTA；品牌电压 = 6 色饱和单色卡
 * （pink/teal/lavender/peach/ochre/cream）与 3D 黏土插画；圆角慷慨
 * （12 按钮 / 16 内容卡 / 24 特色卡）；海拔近零（发丝线分层，
 * 罕见悬浮态才给低 alpha 微影）；display 字重 500 + 负字距（Plain Black
 * 的 Inter 替身），标题 600，暖色贯穿到底（连页脚都是奶油色）。
 * CSS 令牌见 themes/css/clay.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = cream canvas #FFFAF0（规格「the warmth differentiates Clay」）。
 */
export const clayTheme: ThemeDefinition = {
  id: 'clay',
  label: 'Clay',
  family: 'warm',
  isDark: false,
  chromeColor: '#fffaf0',
  swatches: claySwatches,
  antd: clayAntdTheme,
  slots: claySlots,
}
