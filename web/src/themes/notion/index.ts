import type { ThemeDefinition, ThemeSwatches } from '../types'
import { notionAntdTheme } from './antd'
import { notionSlots } from './slots'

/** notion 主题五色示意条（外观面板展示）。 */
export const notionSwatches: ThemeSwatches = {
  canvas: '#f6f5f4',
  surface: '#ffffff',
  ink: '#000000',
  primary: '#0075de',
  accent: '#213183',
}

/**
 * notion 主题 = Notion 设计语言。
 * 纸上文档：暖纸画布 #F6F5F4 + 白卡 figure/ground + 近黑 Inter 墨（标题 700 + 负字距），
 * 唯一结构性蓝 #0075DE 只落在 CTA/链接/激活信号；贴纸彩盘（天蓝/粉/橙/青/绿）
 * 只做装饰（brand-mark 贴纸条、登录 signal 星座），不刷结构。
 * 形态签名：药丸 CTA + 4px 输入角 + 12px 工作卡 + 多层近透明微叠阴影；
 * Deep Indigo #213183「夜带」倒置面只给登录 showcase 与 json/tooltip 暗岛。
 * CSS 令牌见 themes/css/notion.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = Warm Paper（规格「signature page canvas」主导底）。
 */
export const notionTheme: ThemeDefinition = {
  id: 'notion',
  label: 'Notion',
  family: 'minimal',
  isDark: false,
  chromeColor: '#f6f5f4',
  swatches: notionSwatches,
  antd: notionAntdTheme,
  slots: notionSlots,
}
