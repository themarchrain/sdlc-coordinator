import type { ThemeConfig } from 'antd'

/**
 * tesla 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Tesla 设计语言 = 激进减法：白画布 + 近黑墨 + 唯一交互色 Electric Blue #3E6AE1，
 * 近零 UI 装饰（两级阴影全关、无渐变、发丝线分隔、4px「barely-rounded」圆角），
 * 字重只有 400/500（fontWeightStrong 压到 500，全站无 bold），
 * 交互节奏统一 0.33s（motionDuration 三档全部钉住）。
 * algorithm 未指定 = antd 默认算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/tesla.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#3E6AE1，规格「sole interactive color signal」)
 *   colorPrimaryHover/Active = 蓝压深两阶        (#3560C8 / #2C52AE，规格「hover: subtle darkening」)
 *   colorInfo/colorLink     = --info/--link     (#3E6AE1，蓝即唯一交互信号)
 *   colorInfoBg/colorInfoBorder = --info-soft/--hairline-strong
 *                                                (#F0F3FC 蓝薄纱 + #D0D1D2 发丝线，钉住
 *                                                 「软面 + 发丝线」的克制 info 形态)
 *   colorSuccess/Warning/Error = 语义三色（派生：规格「语义态走浏览器默认」→ 标准语义色克制化）
 *   colorText 四阶           = Carbon/Graphite/Pewter/Silver Fog
 *                             (#171A20 / #393C41 / #5C5E62 / #8E8E8E，规格 文字四级原文)
 *   colorBgLayout/Container/Elevated = --canvas/--canvas-card (#FFFFFF，白画布)
 *   colorBorder 双阶 + colorSplit = Pale Silver / Cloud Gray (#D0D1D2 / #EEEEEE，规格 边线双色)
 *   borderRadius 4          = 规格 按钮「barely-rounded rectangles (4px)」
 *   borderRadiusLG 12       = 规格 大面卡片「~12px restrained rounding」
 *   fontFamily              = --font-sans（Universal Sans 的系统栈近似，禁外部字体）
 */
export const teslaAntdTheme: ThemeConfig = {
  token: {
    // Electric Blue：唯一交互色（Primary / Do）
    colorPrimary: '#3E6AE1',
    colorPrimaryHover: '#3560C8',
    colorPrimaryActive: '#2C52AE',
    // info / link 与 primary 同蓝：蓝是唯一交互信号
    colorInfo: '#3E6AE1',
    colorLink: '#3E6AE1',
    colorLinkHover: '#3560C8',
    // 钉住 info Alert 为「蓝薄纱软面 + Pale Silver 发丝线」（先例：cursor/antd.ts 40-46）
    colorInfoBg: '#F0F3FC',
    colorInfoBorder: '#D0D1D2',
    // 语义三色（「语义态跟随浏览器默认」→ 取标准语义的克制饱和度）
    colorSuccess: '#1F7A3D',
    colorWarning: '#A16207',
    colorError: '#C8102E',
    // 文字阶梯：Carbon Dark / Graphite / Pewter / Silver Fog（Neutrals 四级原文）
    colorText: '#171A20',
    colorTextSecondary: '#393C41',
    colorTextTertiary: '#5C5E62',
    colorTextQuaternary: '#8E8E8E',
    // 白画布：页面、容器、浮层全白，靠发丝线与留白分隔
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：Pale Silver（控件边）/ Cloud Gray（分隔线）
    colorBorder: '#D0D1D2',
    colorBorderSecondary: '#EEEEEE',
    colorSplit: '#EEEEEE',
    // 圆角：4px 微圆角为默认；12px 只给大面（Border Radius Scale）
    borderRadius: 4,
    borderRadiusSM: 4,
    borderRadiusLG: 12,
    // 字重克制：只有 400/500，无 bold（「Weight restraint」）
    fontWeightStrong: 500,
    // 近零 UI：两级阴影全关（「approach to elevation is none」）
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // 0.33s 是全站唯一交互时长（「universal timing」）
    motionDurationFast: '0.33s',
    motionDurationMid: '0.33s',
    motionDurationSlow: '0.33s',
    // Universal Sans 的系统/web-safe 栈近似（规格 字体为定制族，禁外部加载）
    fontFamily:
      'system-ui, -apple-system, "Segoe UI", Arial, Helvetica, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
  },
  components: {
    Button: {
      // 按钮字重 500（规格 Button Label 14px/500）；三态阴影全关
      fontWeight: 500,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = Light Ash 微高亮 + Carbon 墨（规格 Nav Button「subtle background highlight」）
      itemSelectedBg: '#F4F4F4',
      itemSelectedColor: '#171A20',
    },
    Table: {
      // 白底表头 + Pewter 标签 + 透明分隔缝；行 hover = Light Ash
      headerBg: '#FFFFFF',
      headerColor: '#5C5E62',
      headerSplitColor: 'transparent',
      rowHoverBg: '#F4F4F4',
      borderColor: '#EEEEEE',
    },
    Tooltip: {
      colorBgSpotlight: '#171A20',
    },
  },
}
