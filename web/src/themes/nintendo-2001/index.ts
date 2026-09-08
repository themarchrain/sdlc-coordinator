import type { ThemeDefinition, ThemeSwatches } from '../types'
import { nintendo2001ThemeConfig } from './antd'
import { nintendo2001Slots } from './slots'

/** nintendo-2001 主题五色示意条（外观面板展示）：佩瑞汪金属画布 + 白面板 + 暖信号。 */
export const nintendo2001Swatches: ThemeSwatches = {
  canvas: '#7a8aba',
  surface: '#ffffff',
  ink: '#21242e',
  primary: '#f68d1f',
  accent: '#ecab37',
}

/**
 * nintendo-2001 主题 = Nintendo.com 2001 设计语言（Y2K 主机面板）。
 * CSS 令牌见 themes/css/nintendo-2001.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 *
 * isDark = false：画布 #7a8aba 虽是中明度拉丝金属，但整套界面是亮色极性——
 * 内容面是白/铂灰卡，正文墨是近黑 carbon（#21242e 在画布上 4.5:1，白字仅 3.4:1），
 * 规格 Text 节亦自称 "primary text on light"、示例卡注明 "light mode"。
 * 碳黑只属于命令层（导航/页脚），不构成暗色主题。
 * chromeColor 取画布 #7a8aba（首屏底即主机面板金属色）。
 */
export const nintendo2001Theme: ThemeDefinition = {
  id: 'nintendo-2001',
  label: 'Nintendo 2001',
  family: 'retro',
  isDark: false,
  chromeColor: '#7a8aba',
  swatches: nintendo2001Swatches,
  antd: nintendo2001ThemeConfig,
  slots: nintendo2001Slots,
}
