import type { ThemeDefinition, ThemeSwatches } from '../types'
import { composioAntdTheme } from './antd'
import { composioSlots } from './slots'

/** composio 主题五色示意条（外观面板展示）：近黑画布 + 灰阶表面 + 深电蓝一族。 */
export const composioSwatches: ThemeSwatches = {
  canvas: '#0f0f0f',
  surface: '#181818',
  ink: '#ffffff',
  primary: '#0007cd',
  accent: '#1a26ff',
}

/**
 * composio 主题 = Composio 设计语言（暗色）。
 * 近黑画布 #0f0f0f + 纯黑终端凹陷 #000000，海拔全靠亮度步进（#181818 卡 →
 * #222222 面板 → #2a2a2a 下拉），不发投影；唯一强调 = 深电蓝电压
 * #0007cd（CTA/wordmark/spotlight 辉光 #1a26ff），蓝保持稀缺；
 * 8px CTA / 12-16px 卡的开发者方言圆角；abcDiatype → Inter 替身 +
 * JetBrains Mono 代码面；签名 = 2×2 终端面板网格 + 中央蓝色聚光辉光。
 * CSS 令牌见 themes/css/composio.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = --canvas（#0f0f0f；暗色首屏防白闪映射同值）。
 */
export const composioTheme: ThemeDefinition = {
  id: 'composio',
  label: 'Composio',
  family: 'dark',
  isDark: true,
  chromeColor: '#0f0f0f',
  swatches: composioSwatches,
  antd: composioAntdTheme,
  slots: composioSlots,
}
