import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * spacex 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文（字面 hex，禁 var()）。
 *
 * SpaceX 设计语言（pure black canvas 暗色勘误版）：
 * - 画布纯黑 #000000（canvas-night）；面板 #0a0a0a（canvas-night-soft）+ 1px
 *   #3a3a3f 发丝线（hairline-on-dark）——规格「lifted panel」唯一形态；
 * - 黑白即全部色板：白 #ffffff 是唯一信号 —— colorPrimary = #ffffff，
 *   主按钮 = 白底黑字（colorTextLightSolid #000000）；colorLink = #ffffff
 *   （规格 Link on Dark 原文：白链 + 持久下划线）；
 * - 全屏摄影无法进管理台 → 深度让位给黑白反转与提升阶梯
 *   （黑 #000000 → #0a0a0a → 派生 #101014 / #17171c 浮层）；
 * - 零阴影零渐变（规格 Don't：no drop shadows / no gradient overlays），
 *   分离只靠发丝线与明度阶梯；
 * - ghost 药丸按钮 999px（rounded.pill）+ button-cap 大写字汇
 *   （uppercase / 1.17px tracking 由 themes/css/spacex.css 片段承担）；
 * - 语义三色为派生（规格黑白体系无语义色码，管理台成败/告警需要可辨色，
 *   按 #000000 画布上的可读性推导：#3fb950 / #d29922 / #f2555a）。
 * algorithm = darkAlgorithm（暗色算法派生填充/禁用态等中间色，照 spotify 暗色范例模式）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/spacex.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary        (#ffffff 白即唯一信号)
 *   colorPrimaryHover  = --link-hover     (#f0f0fa on-primary-mute，规格冷白)
 *   colorPrimaryActive = --primary-active (#c8c8cf 派生冷灰，按压回落)
 *   colorInfo          = --info           (#ffffff info = 白信号)
 *   colorSuccess/Warning/Error = --success/--warning/--error（派生暗底语义三色）
 *   colorLink          = --link           (#ffffff 规格 Link on Dark)
 *   colorText 四阶      = ink #ffffff / f0f0fa / c8c8cf / 9a9aa2
 *   colorBgLayout      = --canvas         (#000000 canvas-night)
 *   colorBgContainer   = --canvas-card    (#0a0a0a canvas-night-soft)
 *   colorBgElevated    = 浮层派生阶       (#101014，规格只有两级暗面，按阶梯推导)
 *   colorBorder        = --hairline       (#3a3a3f hairline-on-dark)
 *   colorBorderSecondary/colorSplit = 内部近隐线 #26262b（派生）
 *   fontFamily         = --font-sans（D-DIN → Arial Narrow/Arial/Verdana 官方回退链）
 */
export const spacexAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    // 白 = 唯一交互信号：主按钮白底黑字，hover 冷白（on-primary-mute），按压回落冷灰
    colorPrimary: '#ffffff',
    colorPrimaryHover: '#f0f0fa',
    colorPrimaryActive: '#c8c8cf',
    // info / link = 白（规格：Link on Dark 白链；黑白体系无第二信号色）
    colorInfo: '#ffffff',
    colorLink: '#ffffff',
    colorLinkHover: '#f0f0fa',
    colorLinkActive: '#c8c8cf',
    // 语义三色（派生：黑白体系无语义色码，取黑画布上可读的克制变体）
    colorSuccess: '#3fb950',
    colorWarning: '#d29922',
    colorError: '#f2555a',
    // 文字阶梯坍缩为白 / 冷白 / 冷灰 / 中灰四级（规格正文只给 #ffffff 与 #f0f0fa，
    // 下两级按 hairline-on-dark #3a3a3f 的冷调推导）
    colorText: '#ffffff',
    colorTextHeading: '#ffffff',
    colorTextSecondary: '#f0f0fa',
    colorTextTertiary: '#c8c8cf',
    colorTextQuaternary: '#9a9aa2',
    // 签名：白底黑字（ghost 药丸的实底版；login CTA 白底黑字同源）
    colorTextLightSolid: '#000000',
    // 表面阶梯：画布纯黑 / 容器微升 #0a0a0a / 浮层派生阶
    colorBgBase: '#000000',
    colorBgLayout: '#000000',
    colorBgContainer: '#0a0a0a',
    colorBgElevated: '#101014',
    colorBgSpotlight: '#17171c',
    // 边线双阶：hairline-on-dark #3a3a3f（chrome 线）/ 内部近隐 #26262b（派生）
    colorBorder: '#3a3a3f',
    colorBorderSecondary: '#26262b',
    colorSplit: '#26262b',
    controlOutline: 'rgba(255, 255, 255, 0.12)',
    // 圆角刻度：小件 4（rounded.xs）/ 大面 16（rounded.md）；药丸在 Button/Tag 单独钉 999
    borderRadius: 4,
    borderRadiusSM: 4,
    borderRadiusLG: 16,
    // 零阴影体系（规格 Don't：暗画布禁投影/渐变）
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // D-DIN 官方回退链（Arial Narrow → Arial → Verdana；禁外部字体加载）
    fontFamily:
      "'D-DIN', 'Arial Narrow', Arial, Verdana, 'PingFang SC', 'Microsoft YaHei', sans-serif",
    fontFamilyCode: '"Cascadia Code", Consolas, monospace',
    // D-DIN-Bold 展示字 = 700（规格字重只有 400/700 两档）
    fontWeightStrong: 700,
  },
  components: {
    Button: {
      // ghost 药丸是唯一 CTA 形态：全部按钮 999px（rounded.pill 32 对按钮即全圆）
      borderRadius: 999,
      borderRadiusSM: 999,
      borderRadiusLG: 999,
      // button-cap：13px/700/1.17px 大写（字重在此、大写与字距在 css 片段）
      fontWeight: 700,
      // 主按钮 = 白底黑字（白即主色）
      primaryColor: '#000000',
      // 默认按钮 = ghost 描边：透明底 + 白字白线；hover 反转为白底黑字
      // （黑白体系里唯一的 hover 语言，规格 hover 态未记载、按黑白反转推导）
      defaultBg: 'transparent',
      defaultColor: '#ffffff',
      defaultBorderColor: '#ffffff',
      defaultHoverBg: '#ffffff',
      defaultHoverColor: '#000000',
      defaultHoverBorderColor: '#ffffff',
      // 零阴影（海拔只靠明度阶梯）
      defaultShadow: 'none',
      primaryShadow: 'none',
      dangerShadow: 'none',
    },
    Checkbox: { colorWhite: '#000000' },
    Steps: { colorWhite: '#000000' },
    Table: {
      // 扁平表头：与容器同底 #0a0a0a + 中灰标签（大写/字距由 css 片段承担）
      headerBg: '#0a0a0a',
      headerColor: '#9a9aa2',
      headerSplitColor: 'transparent',
      rowHoverBg: '#101014',
      rowSelectedBg: '#101014',
      rowSelectedHoverBg: '#16161c',
      fixedHeaderSortActiveBg: '#101014',
      borderColor: '#26262b',
    },
    Modal: { contentBg: '#0a0a0a', headerBg: '#0a0a0a', footerBg: '#0a0a0a' },
    Drawer: { colorBgElevated: '#0a0a0a' },
    Tooltip: { colorBgSpotlight: '#17171c' },
    Popover: { colorBgElevated: '#101014' },
    // 输入族 = 纯黑内嵌（画布同底）+ hairline 线；focus 线转白
    Select: { colorBgContainer: '#000000', optionSelectedBg: '#1c1c22' },
    Input: {
      colorBgContainer: '#000000',
      hoverBorderColor: '#9a9aa2',
      activeBorderColor: '#ffffff',
      activeShadow: 'none',
    },
    InputNumber: { colorBgContainer: '#000000' },
    Form: { labelColor: '#ffffff' },
    Segmented: { trackBg: '#0a0a0a', itemSelectedBg: '#1c1c22' },
    Tag: { borderRadiusSM: 999 },
    Pagination: { itemActiveBg: 'transparent' },
  },
}
