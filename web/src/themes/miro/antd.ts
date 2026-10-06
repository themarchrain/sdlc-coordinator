import type { ThemeConfig } from 'antd'

/**
 * miro 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Miro 设计语言 = AI 白板工作台：纯白画布 + 近黑墨 + 黑色药丸主 CTA
 * （primary #1C1C1E 是规格的「dominant interactive element」，黄不是 CTA），
 * Miro 黄 #FFD02F 只属于 wordmark / 顶部 promo / 黄标签 chip 三处；
 * 行内链接与聚焦 = 动作蓝 #4262FF（规格 Brand Blue）；
 * 文字阶梯 = Ink Deep #050038 → Ink #1C1C1E → Charcoal #2C2C34 → Slate #555A6A
 *   → Steel #6B6F7E → Stone #8E91A0 → Muted #A5A8B5（七级原文）；
 * 发丝线双轨 = Hairline #E0E2E8 / Soft #EEF0F3 / Strong #C7CAD5；
 * 圆角刻度 4/6/8/12/16/20/28/32/9999（「the pill is a brand signature」→
 * Button 钉 9999 药丸）；字重只有 400/500/600（无 700，规格「single weight scale」）；
 * 海拔以白板 mockup 阴影族 rgba(5,0,56,x) 表达，平面卡不投影。
 * algorithm 未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/miro.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#1C1C1E，黑药丸 CTA)
 *   colorPrimaryHover/Active = --primary-active 族（规格只记 pressed = Charcoal
 *                              #2C2C34「lifts to charcoal」，hover 无记载，从 pressed）
 *   colorInfo/colorLink     = --link            (#4262FF Brand Blue，inline link 色)
 *   colorLinkHover/Active   = blue-450/blue-pressed (#5B76FE / #2A41B6，规格蓝三阶)
 *   colorInfoBg/colorInfoBorder = --info-soft/--hairline-strong
 *                              (#F5F3FF 规格 lavender「AI agent tag」面 + Strong 边)
 *   colorSuccess/Warning/Error = 语义三色（warning/error 为派生，见行内注释）
 *   colorText 四阶           = Ink / Slate / Steel / Stone（规格文字七级的四档抽稀）
 *   colorBgLayout/Container/Elevated = --canvas/--canvas-card (#FFFFFF 纯白画布)
 *   colorBorder 双阶 + colorSplit = Strong #C7CAD5 / Hairline #E0E2E8 / Soft #EEF0F3
 *   borderRadius 8          = 规格 rounded.md（inputs / search-pill / 表容器）
 *   borderRadiusLG 16       = 规格 rounded.xl（pricing 卡 / 大面板）
 *   fontFamily              = --font-sans（Roobert PRO 的规格回退栈，禁外部加载）
 */
export const miroAntdTheme: ThemeConfig = {
  token: {
    // 黑药丸主 CTA：Miro 的主导交互元素
    colorPrimary: '#1C1C1E',
    // 规格 no-hover policy：只记 pressed「lifts to Charcoal」#2C2C34，hover 从 pressed
    colorPrimaryHover: '#2C2C34',
    colorPrimaryActive: '#2C2C34',
    // info / link = 动作蓝 Brand Blue（规格：inline links 与 featured 边框的取色）
    colorInfo: '#4262FF',
    colorLink: '#4262FF',
    colorLinkHover: '#5B76FE', // 规格 blue-450
    colorLinkActive: '#2A41B6', // 规格 blue-pressed
    // info Alert = lavender 软面（规格 badge-tag-purple 面 + Brand Blue 字的形态）
    colorInfoBg: '#F5F3FF',
    colorInfoBorder: '#C7CAD5',
    // 语义三色：success = 规格 success-accent；warning/error 规格只给薄纱底与边框档，
    // 文字级取值按同色相推导（warning 锚定 yellow-dark #746019，error 锚定 brand-red 系）
    colorSuccess: '#00B473',
    colorWarning: '#746019',
    colorError: '#C92A2A',
    // 文字阶梯（Ink / Slate / Steel / Stone 四档）
    colorText: '#1C1C1E',
    colorTextSecondary: '#555A6A',
    colorTextTertiary: '#6B6F7E',
    colorTextQuaternary: '#8E91A0',
    // 纯白画布：页面、容器、浮层全白，靠 hairline 分隔（规格 Canvas White 双职）
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线三轨：Strong（控件边）/ Hairline（1px 边）/ Soft（表行分隔）
    colorBorder: '#C7CAD5',
    colorBorderSecondary: '#E0E2E8',
    colorSplit: '#EEF0F3',
    // 圆角：默认 8 = rounded.md（inputs/search/table）；SM 6 = rounded.sm；
    // LG 16 = rounded.xl（pricing 卡/大面板）。按钮药丸 9999 在 components.Button 钉
    borderRadius: 8,
    borderRadiusSM: 6,
    borderRadiusLG: 16,
    // 字重刻度 400/500/600（规格「Roobert PRO does not use 700」）；
    // 标题/强调 = 500，600 只给 badge/micro-uppercase（css 片段）
    fontWeightStrong: 500,
    // 海拔 = 白板 mockup 阴影族 rgba(5,0,56,x)：浮层用 Level 4 modal / Level 3 mockup
    boxShadow: 'rgba(5, 0, 56, 0.12) 0px 16px 48px -8px',
    boxShadowSecondary: 'rgba(5, 0, 56, 0.08) 0px 12px 32px -4px',
    // Known Gaps：「recommend 150–200ms ease」→ 三档钉在区间内
    motionDurationFast: '0.15s',
    motionDurationMid: '0.18s',
    motionDurationSlow: '0.2s',
    // 触达高：规格区间 40–44px，取 40 保密度（登录主 CTA 钉 44，见 css 片段）
    controlHeight: 40,
    // Roobert PRO（定制族，禁外部加载）→ 规格 fallback：Noto Sans / -apple-system /
    // BlinkMacSystemFont，再接 CJK 安全栈
    fontFamily:
      '"Roobert PRO", "Noto Sans", -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
  },
  components: {
    Select: { optionSelectedBg: 'rgba(28, 28, 30, 0.08)' },
    Button: {
      // 药丸是品牌签名（规格 Don't：「Don't soften corners on buttons」）
      borderRadius: 9999,
      // 按钮字重 500（规格 button-md 14px/500）；平面体系三态阴影全关
      fontWeight: 500,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = 黄标签 chip 形态（黄的三处合法用法之一）
      itemSelectedBg: '#FFF8E0',
      itemSelectedColor: '#1C1C1E',
    },
    Table: {
      // 白底表头 + Slate 标签 + Soft 行缝；行 hover = Surface #F7F8FA
      headerBg: '#FFFFFF',
      headerColor: '#555A6A',
      headerSplitColor: 'transparent',
      rowHoverBg: '#F7F8FA',
      borderColor: '#EEF0F3',
    },
    Tooltip: {
      colorBgSpotlight: '#1C1C1E',
    },
  },
}
