import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * raycast 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文（字面 hex，禁 var()）。
 *
 * Raycast 设计语言（连续暗模式，营销页 = 放大的产品 chrome）：
 * - 面阶梯：canvas #07080a → surface #0d0d0d → surface-elevated #101111 → surface-card #121212，
 *   深一档 = 近一步；无投影，海拔全靠面色阶梯；
 * - 主交互 = 白色 CTA：colorPrimary #ffffff / 按压 #e8e8e8（primary-pressed）/
 *   colorTextLightSolid #000000（on-primary 纯黑，全站唯一黑字位）；
 * - 发丝线 #242728 承担一切卡缘；强调线 = 白 16%（hairline-strong）；
 * - 语义四色（HN 黄 / Slack 红 / Mac 绿 / info 蓝）只作状态信号：
 *   success #59d499 / warning #ffc533 / error #ff6161 / info #57c1ff；
 * - 圆角刻度 4/6/8/10/16：默认 8（rounded.md 按钮/输入）、SM 6（rounded.sm 行）、LG 10（rounded.lg 卡）；
 * - 字重纪律：标题/按钮 500，display 数值 600（fontWeightStrong 压 500）；
 * - 输入 focus = 发丝线提亮为白 16%（「subtle brightening rather than a colored ring」）。
 * algorithm = darkAlgorithm（暗色算法派生填充/禁用态等中间色，照 spotify 暗色范例模式）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/raycast.css 的同名令牌手工保持同源：
 *   colorPrimary        = --primary         (#ffffff 白 CTA)
 *   colorPrimaryHover   = #ffffff           (规格无 hover，钉住不漂移)
 *   colorPrimaryActive  = --primary-active  (#e8e8e8 primary-pressed)
 *   colorInfo           = --info            (#57c1ff accent-blue)
 *   colorSuccess        = --success         (#59d499 accent-green)
 *   colorWarning        = --warning         (#ffc533 accent-yellow)
 *   colorError          = --error           (#ff6161 accent-red)
 *   colorLink           = --link            (#ffffff，规格 inline link = on-dark 白)
 *   colorText 四阶       = ink #f4f4f6 / body #cdcdcd / mute #9c9c9d / ash #6a6b6c
 *   colorTextLightSolid = --on-primary      (#000000 绿底黑字的白底版：白底黑字)
 *   colorBgLayout       = --canvas          (#07080a)
 *   colorBgContainer    = --canvas-card     (#0d0d0d surface)
 *   colorBgElevated     = --tooltip-bg      (#18191a button-fg 深卡阶，浮层面)
 *   colorBorder         = --hairline        (#242728) / Secondary 白 8% / colorSplit #242728
 *   boxShadow 双键      = none              (规格「no drop shadows in the system」)
 *   fontFamily          = --font-sans（Inter + ss03，ss03 开关在 raycast.css 片段）
 */
export const raycastAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    // 白 CTA 一族：base #ffffff → hover 钉住（规格只定义 pressed）→ 按压 #e8e8e8
    colorPrimary: '#ffffff',
    colorPrimaryHover: '#ffffff',
    colorPrimaryActive: '#e8e8e8',
    // 语义四色 = 规格点缀组的官方语义位
    colorInfo: '#57c1ff',
    colorSuccess: '#59d499',
    colorWarning: '#ffc533',
    colorError: '#ff6161',
    // 链接 = 全白（规格 link-inline：「full-white rather than a tinted accent」）
    colorLink: '#ffffff',
    colorLinkHover: '#e8e8e8',
    colorLinkActive: '#e8e8e8',
    // 文字阶梯坍缩为规格六级：ink / body / mute / ash（charcoal 归入 CSS 侧）
    colorText: '#f4f4f6',
    colorTextHeading: '#f4f4f6',
    colorTextSecondary: '#cdcdcd',
    colorTextTertiary: '#9c9c9d',
    colorTextQuaternary: '#6a6b6c',
    // 签名：白底纯黑字（on-primary #000000 是全站唯一黑字位）
    colorTextLightSolid: '#000000',
    // 表面阶梯：画布 #07080a / 容器 #0d0d0d / 浮层 #18191a（button-fg 深卡阶）
    colorBgBase: '#07080a',
    colorBgLayout: '#07080a',
    colorBgContainer: '#0d0d0d',
    colorBgElevated: '#18191a',
    colorBgSpotlight: '#18191a',
    // info Alert = accent-blue-soft 薄纱 + 白 16% 强线（badge-info-soft 形态）
    colorInfoBg: 'rgba(87, 193, 255, 0.15)',
    colorInfoBorder: 'rgba(255, 255, 255, 0.16)',
    // 发丝线双轨：#242728 卡缘 / 白 8% 内部弱线
    colorBorder: '#242728',
    colorBorderSecondary: 'rgba(255, 255, 255, 0.08)',
    colorSplit: '#242728',
    // focus = 发丝线提亮（规格输入聚焦：hairline-strong 而非彩色环）
    controlOutline: 'rgba(255, 255, 255, 0.16)',
    // 圆角刻度：默认 8（按钮/输入 rounded.md）/ SM 6（行 rounded.sm）/ LG 10（卡 rounded.lg）
    borderRadius: 8,
    borderRadiusSM: 6,
    borderRadiusLG: 10,
    // 无投影体系（规格 Elevation：「no drop-shadow elevation at all」）
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // Inter + ss03：ss03 特征开关由 raycast.css 专属片段挂到作用域根
    fontFamily:
      'Inter, system-ui, -apple-system, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
    fontFamilyCode: '"JetBrains Mono", "Cascadia Code", Consolas, monospace',
    // 字重纪律：标题/强调 500（规格排版表 heading 一律 500）
    fontWeightStrong: 500,
  },
  components: {
    Button: {
      // 按钮字重 500（规格 button-md 14px/500）；三态阴影全关
      fontWeight: 500,
      primaryColor: '#000000',
      // 默认按钮 = button-tertiary：surface-elevated 底白字，hover 抬到 surface-card 一档
      defaultBg: '#101111',
      defaultColor: '#ffffff',
      defaultBorderColor: '#101111',
      defaultHoverBg: '#121212',
      defaultHoverColor: '#ffffff',
      defaultHoverBorderColor: '#121212',
      defaultShadow: 'none',
      primaryShadow: 'none',
      dangerShadow: 'none',
    },
    // 白主色上的勾选/步骤笔画必须是黑（colorWhite = 主色底上的图形色）
    Checkbox: { colorWhite: '#000000' },
    Steps: { colorWhite: '#000000' },
    Table: {
      // 扁平表头：surface 底 + mute 银标签；行 hover = surface-card（command-palette-row-active）
      headerBg: '#0d0d0d',
      headerColor: '#9c9c9d',
      // 对比表行分隔 = hairline-strong 白 16%（规格 Elevation Level 2）
      headerSplitColor: 'rgba(255, 255, 255, 0.16)',
      rowHoverBg: '#121212',
      rowSelectedBg: '#121212',
      rowSelectedHoverBg: '#18191a',
      fixedHeaderSortActiveBg: '#121212',
      borderColor: '#242728',
    },
    Modal: { contentBg: '#0d0d0d', headerBg: '#0d0d0d', footerBg: '#0d0d0d' },
    Drawer: { colorBgElevated: '#0d0d0d' },
    Tooltip: { colorBgSpotlight: '#18191a' },
    Popover: { colorBgElevated: '#0d0d0d' },
    Select: { colorBgContainer: '#101111', optionSelectedBg: '#121212' },
    Input: {
      // 输入 = surface-elevated 填充；聚焦边 = 白 16% 提亮（规格 text-input-focused）
      colorBgContainer: '#101111',
      activeBorderColor: 'rgba(255, 255, 255, 0.16)',
    },
    InputNumber: { colorBgContainer: '#101111' },
    Form: { labelColor: '#f4f4f6' },
    // pill-tab：透明轨道 + surface-elevated 激活片（「chip lifts by one surface notch」）
    Segmented: { trackBg: 'transparent', itemSelectedBg: '#101111' },
    Tag: { borderRadiusSM: 4 },
    Pagination: { itemActiveBg: 'transparent' },
  },
}
