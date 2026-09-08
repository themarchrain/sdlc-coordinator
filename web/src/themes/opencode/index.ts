import type { ThemeDefinition, ThemeSwatches } from '../types'
import { opencodeAntdTheme } from './antd'
import { opencodeSlots } from './slots'

/** opencode 主题五色示意条（外观面板展示）。 */
export const opencodeSwatches: ThemeSwatches = {
  canvas: '#fdfcfc',
  surface: '#f1eeee',
  ink: '#201d1d',
  primary: '#201d1d',
  accent: '#007aff',
}

/**
 * opencode 主题 = OpenCode 设计语言。
 * 终端原生 manpage 美学：全站 100% Berkeley Mono 等宽（商业字体禁外部加载，
 * 以规格替代栈 JetBrains Mono → IBM Plex Mono → 系统等宽近似）；
 * 暖奶油画布 #fdfcfc（亮色勘误定案）+ 近黑墨 #201d1d；容器 0px 直角、
 * 交互件 4px 方角（圆角二值律），零阴影零渐变，分层只靠 1px 暖调发丝线；
 * 墨即品牌色（主 CTA 墨底奶油字，按压落 ink-deep #0f0000），链接 = 墨 +
 * 下划线表意；Apple HIG 语义 ramp（#007aff/#ff3b30/#ff9f0a/#30d158）归
 * in-TUI 数据语境；brand-mark = 像素块 ASCII 字标的三墨条直角重绘，
 * json 卡 / 登录 showcase = hero TUI 暗面（#201d1d）的唯一时刻。
 * CSS 令牌见 themes/css/opencode.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = 暖奶油（规格 canvas #fdfcfc）。
 */
export const opencodeTheme: ThemeDefinition = {
  id: 'opencode',
  label: 'OpenCode',
  family: 'minimal',
  isDark: false,
  chromeColor: '#fdfcfc',
  swatches: opencodeSwatches,
  antd: opencodeAntdTheme,
  slots: opencodeSlots,
}
