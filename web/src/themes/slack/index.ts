import type { ThemeDefinition, ThemeSwatches } from '../types'
import { slackAntdTheme } from './antd'
import { slackSlots } from './slots'

/** slack 主题五色示意条（外观面板展示）：白画布 + cream 柔面 + 茄紫 + 蓝链接。 */
export const slackSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f4ede4',
  ink: '#1d1d1d',
  primary: '#4a154b',
  accent: '#1264a3',
}

/**
 * slack 主题 = Slack 设计语言（设计语言；
 * 规格名 Slacc 为匿名化笔误，label 用 Slack）。
 * 茄紫一神教：#ffffff 画布 + cream #f4ede4 / lavender #f9f0ff 柔面，
 * aubergine #4A154B 是唯一品牌面色（CTA/featured/footer/wordmark），
 * link-blue #1264A3 为唯一彩色点缀，多彩 pastel-mesh 渐变（peach/lavender/
 * dusty green）按规格克制地只落在登录 showcase 展示板。药丸按钮 90px
 * 圆角 + over-padded 内距是全站唯一按钮形状；display 档 700 + 负字距，
 * 双字族（Salesforce Avant Garde/Sans → 规格替代 Inter）。海拔 =
 * Level 1 浮层影 + 茄紫 inset 焦点环，「渐变即深度」。brand-mark =
 * 茄紫 squircle + 三根白短杠 ±45° 交错（交叠菱纹的抽象）。
 * CSS 令牌见 themes/css/slack.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = #FFFFFF（规格 canvas 白）。
 */
export const slackTheme: ThemeDefinition = {
  id: 'slack',
  label: 'Slack',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: slackSwatches,
  antd: slackAntdTheme,
  slots: slackSlots,
}
