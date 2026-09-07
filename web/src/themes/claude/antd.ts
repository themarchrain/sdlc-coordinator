import type { ThemeConfig } from 'antd'

/**
 * claude 主题的 antd ThemeConfig —— 自 claude 源 main.tsx 的 themeTokens
 * 与 components 覆写原样搬移（字面 hex，禁 var()）。
 *
 * 与 themes/css/claude.css 的同名令牌手工保持同源：
 *   colorPrimary      = --primary        (#cc785c 珊瑚)
 *   colorPrimaryActive/LinkHover = --primary-active (#a9583e)
 *   colorInfo         = --bar-usage      (#5db8a6 辅色 teal，状态点/处理中)
 *   colorSuccess      = timeline success 槽位 (#5db872；CSS --success 槽按源取 teal context-dot)
 *   colorText         = --ink            (#3d3d3a 暖墨正文)
 *   colorTextHeading  = --ink-strong     (#141413)
 *   colorTextSecondary= --body-alt       (#6c6a64)
 *   colorBgLayout     = --canvas         (#faf9f5 奶油画布)
 *   colorBorder       = --hairline       (#e6dfd8 暖发丝线)
 *   colorFill         = --surface-cream-strong 家族四阶
 *   controlOutline    = claude text-input-focused 规范（珊瑚 15% 外环，3px）
 */
export const claudeAntdTheme: ThemeConfig = {
  token: {
    // 珊瑚主色：CTA/链接/选中态；按下加深为 primary-active
    colorPrimary: '#cc785c',
    colorPrimaryActive: '#a9583e',
    colorPrimaryHover: '#d4886e',
    colorLink: '#cc785c',
    colorLinkHover: '#a9583e',
    // 信息态使用辅色 teal（状态点/处理中），珊瑚保持稀缺
    colorInfo: '#5db8a6',
    colorSuccess: '#5db872',
    colorWarning: '#d4a017',
    colorError: '#c64545',
    // 奶油画布与表面
    colorBgLayout: '#faf9f5',
    colorBgContainer: '#faf9f5',
    colorBgElevated: '#faf9f5',
    // 暖发丝线
    colorBorder: '#e6dfd8',
    colorBorderSecondary: '#ebe6df',
    // 暖墨文本
    colorTextBase: '#141413',
    colorTextHeading: '#141413',
    colorText: '#3d3d3a',
    colorTextSecondary: '#6c6a64',
    colorTextTertiary: '#8e8b82',
    colorTextQuaternary: '#8e8b82',
    // 暖调填充（hover / 选中底等中间态）
    colorFill: '#e8e0d2',
    colorFillSecondary: '#efe9de',
    colorFillTertiary: '#f5f0e8',
    colorFillQuaternary: '#f5f0e8',
    // 焦点：珊瑚 15% 外环（text-input-focused 规范）
    controlOutline: 'rgba(204, 120, 92, 0.15)',
    controlOutlineWidth: 3,
    // 尺度：按钮/输入高 40，圆角 md=8 / lg=12 / sm=6
    controlHeight: 40,
    borderRadius: 8,
    borderRadiusLG: 12,
    borderRadiusSM: 6,
    // 人文无衬线正文（衬线展示标题由 CSS --font-display 分工承担）
    fontFamily:
      "StyreneB, Inter, system-ui, -apple-system, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
  },
  components: {
    Button: { fontWeight: 500, primaryShadow: 'none', defaultShadow: 'none', dangerShadow: 'none' },
    Table: { headerBg: '#f5f0e8', headerColor: '#6c6a64', rowHoverBg: '#f5f0e8', headerSplitColor: 'transparent', borderColor: '#e6dfd8' },
    Tag: { defaultBg: '#efe9de', defaultColor: '#3d3d3a' },
    Segmented: { trackBg: '#efe9de', itemSelectedBg: '#faf9f5' },
    Tooltip: { colorBgSpotlight: '#252320' },
    Input: { hoverBorderColor: '#d8cfc0', activeBorderColor: '#cc785c', activeShadow: '0 0 0 3px rgba(204, 120, 92, 0.15)' },
  },
}
