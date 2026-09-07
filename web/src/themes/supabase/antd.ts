import type { ThemeConfig } from 'antd'

/**
 * supabase 主题的 antd ThemeConfig —— 自 design/supabase main.tsx 原样搬移。
 *
 * Supabase 设计语言：白画布 + 近黑墨色 + 唯一彩色事件祖母绿 #3ecf8e；
 * 绿底之上文字用近黑 #171717（on-primary 规则，colorTextLightSolid 同源）。
 * algorithm 未指定 = antd 默认算法（supabase 源即如此）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/supabase.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary         (#3ecf8e)
 *   colorPrimaryHover/Active = --primary-active (#24b47e / 源 --primary-deep)
 *   colorInfo          = --ink             (#171717，墨色而非彩色的 info 语义)
 *   colorSuccess       = --primary-active  (#24b47e)
 *   colorText          = --ink             (#171717)
 *   colorTextSecondary = --body            (#707070)
 *   colorTextLightSolid= --on-primary      (#171717，绿底近黑签名)
 *   colorBgLayout      = --canvas          (#ffffff)
 *   colorBorder        = --hairline-card   (#dfdfdf)
 *   colorBorderSecondary / colorSplit = --hairline (-soft) (#ededed)
 *   borderRadius 6 / SM 4 / LG 12         (源圆角刻度 sm/lg/xl)
 *   fontFamily         = --font-sans（Circular 系统栈近似）
 *   fontFamilyCode     = --font-mono
 *   boxShadow 三键     = 源阴影 Level 2/2/1
 */
export const supabaseAntdTheme: ThemeConfig = {
  token: {
    // Supabase 设计语言：白画布 + 近黑墨色 + 唯一彩色事件祖母绿 #3ecf8e。
    colorPrimary: '#3ecf8e',
    colorPrimaryHover: '#24b47e',
    colorPrimaryActive: '#24b47e',
    colorSuccess: '#24b47e',
    colorWarning: '#d48806',
    colorError: '#d14343',
    colorInfo: '#171717',
    colorInfoBg: '#fafafa',
    colorTextBase: '#171717',
    colorText: '#171717',
    colorTextSecondary: '#707070',
    colorTextTertiary: '#9a9a9a',
    colorTextQuaternary: '#b2b2b2',
    // 绿底之上用近黑而不是白色（on-primary 规则）。
    colorTextLightSolid: '#171717',
    colorBgLayout: '#ffffff',
    colorBgContainer: '#ffffff',
    colorBgElevated: '#ffffff',
    colorBorder: '#dfdfdf',
    colorBorderSecondary: '#ededed',
    colorSplit: '#ededed',
    colorLink: '#171717',
    borderRadius: 6,
    borderRadiusSM: 4,
    borderRadiusLG: 12,
    fontFamily:
      "'Circular', 'Helvetica Neue', Helvetica, Arial, 'PingFang SC', 'Microsoft YaHei', sans-serif",
    fontFamilyCode: "ui-monospace, Menlo, Monaco, Consolas, 'Liberation Mono', monospace",
    boxShadow: '0 8px 24px rgba(0,0,0,0.08)',
    boxShadowSecondary: '0 8px 24px rgba(0,0,0,0.08)',
    boxShadowTertiary: '0 1px 3px rgba(0,0,0,0.06)',
  },
  components: {
    Button: { fontWeight: 500, primaryShadow: 'none', defaultShadow: 'none', dangerShadow: 'none' },
    Table: { headerBg: '#fafafa', headerColor: '#707070', headerSplitColor: 'transparent', rowHoverBg: '#fafafa' },
    Tooltip: { colorBgSpotlight: '#1c1c1c', colorTextLightSolid: '#ffffff' },
    Tag: { defaultBg: '#fafafa' },
    Segmented: { trackBg: '#efefef', itemSelectedBg: '#ffffff' },
  },
}
