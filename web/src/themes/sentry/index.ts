import type { ThemeDefinition, ThemeSwatches } from '../types'
import { sentryAntdTheme } from './antd'
import { sentrySlots } from './slots'

/** sentry 主题五色示意条（外观面板展示）：午夜紫画布 + night 卡 + 白 + lime。 */
export const sentrySwatches: ThemeSwatches = {
  canvas: '#1f1633',
  surface: '#150f23',
  ink: '#ffffff',
  primary: '#ffffff', // 暗极性主操作 = 白（button-inverted），与 ink 同值是规格使然
  accent: '#c2ef4e',
}

/**
 * sentry 主题 = Sentry 设计语言（暗色）。
 * 深紫午夜画布 #1f1633 + night 卡 #150f23（带 hairline-violet #362d59 边，无投影，
 * 深度靠纹理与插画）+ electric lime #c2ef4e 高亮 chip 语汇 + hot pink #fa7faa 标点；
 * 主操作随极性反转：暗画布上 = 白底墨紫字（button-inverted），故 primary 取 #ffffff
 * 而非令牌表 #150f23（后者与画布仅 1.09:1，作按钮面不可见；已保留为卡面/浮层）。
 * chunky 展示字（Rubik 重字重近似）+ Monaco 代码 + 大写 0.2px 字距的 button-cap 节奏。
 * CSS 令牌见 themes/css/sentry.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = --canvas（#1f1633；首屏防白闪映射同值）。
 */
export const sentryTheme: ThemeDefinition = {
  id: 'sentry',
  label: 'Sentry',
  family: 'dark',
  isDark: true,
  chromeColor: '#1f1633',
  swatches: sentrySwatches,
  antd: sentryAntdTheme,
  slots: sentrySlots,
}
