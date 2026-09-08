import type { ThemeConfig } from 'antd'

/**
 * starbucks 主题的 antd ThemeConfig —— 自设计语言 正文派生
 * （规格为叙述体、无令牌表，色值全部自正文角色句提取）。
 *
 * Starbucks 设计语言 = 暖奶油旗舰零售：Neutral Warm #F2F0EB 画布 + 白卡，
 * 四档绿各司其职（Primary 正文）——Starbucks Green #006241 用于 h1/区块标题，
 * Green Accent #00754A 是主填充 CTA，House Green #1E3932 承担 bookend 深带/暗面，
 * Green Uplift #2B5148 只作装饰；金 #CBA258 仅 Rewards 仪式，不进交互位。
 * 50px 全药丸按钮（「Radius 50px (full pill)」）+ scale(0.95) 按压（形态见 css 片段）、
 * 12px 圆角卡、耳语双层卡影（字面值）、SoDoSans 紧排 -0.01em、字重带 400–600（无 700）。
 * algorithm 未指定 = antd 默认算法（亮色）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/starbucks.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#00754A Green Accent，Primary Filled CTA)
 *   colorPrimaryHover       = #008248           (推导：Consent Agree 变体绿 rgb(0,130,72)——
 *                                                规格正文中唯一「更亮」的绿，作 hover 提亮档)
 *   colorPrimaryActive      = --primary-active  (#006241 Starbucks Green，按压压深回历史绿)
 *   colorInfo/colorLink     = --info/--link     (#006241/#00754A 标题绿与链接绿分轨)
 *   colorInfoBg/colorInfoBorder = --info-soft/--hairline-strong
 *                                                (#D4E9E2 Green Light 淡薄荷洗 + #D6DBDE Input Border)
 *   colorSuccess/Warning/Error = 语义三色（success 复用 CTA 绿；warning = Yellow #fbbc05
 *                                深化到文字级；error = Red #c82014 规格字面值）
 *   colorText 四阶           = Text Black .87 / Text Black Soft .58 / Rewards Green #33433D / 黑 50%
 *   colorBgLayout            = --canvas         (#F2F0EB Neutral Warm 主画布)
 *   colorBgContainer/Elevated = --canvas-card   (#FFFFFF 白卡/浮层)
 *   colorBorder 双阶 + colorSplit = Input Border #D6DBDE / 表格发丝线 #E7E7E7
 *   borderRadius 4           = 规格 Input radius 4px（输入/小件）
 *   Button borderRadius 50   = 规格「50px full pill」全药丸按钮
 *   borderRadiusLG 12        = 规格 --cardBorderRadius 12px（卡/模态）
 *   boxShadow                = 规格 --cardBoxShadow 耳语双层卡影字面值
 *   boxShadowSecondary       = 规格 Global Nav 三层软抬升阴影字面值
 *   fontWeightStrong 600     = 规格 H1 600（字重带封顶 600，全站无 700）
 *   fontFamily               = --font-sans（SoDoSans 专有 → 规格替代指引 Inter 等系统栈近似）
 */
export const starbucksAntdTheme: ThemeConfig = {
  token: {
    // Green Accent：主填充 CTA 色（Buttons「Background #00754A」）
    colorPrimary: '#00754A',
    colorPrimaryHover: '#008248', // 推导：Consent Agree rgb(0,130,72)，规格内更亮的绿作提亮档
    colorPrimaryActive: '#006241', // Starbucks Green：按压压深（「historic brand green」）
    // info = 标题绿 / link = CTA 绿：两档绿按正文角色分轨
    colorInfo: '#006241',
    colorLink: '#00754A',
    colorLinkHover: '#006241',
    // info Alert = Green Light 淡薄荷洗 + Input Border（「form-valid-state tints」同族）
    colorInfoBg: '#D4E9E2',
    colorInfoBorder: '#D6DBDE',
    // 语义三色：success 复用 CTA 绿；warning 深化；error 规格字面值（Semantic & Accent）
    colorSuccess: '#00754A',
    colorWarning: '#97690F',
    colorError: '#C82014',
    // 文字阶梯：Text Black .87 / Text Black Soft .58 / Rewards Green / 黑 50%（黑 alpha 阶）
    colorText: '#1F1F1E', /* rgba(0,0,0,.87) 在奶油画布上的合成 hex（colorText 契约禁 rgba，其余文字档保留原值） */
    colorTextSecondary: 'rgba(0, 0, 0, 0.58)',
    colorTextTertiary: '#33433D',
    colorTextQuaternary: 'rgba(0, 0, 0, 0.5)',
    // 三面：奶油画布 + 白容器 + 白浮层
    colorBgLayout: '#F2F0EB',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：Input Border #D6DBDE（控件边）/ #E7E7E7（营养表格发丝线字面值）
    colorBorder: '#D6DBDE',
    colorBorderSecondary: '#E7E7E7',
    colorSplit: '#E7E7E7',
    // 焦点环 = Green Accent 薄纱（推导，与 --focus-ring 同源）
    controlOutline: 'rgba(0, 117, 74, 0.12)',
    // 圆角：输入 4px（Add-in Select）/ 大面卡 12px；按钮药丸由 Button 组件钉住
    borderRadius: 4,
    borderRadiusSM: 4,
    borderRadiusLG: 12,
    // 字重带 400–600：strong 封顶 600（「Weight shifts carry hierarchy」）
    fontWeightStrong: 600,
    // 耳语双层卡影（Card 字面值）+ 全局导航三层软抬升（Global Nav 字面值）
    boxShadow: '0 0 0.5px 0 rgba(0, 0, 0, 0.14), 0 1px 1px 0 rgba(0, 0, 0, 0.24)',
    boxShadowSecondary:
      '0 1px 3px rgba(0, 0, 0, 0.1), 0 2px 2px rgba(0, 0, 0, 0.06), 0 0 2px rgba(0, 0, 0, 0.07)',
    // 交互节奏：按钮 all .2s ease / 展开件 300ms（Transition、Expander）
    motionDurationFast: '0.2s',
    motionDurationMid: '0.3s',
    motionDurationSlow: '0.3s',
    // SoDoSans 专有族（禁外部加载）→ 规格替代指引 Inter/Nunito Sans 的系统栈近似
    fontFamily:
      "'SoDo Sans', SoDoSans, Inter, 'Nunito Sans', 'Helvetica Neue', Helvetica, Arial, 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
  },
  components: {
    Button: {
      // 50px 全药丸按钮（「Radius 50px (full pill)」），标签 600
      borderRadius: 50,
      borderRadiusSM: 50,
      borderRadiusLG: 50,
      fontWeight: 600,
      // 按钮平坦：海拔只留给卡影与 Frap（形态见 css 片段）
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
      // 默认钮 = Primary Outlined：透明底 + Green Accent 字与边
      defaultBg: 'transparent',
      defaultColor: '#00754A',
      defaultBorderColor: '#00754A',
      defaultHoverBg: '#D4E9E2', // hover = Green Light 淡薄荷洗（「light green utility surfaces」）
      defaultHoverColor: '#006241',
      defaultHoverBorderColor: '#006241',
    },
    Menu: {
      // 下拉当前项 = Green Light 淡洗底 + Text Black（「Neutral Cool 下拉/utility 容器」语汇）
      itemSelectedBg: '#D4E9E2',
      itemSelectedColor: 'rgba(0, 0, 0, 0.87)',
    },
    Table: {
      // Neutral Cool #F9F9F9 表头（「quiet utility containers」）+ 发丝线分隔；行 hover 同面
      headerBg: '#F9F9F9',
      headerColor: 'rgba(0, 0, 0, 0.58)',
      headerSplitColor: '#E7E7E7',
      rowHoverBg: '#F9F9F9',
      borderColor: '#E7E7E7',
    },
    Tooltip: {
      // House Green 反转面（「deep near-black brand green」的暗面角色）
      colorBgSpotlight: '#1E3932',
    },
    Tag: {
      // 迷你徽标 = 药丸（Rewards Cost Pill 的 50px 语汇）
      borderRadiusSM: 999,
    },
  },
}
