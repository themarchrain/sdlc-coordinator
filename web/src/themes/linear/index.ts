import type { ThemeDefinition, ThemeSwatches } from '../types'
import { linearAntdTheme } from './antd'
import { linearSlots } from './slots'

/** linear 主题五色示意条（外观面板展示）：近黑画布 + 炭面板 + 浅灰墨 + 薰衣草一族。 */
export const linearSwatches: ThemeSwatches = {
  canvas: '#010102',
  surface: '#0f1011',
  ink: '#f7f8f8',
  primary: '#5e6ad2',
  accent: '#828fff',
}

/**
 * linear 主题 = Linear 设计语言（暗色）。
 * 软件工艺文档气质：#010102 近黑画布（全场最深暗面，一丝蓝调的锚定色）+
 * 四级表面阶梯（#0f1011/#141516/#18191a/#191a1b）+ 浅灰墨文字 #f7f8f8；
 * 签名淡薰衣草蓝 #5e6ad2 为唯一彩色，只落在 brand-mark、主 CTA、focus ring、
 * 链接强调（hover 提亮 #828fff）；海拔不用投影 = 表面阶梯 + 三级发丝线
 * （#23252a/#34343a/#3e3e44）+ 顶部白缘高光；显示排版 600 + 负字距，抗 700+。
 * CSS 令牌见 themes/css/linear.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor 取 --canvas 实际值 #010102（暗色主题防白闪底色映射同值；
 * 规格明令禁 #000000 纯黑画布，蓝调近黑是刻意的）。
 */
export const linearTheme: ThemeDefinition = {
  id: 'linear',
  label: 'Linear',
  family: 'dark',
  isDark: true,
  chromeColor: '#010102',
  swatches: linearSwatches,
  antd: linearAntdTheme,
  slots: linearSlots,
}
