import type { ThemeConfig } from 'antd'

/**
 * MiniMax 主题的 antd ThemeConfig —— 自 minimax 源 main.tsx 原样搬移。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/minimax.css 的同名令牌手工保持同源：
 *   colorPrimary  = --primary      (#0a0a0a 黑药丸)
 *   colorSuccess  = --success      (#1ba673)
 *   colorWarning  ≠ --warning      (源即两值：antd #ad6800 / css #97650d，忠实保留)
 *   colorError    = --error        (#d45656)
 *   colorText     = 文字链 body 级 (#222222)
 *   colorBg 三键   = --canvas       (#ffffff 白画布)
 *   fontFamily    = --font-sans    (DM Sans 近似系统栈)
 */
export const minimaxAntdTheme: ThemeConfig = {
  token: {
    // MiniMax 设计语言：近黑主色药丸 CTA + 白画布 + 发丝线 + 灰阶文字链
    colorPrimary: '#0a0a0a',
    colorInfo: '#1456f0',
    colorLink: '#1d4ed8',
    colorLinkHover: '#17437d',
    colorSuccess: '#1ba673',
    colorSuccessBg: '#e8ffea',
    colorWarning: '#ad6800',
    colorWarningBg: '#fdf3d9',
    colorError: '#d45656',
    colorErrorBg: '#fdecec',
    colorText: '#222222',
    colorTextSecondary: '#45515e',
    colorTextTertiary: '#5f5f5f',
    colorTextQuaternary: '#8e8e93',
    colorBorder: '#e5e7eb',
    colorBorderSecondary: '#eaecf0',
    colorBgLayout: '#ffffff',
    colorBgContainer: '#ffffff',
    colorBgElevated: '#ffffff',
    borderRadius: 8,
    borderRadiusSM: 6,
    borderRadiusLG: 16,
    controlHeight: 40,
    fontSize: 14,
    fontFamily: '"DM Sans", system-ui, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
  },
  components: {
    Select: { optionSelectedBg: 'rgba(34, 34, 34, 0.08)' },
    Table: {
      headerBg: '#f7f8fa',
      headerColor: '#5f5f5f',
      headerSplitColor: 'transparent',
      rowHoverBg: '#f7f8fa',
      borderColor: '#eaecf0',
    },
    Descriptions: {
      colorFillAlter: '#f7f8fa',
      colorSplit: '#eaecf0',
    },
  },
}
