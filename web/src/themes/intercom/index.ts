import type { ThemeDefinition, ThemeSwatches } from '../types'
import { intercomAntdTheme } from './antd'
import { intercomSlots } from './slots'

/** intercom 主题五色示意条（外观面板展示）。 */
export const intercomSwatches: ThemeSwatches = {
  canvas: '#f5f1ec',
  surface: '#ffffff',
  ink: '#111111',
  primary: '#111111',
  accent: '#ff5600',
}

/**
 * intercom 主题 = Intercom 设计语言。
 * 要点：暖奶油画布 #F5F1EC（规格明令禁纯白画布）+ 炭墨 #111111 一族文字；
 * **炭墨即系统主色**（button-primary = 墨底白字，按压转 #000），
 * Fin Orange #FF5600 只作 Fin 徽标级强调（swatch accent 位 / brand-dot），绝不装饰化；
 * 海拔 = 白卡浮上奶油的明度差（零投影、零渐变），发丝线 #D3CEC6 勾边；
 * Saans 500 展示 + 400 正文，负字距只给展示级，eyebrow 句首大写禁全大写宽字距；
 * 圆角克制 8/12/16（禁药丸 CTA，药丸只留 tab 切换）；分析图表走 report 色板。
 * CSS 令牌见 themes/css/intercom.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = Cream Canvas（规格「dominant background」）。
 */
export const intercomTheme: ThemeDefinition = {
  id: 'intercom',
  label: 'Intercom',
  family: 'warm',
  isDark: false,
  chromeColor: '#f5f1ec',
  swatches: intercomSwatches,
  antd: intercomAntdTheme,
  slots: intercomSlots,
}
