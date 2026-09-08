import type { ThemeDefinition, ThemeSwatches } from '../types'
import { coinbaseAntdTheme } from './antd'
import { coinbaseSlots } from './slots'

/** coinbase 主题五色示意条（外观面板展示）。 */
export const coinbaseSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f7f7f7',
  ink: '#0a0b0d',
  primary: '#0052ff',
  accent: '#05b169', // 语义 up 绿：蓝之外唯一允许的第二电压（仅文字/数据）
}

/**
 * coinbase 主题 = Coinbase 设计语言。
 * 机构级金融品牌的编辑级冷静：白画布 + 软灰交替带 + 墨阶文字，
 * 唯一品牌电压 Coinbase Blue #0052FF 稀缺使用（每 band 一两个蓝色时刻）；
 * 展示字重 400 不 700（CoinbaseDisplay）、数字一律 CoinbaseMono、
 * 全药丸 CTA + 24px 大圆卡 + 单一阴影层 0 4px 12px rgba(0,0,0,.04)；
 * 暗 editorial hero（#0A0B0D 底 + #16181C 浮卡）轮转节奏映射到
 * json 深块与登录 showcase；交易语义 up/down 只作文字色。
 * CSS 令牌见 themes/css/coinbase.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = 纯白（规格「The base canvas is pure white」）。
 */
export const coinbaseTheme: ThemeDefinition = {
  id: 'coinbase',
  label: 'Coinbase',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: coinbaseSwatches,
  antd: coinbaseAntdTheme,
  slots: coinbaseSlots,
}
