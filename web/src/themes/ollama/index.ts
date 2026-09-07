import type { ThemeDefinition, ThemeSwatches } from '../types'
import { ollamaAntdTheme } from './antd'
import { ollamaSlots } from './slots'

/** ollama 主题五色示意条（外观面板展示）：黑主色 + 唯一暗面作 accent 候选。 */
export const ollamaSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#ffffff',
  ink: '#000000',
  primary: '#000000',
  accent: '#171717',
}

/**
 * ollama 主题 = Ollama 设计语言（design/ollama）。
 * 纸白画布 + 单色黑药丸 CTA + 终端红黄绿唯一彩色；
 * #171717 是全应用唯一暗面（登录 showcase），作 swatch accent 候选。
 * CSS 令牌见 themes/css/ollama.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor 取 ollama 源 index.html 的 meta theme-color（纯白）。
 */
export const ollamaTheme: ThemeDefinition = {
  id: 'ollama',
  label: 'Ollama',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: ollamaSwatches,
  antd: ollamaAntdTheme,
  slots: ollamaSlots,
}
