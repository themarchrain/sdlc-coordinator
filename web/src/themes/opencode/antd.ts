import type { ThemeConfig } from 'antd'

/**
 * opencode 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * OpenCode 设计语言 = 终端原生的 manpage 美学：100% Berkeley Mono 等宽
 * （商业字体禁外部加载 → 规格替代栈 JetBrains Mono → IBM Plex Mono → 系统等宽）、
 * 暖奶油画布 #FDFCFC（亮色勘误定案）、近黑墨 #201D1D 即品牌色（主 CTA =
 * 墨底奶油字 4px 方钮，按压落到 ink-deep #0F0000）；圆角二值律：交互件
 * 一律 4px、容器一律 0px；零阴影零渐变，分层只靠 1px 暖调发丝线
 * rgba(15,0,0,.12) 与 hairline-strong #646262。语义板 = 完整 Apple HIG ramp
 * （accent #007AFF / danger #FF3B30 / warning #FF9F0A / success #30D158），
 * 规格明言该色板「belongs to the in-product TUI」——管理台的数据面板即
 * TUI 语境，照单取用；链接按规格走墨色下划线而非蓝（link-inline 原文）。
 * algorithm 未指定 = antd 默认算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/opencode.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#201D1D ink；墨即品牌，CTA 墨底)
 *   colorPrimaryActive      = --primary-active  (#0F0000 Ink Deep，规格按压态字面)
 *   colorPrimaryHover       = 派生 #181515      （规格「无 hover 记载」，向 ink-deep 推半阶）
 *   colorInfo               = --info            (#007AFF Apple Blue = 主信息信号)
 *   colorLink               = --link            (#201D1D 规格「links use ink rather than accent blue」)
 *   colorSuccess/Warning/Error = HIG ramp      (#30D158 / #FF9F0A / #FF3B30)
 *   colorText 四阶          = Ink/Body/Mute/Ash (#201D1D / #424245 / #646262 / #9A9898，四级)
 *   colorBgLayout/Container/Elevated = --canvas/--canvas-card (#FDFCFC 奶油；卡片即画布)
 *   colorBorder 双阶 + colorSplit = 暖调发丝线 (rgba(15,0,0,.12)；强线 #646262 归 CSS 片段的线钮)
 *   borderRadius 4          = 规格 rounded.sm（「every interactive element」）
 *   borderRadiusLG 0        = 规格 rounded.none（「every container that isn't a button」）
 *   fontWeightStrong 700    = 规格 display/heading 同字重 700，靠字号分层
 *   fontFamily/fontFamilyCode = --font-sans（全站等宽栈；sans 禁入）
 */
export const opencodeAntdTheme: ThemeConfig = {
  token: {
    // 墨即品牌：主 CTA = 墨底奶油字（「the brand's only color」）
    colorPrimary: '#201D1D',
    colorPrimaryHover: '#181515', // 派生：规格无 hover 记载，向 ink-deep 推半阶
    colorPrimaryActive: '#0F0000', // Ink Deep：规格 button-primary-active 字面色
    // Apple Blue = 主信息信号（规格 accent：信息/命令高亮归 in-TUI，管理台数据面板即 TUI 语境）
    colorInfo: '#007AFF',
    // 链接 = 墨 + 下划线表意（规格 link-inline「ink rather than accent blue」）
    colorLink: '#201D1D',
    colorLinkHover: '#0F0000',
    // 语义板 = 完整 Apple HIG ramp（原文照单）
    colorSuccess: '#30D158',
    colorWarning: '#FF9F0A',
    colorError: '#FF3B30',
    // 文字四阶：Ink / Body / Mute / Ash（四级原文）
    colorText: '#201D1D',
    colorTextSecondary: '#424245',
    colorTextTertiary: '#646262',
    colorTextQuaternary: '#9A9898',
    // 暖奶油画布：页面/容器/浮层同面（「every page body, every card」）
    colorBgLayout: '#FDFCFC',
    colorBgContainer: '#FDFCFC',
    colorBgElevated: '#FDFCFC',
    // 1px 暖调发丝线承担全部分层（强线 #646262 由 CSS 片段给线钮）
    colorBorder: 'rgba(15, 0, 0, 0.12)',
    colorBorderSecondary: 'rgba(15, 0, 0, 0.12)',
    colorSplit: 'rgba(15, 0, 0, 0.12)',
    // 圆角二值律：交互件 4px / 容器 0px（「two values」）
    borderRadius: 4,
    borderRadiusSM: 4,
    borderRadiusLG: 0,
    // 标题与 display 同字重 700，层级只靠字号（原则）
    fontWeightStrong: 700,
    // 零阴影体系（「Nothing lifts, nothing floats」）
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // 100% Berkeley Mono（商业字体禁外部加载）→ 规格替代栈（规格 字体替代注记）
    fontFamily:
      '"Berkeley Mono", "JetBrains Mono", "IBM Plex Mono", ui-monospace, "SFMono-Regular", Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", "PingFang SC", "Microsoft YaHei", monospace',
    fontFamilyCode:
      '"Berkeley Mono", "JetBrains Mono", "IBM Plex Mono", ui-monospace, "SFMono-Regular", Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", "PingFang SC", "Microsoft YaHei", monospace',
  },
  components: {
    Select: { optionSelectedBg: 'rgba(32, 29, 29, 0.08)' },
    Button: {
      // button-md 16px/500/行高 2；三态阴影全关；线钮 1px hairline-strong 边由 CSS 片段承担
      fontWeight: 500,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 当前项 = 透明底 + 墨字（button-tab-active 规格），2px ash 下划线由 CSS 片段承担
      itemSelectedBg: 'transparent',
      itemSelectedColor: '#201D1D',
    },
    Table: {
      // 扁表头：透明面 + Mute 标签 + 暖发丝缝；行 hover = surface-soft 交替染（规格「alternating row tint」）
      headerBg: 'transparent',
      headerColor: '#646262',
      headerSplitColor: 'rgba(15, 0, 0, 0.12)',
      rowHoverBg: '#F8F7F7',
      borderColor: 'rgba(15, 0, 0, 0.12)',
    },
    Tooltip: {
      // 浮层 = surface-dark 终端面（TUI 暗面的合法场景）
      colorBgSpotlight: '#201D1D',
    },
  },
}
