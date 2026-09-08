import type { ThemeDefinition, ThemeSwatches } from '../types'
import { mobbinAntdTheme } from './antd'
import { mobbinSlots } from './slots'

/** mobbin 主题五色示意条（外观面板展示）。 */
export const mobbinSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f3f3f3',
  ink: '#141414',
  primary: '#141414',
  accent: '#0066ff',
}

/**
 * mobbin 主题 = Mobbin 设计语言。
 * 画廊白单色系：纯白画布 + 近黑墨 #141414，结构全靠「极浅中性染阶」
 * （soft #f3f3f3 / field #f0f0f0 / 发丝线 #e0e0e0）与留白，零阴影零渐变；
 * 交互语汇 = stadium 药丸（按钮/徽标 9999px，CSS 片段承担）+ 输入 16px；
 * 卡片几何 24px、brand-mark 为 iOS 30% squircle 墨面白条；
 * 唯一彩色 Electric Blue #0066ff 只承担正向/信息信号（规格「commercial
 * signals」在管理台语境下迁移为 success/info/link）；Saans 652/456 字重
 * 以 650 + 规格替代栈（Inter/系统 sans）近似。
 * CSS 令牌见 themes/css/mobbin.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = 画廊白（规格 canvas #ffffff）。
 */
export const mobbinTheme: ThemeDefinition = {
  id: 'mobbin',
  label: 'Mobbin',
  family: 'minimal',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: mobbinSwatches,
  antd: mobbinAntdTheme,
  slots: mobbinSlots,
}
