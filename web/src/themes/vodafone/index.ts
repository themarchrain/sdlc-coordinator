import type { ThemeDefinition, ThemeSwatches } from '../types'
import { vodafoneAntdTheme } from './antd'
import { vodafoneSlots } from './slots'

/** vodafone 主题五色示意条（外观面板展示）。 */
export const vodafoneSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f2f2f2',
  ink: '#25282b',
  primary: '#e60000',
  accent: '#7e7e7e',
}

/**
 * vodafone 主题 = Vodafone 设计语言。
 * 白画布 + 唯一红 #E60000（CTA 药丸 / 演讲符 speechmark 红章 / destructive 信号，
 * 无第二强调色）+ 近黑墨 #25282B（暗带时刻：tooltip/json/登录 showcase 的极性翻转）。
 * 纪念碑式大写 800 排片排版（eyebrow 与登录 hero 的 stencil 语汇）、
 * 交互件全药丸（按钮 9999 / 徽标 32px）、卡面 6px、零软阴影
 * （深度 = 表面对比，规格 Don't：「Don't drop a soft drop-shadow on cards」）。
 * CSS 令牌见 themes/css/vodafone.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = 画布白（规格「canvas #ffffff」）。
 */
export const vodafoneTheme: ThemeDefinition = {
  id: 'vodafone',
  label: 'Vodafone',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: vodafoneSwatches,
  antd: vodafoneAntdTheme,
  slots: vodafoneSlots,
}
