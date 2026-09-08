import type { ThemeDefinition, ThemeSwatches } from '../types'
import { binanceAntdTheme } from './antd'
import { binanceSlots } from './slots'

/** binance 主题五色示意条（外观面板展示）：深近黑画布 + 卡面色 + 白墨 + 品牌黄 + 涨绿。 */
export const binanceSwatches: ThemeSwatches = {
  canvas: '#0b0e11',
  surface: '#1e2329',
  ink: '#ffffff',
  primary: '#fcd535',
  accent: '#0ecb81',
}

/**
 * binance 主题 = Binance 设计语言（暗色）。
 * 深近黑画布 #0b0e11（带微冷调、never pure black）+ 色块分层
 * #0b0e11→#1e2329→#2b3139（边即表面台阶，扁平零装饰）；Binance Yellow
 * #FCD535 承载全部主 CTA/品牌强调，黄底黑字是系统签名；BinanceNova/
 * BinancePlex 字栈（数字一律 Plex）、克制的字重（标题 600/正文 400，信任
 * 字号与黄电压而非粗字重）；圆角刻度 2/4/6/8/12，药丸只给顶部注册类 CTA；
 * 涨绿跌红贯穿全部分析语义。多主题产品按默认轨（marketing dark）落地。
 * CSS 令牌见 themes/css/binance.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor 取 --canvas（#0b0e11；首屏防白闪映射同值）。
 */
export const binanceTheme: ThemeDefinition = {
  id: 'binance',
  label: 'Binance',
  family: 'dark',
  isDark: true,
  chromeColor: '#0b0e11',
  swatches: binanceSwatches,
  antd: binanceAntdTheme,
  slots: binanceSlots,
}
