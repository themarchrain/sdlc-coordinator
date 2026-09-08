import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * hashicorp 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文（字面 hex，禁 var()）。
 *
 * HashiCorp 设计语言（企业基建黑底 + 产品色身份系统）：
 * - 画布 #000000 纯黑 + charcoal 提升阶梯 surface-1 #15181e → surface-2 #1f232b
 *   → surface-3 #3b3d45；海拔靠 surface lift，暗底无投影；
 * - 签名一：主 CTA = 白底黑字（inverse-canvas/inverse-ink）→ colorPrimary #ffffff
 *   + colorTextLightSolid #000000；hover/active 为推导的微降白阶（规格按压同白面）；
 * - 签名二：chrome 单色纪律 —— 产品色（Terraform 紫 #7b42bc / Vault 黄 #ffcf25 /
 *   Consul 红 #e62b1e / Waypoint 青 #14c6cb…）是身份令牌不进通用 chrome；
 *   功能蓝 accent-blue #2b89ff 承担链接与焦点（visited 紫 #a737ff 规格留档，
 *   antd 无 visited 令牌，不落地）；
 * - 文字三级：白 #ffffff / ink-muted #b2b6bd / ink-subtle #656a76（规格禁加第四灰）；
 * - 半透明发丝线 rgba(178,182,189,.1) 是主导边缘（「borders are felt more than seen」）；
 * - 语义：success #00ca8e / warning #ffcf25 / error #e62b1e（与 Nomad/Vault/Consul
 *   产品色同值 —— 规格明文的复用）；
 * - 圆角：CTA/输入 8px（rounded.md，「engineered, not consumer」）、卡 12px（lg）、
 *   chip 药丸（product-pill）；
 * - 字体：hashicorpSans 单族（Inter 为规格书面替代）；display 700 / body 500。
 * algorithm = darkAlgorithm（暗色算法派生填充/禁用态等中间色，照 spotify 暗色范例模式）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/hashicorp.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary        (#ffffff inverse-canvas；白底黑字 CTA)
 *   colorPrimaryHover/Active = 推导微降白阶 (#f0f2f5 / #e0e3e8)
 *   colorInfo          = --info           (#2b89ff accent-blue)
 *   colorInfoBg/colorInfoBorder = --info-soft/--login-notice-border 同源淡染
 *   colorSuccess       = --success        (#00ca8e)
 *   colorWarning       = --warning        (#ffcf25)
 *   colorError         = --error          (#e62b1e)
 *   colorLink          = --link           (#2b89ff；hover #5aa7ff 推导提亮)
 *   colorText 四阶      = 白 / #b2b6bd / #656a76 / #656a76（规格 Text 三级）
 *   colorBgLayout      = --canvas         (#000000)
 *   colorBgContainer   = --canvas-card    (#15181e surface-1)
 *   colorBgElevated    = --tooltip-bg     (#1f232b surface-2)
 *   colorBorder        = hairline         (#3b3d45) / secondary #252830
 *   borderRadius 8     = 规格 rounded.md（CTA/输入/列表项）；LG 12 = 卡（lg）
 *   boxShadow 全关     = 规格「No drop shadows on dark」
 *   fontFamily         = --font-sans（hashicorpSans → Inter 替代栈）
 */
export const hashicorpAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    // 主 CTA = 白底黑字（规格 button-primary：inverse-canvas 底 + inverse-ink 字）
    colorPrimary: '#ffffff',
    colorPrimaryHover: '#f0f2f5',
    colorPrimaryActive: '#e0e3e8',
    // 功能蓝一族：info/链接 = accent-blue（规格超链接色）；Alert 钉淡染
    colorInfo: '#2b89ff',
    colorInfoBg: 'rgba(43, 137, 255, .14)',
    colorInfoBorder: 'rgba(43, 137, 255, .35)',
    // 语义三色（规格明文，与 Nomad/Vault/Consul 产品色同值）
    colorSuccess: '#00ca8e',
    colorWarning: '#ffcf25',
    colorError: '#e62b1e',
    // 链接蓝 + hover 推导提亮（纯黑底上保持可分辨）
    colorLink: '#2b89ff',
    colorLinkHover: '#5aa7ff',
    colorLinkActive: '#5aa7ff',
    // 文字三级坍缩：白 / ink-muted / ink-subtle（规格禁加第四灰）
    colorText: '#ffffff',
    colorTextHeading: '#ffffff',
    colorTextSecondary: '#b2b6bd',
    colorTextTertiary: '#656a76',
    colorTextQuaternary: '#656a76',
    // 白底上的文字一律近黑（inverse-ink）
    colorTextLightSolid: '#000000',
    // 表面阶梯：画布纯黑 / 容器 surface-1 / 浮层 surface-2
    colorBgBase: '#000000',
    colorBgLayout: '#000000',
    colorBgContainer: '#15181e',
    colorBgElevated: '#1f232b',
    colorBgSpotlight: '#1f232b',
    // 边线双阶：hairline #3b3d45 / hairline-soft #252830（分隔）
    colorBorder: '#3b3d45',
    colorBorderSecondary: '#252830',
    colorSplit: '#252830',
    // 焦点 = accent-blue 1px outline（规格 text-input-focused）
    controlOutline: 'rgba(43, 137, 255, .35)',
    // 圆角：CTA/输入 8px（rounded.md）；卡 12px（rounded.lg）
    borderRadius: 8,
    borderRadiusSM: 6,
    borderRadiusLG: 12,
    // 规格「No drop shadows on dark」：海拔全靠 surface lift
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // hashicorpSans 为专有字体；规格书面替代 = Inter 500/600/700（系统/web-safe 近似栈）
    fontFamily:
      "'hashicorpSans', Inter, -apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, 'PingFang SC', 'Microsoft YaHei', sans-serif",
    fontFamilyCode: '"Cascadia Code", Consolas, monospace',
    // 强调 600（规格 weight 阶：500 body / 600 emphasis / 700 display；700 由 css 片段承担）
    fontWeightStrong: 600,
  },
  components: {
    Button: {
      // CTA 8px（rounded.md）；字重 600（typography.button）
      fontWeight: 600,
      primaryColor: '#000000',
      // 默认按钮 = button-secondary：surface-2 底白字，hover 提到 surface-3
      defaultBg: '#1f232b',
      defaultColor: '#ffffff',
      defaultBorderColor: '#1f232b',
      defaultHoverBg: '#3b3d45',
      defaultHoverColor: '#ffffff',
      defaultHoverBorderColor: '#3b3d45',
      // 暗底无投影（规格）
      defaultShadow: 'none',
      primaryShadow: 'none',
      dangerShadow: 'none',
    },
    Checkbox: { colorWhite: '#000000' },
    Steps: { colorWhite: '#000000' },
    Switch: { colorWhite: '#000000' },
    Table: {
      // 扁平表头：surface-1 同底 + muted 字；行分隔 hairline-soft；hover = 半透明灰纱
      headerBg: '#15181e',
      headerColor: '#b2b6bd',
      headerSplitColor: 'transparent',
      rowHoverBg: 'rgba(178, 182, 189, .06)',
      rowSelectedBg: 'rgba(178, 182, 189, .1)',
      rowSelectedHoverBg: 'rgba(178, 182, 189, .14)',
      fixedHeaderSortActiveBg: '#1f232b',
      borderColor: '#252830',
    },
    Modal: { contentBg: '#1f232b', headerBg: '#1f232b', footerBg: '#1f232b' },
    Drawer: { colorBgElevated: '#15181e' },
    Tooltip: { colorBgSpotlight: '#1f232b' },
    Popover: { colorBgElevated: '#1f232b' },
    // 输入 = 规格 text-input：surface-1 底（卡面上靠 hairline 边界成井）
    Select: { colorBgContainer: '#15181e', optionSelectedBg: '#1f232b' },
    Input: { colorBgContainer: '#15181e' },
    InputNumber: { colorBgContainer: '#15181e' },
    Form: { labelColor: '#ffffff' },
    // Segmented 轨道内沉半阶（推导：canvas 与 surface-1 之间），选中件落 surface-2
    Segmented: { trackBg: '#0a0c10', itemSelectedBg: '#1f232b' },
    // product-pill = 药丸 chip（规格 rounded.pill）
    Tag: { borderRadiusSM: 999 },
    Pagination: { itemActiveBg: 'transparent' },
  },
}
