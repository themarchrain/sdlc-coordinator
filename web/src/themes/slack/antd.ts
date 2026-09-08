import type { ThemeConfig } from 'antd'

/**
 * slack 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文/色板（字面 hex，禁 var()）。
 *
 * Slack 设计语言 = 茄紫一神教（chromatic monotheism）+ 奶油/薰衣草柔面：
 * - aubergine #4A154B 是唯一品牌面/CTA 色：药丸主按钮、featured tier、
 *   footer band、wordmark；按压 = primary-press #611F69（lift，hover 无
 *   独立值，复用；primary-deep #481A54 规格明言「功能等价」不启用）；
 * - 亮面双柔底：canvas-cream #F4EDE4（hero/feature 面）+ canvas-lavender
 *   #F9F0FF（次级按钮面/选中面）——表单输入描边仍是 hairline #E6E6E6；
 * - 正文 ink #1D1D1D + ink-mute #696969；链接 = link-blue #1264A3
 *   （体系里唯一非茄紫的彩色点缀），hover #3860BE；
 * - 语义色：success #007A5A / error #CC4117 字面值；规格无 warning →
 *   推导暖琥珀 #975A16（与 error 同族暖度）；
 * - 药丸是唯一按钮形状（rounded.pill 90px，规格「non-negotiable」），
 *   默认钮 = button-outline-aubergine：白底 + 茄紫描边 + 茄紫字；
 * - display 档 700 + 负字距（字距微调见 css 片段）；
 * - 海拔：Level 1 rgba(0,0,0,.1) 0 5px 20px 浮层影；
 *   Level 4 inset 茄紫环 rgb(97,31,105) → controlOutline 同源。
 * algorithm 未指定 = antd 默认算法（亮色）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/slack.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#4A154B aubergine)
 *   colorPrimaryHover/Active = --primary-active (#611F69 primary-press lift)
 *   colorInfo/colorLink     = --info/--link     (#1264A3/#1264A3 link-blue 兼任 info)
 *   colorLinkHover          = --link-hover      (#3860BE 字面值)
 *   colorInfoBg/colorInfoBorder = --info-soft 家族 (#EAF1F8/#CFE0EE 蓝 10% 薄纱，推导)
 *   colorSuccess            = --success         (#007A5A 字面值)
 *   colorWarning            = --warning         (#975A16 推导：规格无 warning)
 *   colorError              = --error           (#CC4117 字面值)
 *   colorText 四阶          = ink/ink-mute/推导灰阶
 *                             (#1D1D1D/#696969/#8C8C8C/#A6A6A6)
 *   colorBgLayout           = --canvas          (#FFFFFF)
 *   colorBgContainer/Elevated = --canvas-card   (#FFFFFF)
 *   colorBorder             = --hairline        (#E6E6E6 text-input 1px 边)
 *   controlOutline          = Level 4 茄紫 inset 环同源 (rgba(97,31,105,.15))
 *   borderRadius 8/4/16     = 规格 rounded.md/sm/xl（输入 4 / 小件 8 / 卡 16）
 *   boxShadow               = Level 1 字面值    (0 5px 20px rgba(0,0,0,.1))
 *   fontFamily              = --font-sans（Salesforce Sans → 规格替代 Inter/Lato）
 */
export const slackAntdTheme: ThemeConfig = {
  token: {
    // aubergine：唯一品牌面色（规格「chromatic monotheism」）
    colorPrimary: '#4A154B',
    colorPrimaryHover: '#611F69', // primary-press（lift）字面值；hover 无独立值，复用
    colorPrimaryActive: '#611F69',
    // info/link = link-blue：体系里唯一非茄紫的彩色（正文链接语汇）
    colorInfo: '#1264A3',
    colorLink: '#1264A3',
    colorLinkHover: '#3860BE',
    colorLinkActive: '#3860BE',
    // info Alert = 蓝 10% 薄纱软面 + 同 hue 淡描边（推导值，与 --info-soft 同源）
    colorInfoBg: '#EAF1F8',
    colorInfoBorder: '#CFE0EE',
    // 语义三色：success/error 字面值；warning 推导暖琥珀（规格未给）
    colorSuccess: '#007A5A',
    colorWarning: '#975A16',
    colorError: '#CC4117',
    // 文字阶梯：ink #1D1D1D + ink-mute #696969 + 推导浅灰两级
    colorText: '#1D1D1D',
    colorTextHeading: '#1D1D1D',
    colorTextSecondary: '#696969',
    colorTextTertiary: '#8C8C8C', // 推导：ink-mute 提亮一档
    colorTextQuaternary: '#A6A6A6', // 推导：占位/禁用级
    // 白画布三面（cream/lavender 只进内嵌面与登录页，见 css 令牌）
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 表单输入描边 = hairline #E6E6E6（规格 text-input 1px 边）
    colorBorder: '#E6E6E6',
    colorBorderSecondary: '#F0F0F0', // 推导：hairline 提亮半档（内部分隔）
    colorSplit: '#F0F0F0',
    // 焦点光环 = Level 4 inset 茄紫环（rgb(97,31,105)）同源
    controlOutline: 'rgba(97, 31, 105, 0.15)',
    // 圆角刻度：rounded.sm 4（hairline tag）/ rounded.md 8（小件）/ rounded.xl 16（卡）
    borderRadius: 8,
    borderRadiusSM: 4,
    borderRadiusLG: 16,
    // display 档 700（规格「display tiers at 700」）
    fontWeightStrong: 700,
    // Level 1 浮层影字面值（按钮平坦，海拔留给卡面）
    boxShadow: '0 5px 20px rgba(0, 0, 0, 0.1)',
    boxShadowSecondary: '0 5px 20px rgba(0, 0, 0, 0.1)',
    // Salesforce Sans（授权族，禁外部加载）→ 规格替代栈 Inter（+ Lato 柔版备选）
    fontFamily:
      "'Salesforce Sans', Inter, Lato, system-ui, -apple-system, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
  },
  components: {
    Button: {
      // 药丸是唯一按钮形状（规格「The pill shape is non-negotiable」）
      borderRadius: 90,
      borderRadiusSM: 90,
      borderRadiusLG: 90,
      // 按钮标签 = button-md 700（+0.144px 字距见 css 片段）
      fontWeight: 700,
      // over-padded 药丸：14px 28px 在管理台 14px 字号下降为 24px 内距（推导，保留舒适感）
      paddingInline: 24,
      paddingInlineSM: 16,
      // 三态阴影全关（药丸平坦，海拔由卡面 Level 1 影承担）
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
      // 默认钮 = button-outline-aubergine：白底 + 茄紫描边 + 茄紫字（规格 Components）
      defaultBg: '#FFFFFF',
      defaultColor: '#4A154B',
      defaultBorderColor: '#4A154B',
      defaultHoverBg: '#F9F0FF', // hover = canvas-lavender 次级药丸面的回声
      defaultHoverColor: '#4A154B',
      defaultHoverBorderColor: '#4A154B',
    },
    Menu: {
      // 下拉当前项 = lavender 微高亮 + ink 字（次级药丸面语汇）
      itemSelectedBg: '#F9F0FF',
      itemSelectedColor: '#1D1D1D',
    },
    Table: {
      // 白底表头 + ink-mute 标签 + hairline 分隔；行 hover = lavender 微染
      headerBg: '#FFFFFF',
      headerColor: '#696969',
      headerSplitColor: '#E6E6E6',
      rowHoverBg: '#F9F0FF',
      borderColor: '#E6E6E6',
    },
    Tooltip: {
      // 茄紫面 + 白字（surface-aubergine 复用为浮层）
      colorBgSpotlight: '#4A154B',
    },
    Segmented: {
      // 分段控件 = cream 轨 + 白选中块（feature-cream 卡面语汇）
      trackBg: '#F4EDE4',
      itemSelectedBg: '#FFFFFF',
    },
    Select: {
      optionSelectedBg: '#F9F0FF',
    },
    Tag: {
      // hairline tag / 状态 pill = rounded.xs 2px（规格 Radius 表）
      borderRadiusSM: 2,
    },
  },
}
