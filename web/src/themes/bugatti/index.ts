import type { ThemeDefinition, ThemeSwatches } from '../types'
import { bugattiAntdTheme } from './antd'
import { bugattiSlots } from './slots'

/** bugatti 主题五色示意条（外观面板展示）：纯黑画布 + 卡面 + 白墨 + 白主色 + 冰蓝。 */
export const bugattiSwatches: ThemeSwatches = {
  canvas: '#000000',
  surface: '#141414',
  ink: '#ffffff',
  primary: '#ffffff',
  accent: '#c3d9f3',
}

/**
 * bugatti 主题 = Bugatti 设计语言（暗色）。
 * 纯黑画布 #000000 + 纪念碑式大写宽字距展示排版（排版签名：Bugatti Display
 * 三体分立——Display 承担全部标题、Text Regular 衬线承文、Monospace 承按钮/
 * 标签/导航，字重一律 400，强调只靠字号/字距/大小写）。全系统无彩色——
 * 唯一非单色是冰蓝链接 #c3d9f3；主 CTA = 透明底 1px 白描边药丸；圆角二值
 * （除按钮全 0px）；零阴影零装饰，层次全靠明度阶梯
 * #000000→#0d0d0d→#141414→#1f1f1f 与发丝线 #262626/#3a3a3a。
 * CSS 令牌见 themes/css/bugatti.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor 取 --canvas（#000000；首屏防白闪映射同值，纯黑是规格主导背景）。
 */
export const bugattiTheme: ThemeDefinition = {
  id: 'bugatti',
  label: 'Bugatti',
  family: 'dark',
  isDark: true,
  chromeColor: '#000000',
  swatches: bugattiSwatches,
  antd: bugattiAntdTheme,
  slots: bugattiSlots,
}
