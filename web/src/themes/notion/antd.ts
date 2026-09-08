import type { ThemeConfig } from 'antd'

/**
 * notion 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Notion 设计语言 = 纸上文档：暖纸画布 #F6F5F4 + 白卡 figure/ground + 近黑 Inter 墨
 * + 唯一结构性蓝 #0075DE（CTA / 链接 / 激活信号）；贴纸彩盘只做装饰不刷结构。
 * 阴影 = 多层近透明微叠（Level 1 字面值），圆角 = 表单 4 / 行 5 / 卡 12 / 药丸 CTA full，
 * 标题 700 + 负字距、正文 400（字重对比是第一表现杠杆）。
 * algorithm 未指定 = antd 默认算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/notion.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#0075DE，规格「the single structural accent」)
 *   colorPrimaryActive      = --primary-active  (#005BAB，规格「Pressed Blue」原文)
 *   colorPrimaryHover       = 派生 #0068C4（主蓝与按压态中点；规格 只记按压更深）
 *   colorInfo/colorLink     = --info/--link     (#0075DE，蓝即唯一交互信号)
 *   colorInfoBg             = --info-soft       (#EAF5FC，蓝 8% 薄纱)
 *   colorInfoBorder         = --hairline        (#E6E6E6，规格发丝线)
 *   colorSuccess            = Sticker Green #1AAE39（规格「affirmative ticks」指定语义承担色）
 *   colorWarning            = Sticker Orange #DD5B00（派生：规格无警示色 → 贴纸橙承担）
 *   colorError              = #E03E3A（派生：规格无红 → Notion 产品内危险红，贴纸级饱和）
 *   colorText 四阶          = Ink/Charcoal/Stone/Ash (#000000/#31302E/#615D59/#A39E98，规格 文字四级；
 *                             css 侧按规格以 rgba(0,0,0,.95) 呈 95% 软墨，antd 侧须字面 hex 取 #000000)
 *   colorBgLayout           = --canvas          (#F6F5F4，Warm Paper 主导底)
 *   colorBgContainer/Elevated = --canvas-card   (#FFFFFF，白卡/浮层)
 *   colorBorder             = #DDDDDD（text-input 规格 rgb(221,221,221) 字面值）
 *   colorBorderSecondary/colorSplit = --hairline/--hairline-soft (#E6E6E6 / #EDEDEC)
 *   borderRadius 4          = 规格 rounded.xs（输入框/小件「Square-ish corners」）
 *   borderRadiusLG 12       = 规格 rounded.lg（feature-card 12px 工作卡）
 *   boxShadow               = 规格 Level 1 微叠阴影字面值
 *   fontFamily              = --font-sans（NotionInter → Inter 直接替代，规格「Note on Font Substitutes」）
 */
export const notionAntdTheme: ThemeConfig = {
  token: {
    // Notion Blue：唯一结构性交互色（「Brand & Accent」）
    colorPrimary: '#0075DE',
    colorPrimaryHover: '#0068C4',
    colorPrimaryActive: '#005BAB',
    // info / link 与 primary 同蓝：蓝是唯一交互信号（规格 Do「Reserve primary for the primary action, inline links, active/focus」）
    colorInfo: '#0075DE',
    colorLink: '#0075DE',
    colorLinkHover: '#0068C4',
    // info Alert = 蓝 8% 薄纱软面 + 规格发丝线
    colorInfoBg: '#EAF5FC',
    colorInfoBorder: '#E6E6E6',
    // 语义三色：规格明言营销面无语义色板、状态由贴纸盘承担 → 绿=贴纸绿（ affirmative ticks 原文），
    // 橙=贴纸橙（派生），红=产品内危险红（派生，贴纸级饱和度）
    colorSuccess: '#1AAE39',
    colorWarning: '#DD5B00',
    colorError: '#E03E3A',
    // 文字阶梯：Ink / Warm Charcoal / Stone / Ash（「Text」四级原文）
    colorText: '#000000',
    colorTextSecondary: '#31302E',
    colorTextTertiary: '#615D59',
    colorTextQuaternary: '#A39E98',
    // 暖纸画布 + 白卡：figure/ground 靠面差而非投影（规格 Do 第 2 条）
    colorBgLayout: '#F6F5F4',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：控件边 = 输入框规格 #DDDDDD；分隔 = 规格发丝线 #E6E6E6
    colorBorder: '#DDDDDD',
    colorBorderSecondary: '#E6E6E6',
    colorSplit: '#EDEDEC',
    // 圆角：4px 输入/小件（rounded.xs）；12px 大面 feature 卡（rounded.lg）
    borderRadius: 4,
    borderRadiusSM: 4,
    borderRadiusLG: 12,
    // 字重：标题 700 / 正文 400 的对比是第一杠杆
    fontWeightStrong: 700,
    // 海拔 = 多层近透明微叠（字面值），非重投影
    boxShadow:
      'rgba(0, 0, 0, 0.01) 0 0.175px 1.041px, rgba(0, 0, 0, 0.02) 0 0.8px 2.925px, rgba(0, 0, 0, 0.027) 0 2.025px 7.847px, rgba(0, 0, 0, 0.04) 0 4px 18px',
    // Level 2（浮层）：规格 5 层深叠的收敛近似，末档对齐 rgba(0,0,0,0.05) 0 23px 52px
    boxShadowSecondary: 'rgba(0, 0, 0, 0.027) 0 2px 8px, rgba(0, 0, 0, 0.05) 0 23px 52px',
    // NotionInter → Inter 直接替代（规格 字体替代注记）；附 CJK 系统栈
    fontFamily:
      'Inter, -apple-system, "Segoe UI", Helvetica, Arial, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
  },
  components: {
    Button: {
      // 按钮字重 500（规格 typography.button 16px/500）；三态阴影全关（药丸 CTA 扁平）
      fontWeight: 500,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = 蓝 8% 薄纱 + 压深蓝（ex-app-shell-row「activeIndicator = primary」）
      itemSelectedBg: '#EAF5FC',
      itemSelectedColor: '#005BAB',
    },
    Table: {
      // 表头 = Warm Paper 软带 + Stone 标签（ex-data-table-cell headerBackground = canvas-soft）
      headerBg: '#F6F5F4',
      headerColor: '#615D59',
      headerSplitColor: 'transparent',
      rowHoverBg: '#F6F5F4',
      borderColor: '#E6E6E6',
    },
    Tooltip: {
      // 浮层提示 = Deep Indigo 夜色岛（规格「夜带」倒置面，与 --tooltip-bg 同源）
      colorBgSpotlight: '#213183',
    },
  },
}
