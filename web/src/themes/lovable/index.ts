import type { ThemeDefinition, ThemeSwatches } from '../types'
import { lovableAntdTheme } from './antd'
import { lovableSlots } from './slots'

/** lovable 主题五色示意条（外观面板展示）。 */
export const lovableSwatches: ThemeSwatches = {
  canvas: '#f7f4ed',
  surface: '#fcfbf8',
  ink: '#1c1c1c',
  primary: '#1c1c1c',
  accent: '#3b82f6',
}

/**
 * lovable 主题 = Lovable 设计语言。
 * 暖纸编辑风：羊皮纸奶油画布 #f7f4ed + 近黑炭墨 #1c1c1c（炭墨同时是主 CTA），
 * 全站灰阶由炭墨透明度派生（0.03 / 0.04 / 0.4 / 0.82–0.83），
 * 边线双轨 Light Cream #eceae4（被动）/ 炭墨 40%（交互），卡不投影、边线即容器，
 * 暗钮签名内阴影 + 软焦点阴影（0 4px 12px rgba(0,0,0,.1)），字重窄幅 400/600，
 * 标题负字距（-0.9px ~ -1.5px），药丸 9999px 只给图标容器与 chips。
 * 唯一功能性彩色 = Ring Blue #3b82f6（无障碍焦点环）。
 * CSS 令牌见 themes/css/lovable.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = Cream（「Background: Cream (#f7f4ed)」主导底）。
 */
export const lovableTheme: ThemeDefinition = {
  id: 'lovable',
  label: 'Lovable',
  family: 'warm',
  isDark: false,
  chromeColor: '#f7f4ed',
  swatches: lovableSwatches,
  antd: lovableAntdTheme,
  slots: lovableSlots,
}
