import type { ThemeDefinition, ThemeSwatches } from '../types'
import { spotifyAntdTheme } from './antd'
import { spotifySlots } from './slots'

/** spotify 主题五色示意条（外观面板展示）：近黑茧房 + 白字 + 品牌绿一族。 */
export const spotifySwatches: ThemeSwatches = {
  canvas: '#121212',
  surface: '#181818',
  ink: '#ffffff',
  primary: '#1db954',
  accent: '#1ed760',
}

/**
 * spotify 主题 = Spotify 设计语言（暗色）。
 * 近黑画布提升阶梯 #121212/#181818/#1f1f1f + 品牌绿（#1db954 基 / #1ed760 hover）
 * + 粗字重排版 + 药丸按钮；绿只用于 play/激活/主 CTA，绿底黑字。
 * CSS 令牌见 themes/css/spotify.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor 取 --canvas（#121212；首屏防白闪映射同值）。
 */
export const spotifyTheme: ThemeDefinition = {
  id: 'spotify',
  label: 'Spotify',
  family: 'dark',
  isDark: true,
  chromeColor: '#121212',
  swatches: spotifySwatches,
  antd: spotifyAntdTheme,
  slots: spotifySlots,
}
