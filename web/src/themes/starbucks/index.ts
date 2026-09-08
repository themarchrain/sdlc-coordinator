import type { ThemeDefinition, ThemeSwatches } from '../types'
import { starbucksAntdTheme } from './antd'
import { starbucksSlots } from './slots'

/** starbucks 主题五色示意条（外观面板展示）。 */
export const starbucksSwatches: ThemeSwatches = {
  canvas: '#f2f0eb',
  surface: '#ffffff',
  ink: '#1f1f1e', /* rgba(0,0,0,.87) 规格墨在奶油画布上的合成 hex（swatch 契约禁 rgba，css 令牌层保留原值） */
  primary: '#00754a',
  accent: '#cba258',
}

/**
 * starbucks 主题 = Starbucks 设计语言。
 * 暖奶油旗舰零售：Neutral Warm #f2f0eb 画布与 Ceramic #edebe9 交替 + 白卡 +
 * 四档绿角色制（Starbucks #006241 标题 / Accent #00754a CTA / House #1e3932
 * bookend 深带 / Uplift #2b5148 装饰）；金 #cba258 只落在 Rewards 仪式位
 * （登录展示板星点/chip 与一条图表数据墨）；50px 全药丸按钮 + scale(0.95)
 * 签名按压、12px 圆角卡 + 耳语双层影、SoDoSans 紧排 -0.01em、字重带 400–600。
 * CSS 令牌见 themes/css/starbucks.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = Neutral Warm（「the warm cream primary page canvas」）。
 */
export const starbucksTheme: ThemeDefinition = {
  id: 'starbucks',
  label: 'Starbucks',
  family: 'warm',
  isDark: false,
  chromeColor: '#f2f0eb',
  swatches: starbucksSwatches,
  antd: starbucksAntdTheme,
  slots: starbucksSlots,
}
