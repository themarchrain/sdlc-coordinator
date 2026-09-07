import type { ThemeConfig } from 'antd'

/**
 * apple 主题的 antd ThemeConfig —— 自 design/apple main.tsx 原样搬移。
 *
 * Apple 设计语言：Action Blue 是唯一交互色；ink 近黑；发丝线；珍珠灰画布。
 * algorithm 未指定 = antd 默认算法（apple 源即如此）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/apple.css 的同名令牌手工保持同源：
 *   colorPrimary  = --primary          (#0066cc)
 *   colorSuccess  = 系统绿              (#34c759，slots.timeline.success 同源)
 *   colorWarning  = 系统橙              (#ff9500，slots.timeline.warning 同源)
 *   colorError    = 系统红              (#ff3b30，slots.timeline.error 同源)
 *   colorText     = --ink              (#1d1d1f)
 *   colorTextSecondary = 文字 48% 阶    (#7a7a7a)
 *   colorBgLayout = --canvas           (#f5f5f7)
 *   colorBorder   = --hairline-strong  (#d2d2d7)
 *   colorBorderSecondary / colorSplit = --hairline (#e0e0e0)
 *   fontFamily    = --font-sans（SF Pro 系统栈近似）
 */
export const appleAntdTheme: ThemeConfig = {
  token: {
    colorPrimary: '#0066cc',
    colorInfo: '#0066cc',
    colorLink: '#0066cc',
    colorSuccess: '#34c759',
    colorWarning: '#ff9500',
    colorError: '#ff3b30',
    colorText: '#1d1d1f',
    colorTextSecondary: '#7a7a7a',
    colorTextTertiary: '#7a7a7a',
    colorBgLayout: '#f5f5f7',
    colorBgContainer: '#ffffff',
    colorBorder: '#d2d2d7',
    colorBorderSecondary: '#e0e0e0',
    colorSplit: '#e0e0e0',
    borderRadius: 8,
    borderRadiusSM: 5,
    borderRadiusLG: 18,
    fontFamily: '-apple-system, BlinkMacSystemFont, "SF Pro Text", "SF Pro Display", system-ui, "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
  },
  components: {
    Button: {
      // 主按钮走苹果 pill 语法（button-primary）
      borderRadius: 9999,
      borderRadiusSM: 9999,
      borderRadiusLG: 9999,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Tag: {
      // 状态签用 pill chip 语法（configurator-option-chip）
      borderRadiusSM: 9999,
    },
    Table: {
      headerBg: '#ffffff',
      headerColor: '#7a7a7a',
      headerSplitColor: 'transparent',
      rowHoverBg: '#fafafc',
    },
    Tooltip: {
      colorBgSpotlight: '#1d1d1f',
    },
  },
}
