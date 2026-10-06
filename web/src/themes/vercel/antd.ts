import type { ThemeConfig } from 'antd'

/**
 * vercel 主题的 antd ThemeConfig —— 自 design/vercel main.tsx 原样搬移。
 *
 * Geist (Vercel) 设计语言：colorPrimary 是近黑 ink #171717（黑白体系，
 * 主按钮即黑钮）；交互蓝 #0070f3 承担 info/success/link；按钮 fontWeight 500
 * 且三态无阴影；表头灰字透明分割线。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/vercel.css 的同名令牌手工保持同源：
 *   colorPrimary  = --primary      (#171717 / ink)
 *   colorInfo = colorSuccess = colorLink = --link/--info/--success (#0070f3)
 *   colorLinkHover = --link-hover  (#0761d1)
 *   colorWarning  = --warning     (#f5a623)
 *   colorError    = --error       (#ee0000)
 *   colorText     = --ink         (#171717)
 *   colorTextSecondary/Tertiary/Quaternary = body/mute/faint 四档灰阶
 *   colorBorder   = --hairline    (#ebebeb)
 *   colorBorderSecondary = --hairline-soft (#f2f2f2)
 *   colorBgLayout = --canvas      (#fafafa)
 *   colorBgContainer = colorBgElevated = --canvas-card (#ffffff)
 *   borderRadius  = 6px app chrome（面板 12px 由 CSS 层承担）
 *   fontFamily    = --font-sans（Geist 栈）；fontFamilyCode = --font-mono
 */
export const vercelAntdTheme: ThemeConfig = {
  token: {
    colorPrimary: '#171717',
    colorInfo: '#0070f3',
    colorSuccess: '#0070f3',
    colorWarning: '#f5a623',
    colorError: '#ee0000',
    colorLink: '#0070f3',
    colorLinkHover: '#0761d1',
    colorText: '#171717',
    colorTextSecondary: '#4d4d4d',
    colorTextTertiary: '#8f8f8f',
    colorTextQuaternary: '#a1a1a1',
    colorBorder: '#ebebeb',
    colorBorderSecondary: '#f2f2f2',
    colorBgLayout: '#fafafa',
    colorBgContainer: '#ffffff',
    colorBgElevated: '#ffffff',
    borderRadius: 6,
    fontSize: 14,
    fontFamily: '"Geist", Inter, system-ui, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
    fontFamilyCode: '"Geist Mono", "JetBrains Mono", Consolas, monospace',
  },
  components: {
    Select: { optionSelectedBg: 'rgba(23, 23, 23, 0.08)' },
    Button: {
      // 黑白体系：按钮无阴影，字重 500（源 main.tsx 原样）
      fontWeight: 500,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Table: {
      // 表头灰字 mono 质感 + 透明分割线（CSS 层片段补 font-family）
      headerBg: '#fafafa',
      headerColor: '#8f8f8f',
      headerSplitColor: 'transparent',
      rowHoverBg: '#fafafa',
      borderColor: '#ebebeb',
    },
  },
}
