import type { ThemeDefinition, ThemeSwatches } from '../types'
import { zapierAntdTheme } from './antd'
import { zapierSlots } from './slots'

/** zapier 主题五色示意条（外观面板展示）。 */
export const zapierSwatches: ThemeSwatches = {
  canvas: '#fffefb',
  surface: '#f8f4f0',
  ink: '#201515',
  primary: '#ff4f00',
  accent: '#201515', // 品牌只有「橙 + 奶油 + 咖啡」三词，accent 位如实填咖啡墨
}

/**
 * zapier 主题 = Zapier 设计语言。
 * 要点：暖白画布 #FFFEFB + 软奶油 #F8F4F0（「温度即品牌」，禁纯白）+
 * 深咖啡墨 #201515（禁纯黑，暖意贯穿文字）；唯一饱和橙 #FF4F00 承担一切
 * 主 CTA/链接（禁第二彩色）；海拔 = 软奶油填充对比（Level 2 无边框），
 * 控件边走 1px 墨线（Level 1）；卡片与按钮一律 12px 中位圆角（禁药丸 CTA）；
 * Degular Display 500 只给 hero、Inter 包办其余；eyebrow 全大写 +1px 正字距
 * 是品牌签名，display 标题则永不大写。
 * CSS 令牌见 themes/css/zapier.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = Warm Canvas（规格「dominant background」）。
 */
export const zapierTheme: ThemeDefinition = {
  id: 'zapier',
  label: 'Zapier',
  family: 'brand',
  isDark: false,
  chromeColor: '#fffefb',
  swatches: zapierSwatches,
  antd: zapierAntdTheme,
  slots: zapierSlots,
}
