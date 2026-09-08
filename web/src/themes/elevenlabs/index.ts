import type { ThemeDefinition, ThemeSwatches } from '../types'
import { elevenlabsAntdTheme } from './antd'
import { elevenlabsSlots } from './slots'

/** elevenlabs 主题五色示意条（外观面板展示）：暖纸底 + 白卡 + 墨 + 药丸墨 + 薄荷球。 */
export const elevenlabsSwatches: ThemeSwatches = {
  canvas: '#f5f5f5',
  surface: '#ffffff',
  ink: '#0c0a09',
  primary: '#292524',
  accent: '#a7e5d3',
}

/**
 * elevenlabs 主题 = ElevenLabs 设计语言。
 * 暖纸编辑杂志：离白画布 #f5f5f5 + 暖近黑墨 #0c0a09；display 走 Waldenburg
 * Light weight-300 编辑感衬线（负字距、永不 bold），body 走 Inter 400/500
 * （+0.15–0.18px 微字距）；主 CTA = 近黑墨药丸（#292524 底白字，按压
 * #0c0a09）；海拔 = 发丝线 + 单层软投影 0 4px 16px rgba(0,0,0,.04)。
 * 柔和渐变球（mint #a7e5d3 / peach #f4c5a8 / lavender #c8b8e0 /
 * sky #a8c8e8 / rose #e8b8c4）是唯一彩色时刻，只做氛围装饰（登录
 * showcase 与渐变球瓦），不引入任何饱和 CTA 色。CSS 令牌见
 * themes/css/elevenlabs.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = #f5f5f5（--canvas 实际值；规格「dominant background」为离白画布）。
 */
export const elevenlabsTheme: ThemeDefinition = {
  id: 'elevenlabs',
  label: 'ElevenLabs',
  family: 'warm',
  isDark: false,
  chromeColor: '#f5f5f5',
  swatches: elevenlabsSwatches,
  antd: elevenlabsAntdTheme,
  slots: elevenlabsSlots,
}
