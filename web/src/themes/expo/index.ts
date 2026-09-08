import type { ThemeDefinition, ThemeSwatches } from '../types'
import { expoAntdTheme } from './antd'
import { expoSlots } from './slots'

/** expo 主题五色示意条（外观面板展示）。 */
export const expoSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f0f0f3',
  ink: '#171717',
  primary: '#000000',
  accent: '#0d74ce',
}

/**
 * expo 主题 = Expo 设计语言。
 * 安静自信的开发者平台：纯白画布 #FFFFFF + soft #FAFAFA 交替带、近黑墨
 * #171717、唯一品牌电压 = 纯黑 CTA #000000（按压 #1A1A1A），蓝 #0D74CE
 * 只做行内链接色；天空蓝渐变 wash 只落在登录展示板（hero 专属 atmosphere），
 * device-mockup 语汇 = 展示板内 16px 圆角白预览卡。Inter 单族（display 600
 * 封顶）+ JetBrains Mono 码面；CTA 8px / 卡 12px、药丸只给徽章、海拔单档
 * 0 4px 12px 4% 墨。
 * CSS 令牌见 themes/css/expo.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = Canvas #FFFFFF（规格「pure white page floor」主导底色）。
 */
export const expoTheme: ThemeDefinition = {
  id: 'expo',
  label: 'Expo',
  family: 'minimal',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: expoSwatches,
  antd: expoAntdTheme,
  slots: expoSlots,
}
