import type { ThemeDefinition, ThemeSwatches } from '../types'
import { togetherAntdTheme } from './antd'
import { togetherSlots } from './slots'

/** together 主题五色示意条（外观面板展示）。 */
export const togetherSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#ebebeb',
  ink: '#000000',
  primary: '#000000',
  accent: '#bdbbff',
}

/**
 * together 主题 = Together AI 设计语言。
 * 黑白带律的极简工程风（本主题取亮轨 Canvas #FFFFFF）：近黑 #010120 hero 带
 * 与纯白产品带交替、无中间灰（唯一软面 #EBEBEB 只做表头轨）；唯一黑 #000000
 * CTA（4px 微圆角矩形，拒绝全药丸）承担全部转化；全部装饰 = 一条三色
 * 橙-品红-长春花渐变丝带（只进 hero 尺度，本主题落在登录 showcase 的
 * 工程蓝图细线网格上）；几何 display sans（The Future → Inter 替代，
 * 500 字重负字距、全站无 bold）+ 大写 mono 眉题（按钮标签/表头）二部制；
 * 卡片 4px 圆角 + 发丝线，浅面零软影（藏青软投影只给浮层）。
 * CSS 令牌见 themes/css/together.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = Canvas 白（规格「The default product / pricing / docs background」）。
 */
export const togetherTheme: ThemeDefinition = {
  id: 'together',
  label: 'Together',
  family: 'minimal',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: togetherSwatches,
  antd: togetherAntdTheme,
  slots: togetherSlots,
}
