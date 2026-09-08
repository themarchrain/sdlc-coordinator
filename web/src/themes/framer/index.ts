import type { ThemeDefinition, ThemeSwatches } from '../types'
import { framerAntdTheme } from './antd'
import { framerSlots } from './slots'

/** framer 主题五色示意条（外观面板展示）：黑画板 + 白墨 + 白药丸 + 信号蓝。 */
export const framerSwatches: ThemeSwatches = {
  canvas: '#090909',
  surface: '#141414',
  ink: '#ffffff',
  primary: '#ffffff',
  accent: '#0099ff',
}

/**
 * framer 主题 = Framer 设计语言（暗色）。
 * 近纯黑画板 #090909 + 提升阶梯 surface-1 #141414 / surface-2 #1c1c1c；
 * 文字二元律（白 #ffffff 或 #999999，无中间灰）；主 CTA = 白药丸
 * （#ffffff 底 #000000 字）；唯一彩色信号 Sky Blue #0099ff 只给链接 /
 * focus 环 / 选中指示；层级靠字号 + 负字距（display 500 负字距是招牌），
 * 全站无 bold；渐变氛围卡（violet/magenta/orange/coral）是仅有的彩色时刻，
 * 收敛在登录 showcase。CSS 令牌见 themes/css/framer.css；antd 对象见
 * ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = #090909（--canvas 实际值；暗色主题首屏防白闪底色映射同值）。
 */
export const framerTheme: ThemeDefinition = {
  id: 'framer',
  label: 'Framer',
  family: 'dark',
  isDark: true,
  chromeColor: '#090909',
  swatches: framerSwatches,
  antd: framerAntdTheme,
  slots: framerSlots,
}
