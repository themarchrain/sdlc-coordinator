import type { ThemeDefinition, ThemeSwatches } from '../types'
import { mistralAntdTheme } from './antd'
import { mistralSlots } from './slots'

/** mistral 主题五色示意条（外观面板展示）。 */
export const mistralSwatches: ThemeSwatches = {
  canvas: '#fff8e0',
  surface: '#ffffff',
  ink: '#1f1f1f',
  primary: '#fa520f',
  accent: '#ffd900',
}

/**
 * mistral 主题 = Mistral AI 设计语言。
 * 大气日落编辑风：暖奶油黄画布（cream #FFF8E0）+ 白卡（beige-deep #E6D5A8 边）+
 * 饱和橙 #FA520F 唯一 CTA（按压 #CC3A05）；签名 sunset stripe
 * （primary → sunshine → yellow → cream 渐变带）回声在 brand-mark 三条柱、
 * 登录展示板渐变天空与页尾渐变条上；展示级交给近衬线 PP Editorial Old
 * （--font-display，Georgia/Times 近似），UI 全 Inter；按钮 8px / 卡片 12px、
 * 禁药丸，海拔克制（Level 1 微影起）。
 * CSS 令牌见 themes/css/mistral.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = cream #FFF8E0（规格「warm yellow-cream surfaces」主导底色）。
 */
export const mistralTheme: ThemeDefinition = {
  id: 'mistral',
  label: 'Mistral AI',
  family: 'warm',
  isDark: false,
  chromeColor: '#fff8e0',
  swatches: mistralSwatches,
  antd: mistralAntdTheme,
  slots: mistralSlots,
}
