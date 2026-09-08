import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * lamborghini 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现；lamborghini 规格为「# Design System Inspired by …」简化格式（无令牌表），
 * 色值埋于正文段落，角色按 正文提取（字面 hex，禁 var()）。
 *
 * Lamborghini 设计语言（夜间超跑剧场）：
 * - 真黑画布 #000000（规格「Not dark gray, not near-black — true, uncompromising black」），
 *   Charcoal #202020 = 卡/面板提升面，Dark Iron #181818 = 内嵌/输入凹陷面；
 *   海拔 = 表面变亮（阶梯 #000000 → #181818 → #202020 → #494949），零阴影；
 * - Lamborghini Gold #FFC000 唯一彩强调、仅主 CTA（金底黑字），hover/按压转
 *   Dark Gold #917300（「Hover: bg #917300 — darkens significantly」）；
 * - ghost 按钮 = 透明底白字白边（黑底上白 50% ≈ Ash #7D7D7D 实色），hover 转
 *   Teal Action #1EAEDB（Transparent Ghost）；
 * - 白链 + Link Blue #3860BE 通用 hover；Cyan Pulse #29ABE2 信息高亮兼 focus；
 * - 零圆角（「Default for everything — 0px… non-negotiable」）；
 * - 字重 400 主导（「Weight 400 dominates」→ fontWeightStrong 400，全站无 bold）；
 * - 字体：LamboType → Roboto/'Open Sans'/'Helvetica Neue' 系统近似栈（禁外部字体）。
 * algorithm = darkAlgorithm（暗色算法派生填充/禁用态等中间色，照 spotify 暗色范例）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/lamborghini.css 的同名令牌手工保持同源：
 *   colorPrimary             = --primary         (#FFC000，规格「the only chromatic color… exclusively for primary action buttons」)
 *   colorPrimaryHover/Active = --primary-active  (#917300 Dark Gold，规格 hover「darkens significantly」)
 *   colorInfo                = --info            (#29ABE2 Cyan Pulse「informational accent」)
 *   colorSuccess             = --success         (#34D399，推导：规格无绿，暗底标准语义克制化，照 tesla「浏览器默认→克制化」先例)
 *   colorWarning             = --warning         (#FFC000，推导：品牌金即暖琥珀警示族)
 *   colorError               = --error           (#FF453A，推导：规格无红，暗底标准语义克制化)
 *   colorLink/-Hover         = --link/-hover     (#FFFFFF 白链 → #3860BE Link Blue 通用 hover，)
 *   colorText 四阶            = Pure White/Smoke/Steel/Ash (#FFFFFF/#F5F5F5/#969696/#7D7D7D，Neutrals)
 *   colorBgLayout/Container/Elevated = --canvas/--canvas-card (#000000/#202020)
 *   colorBorder              = Ash #7D7D7D（≈ 规格 ghost 按钮「1px solid #FFFFFF, opacity 0.5」在黑底上的实色）
 *   borderRadius 0           = 「Default for everything — 0px」
 *   fontWeightStrong 400     = 「Weight 400 dominates」
 *   fontFamily               = --font-sans（LamboType 系统近似栈）
 */
export const lamborghiniAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    // 金一族：CTA 基色 #FFC000；hover/按压显著压深到 Dark Gold #917300
    colorPrimary: '#FFC000',
    colorPrimaryHover: '#917300',
    colorPrimaryActive: '#917300',
    // 语义色（无语义色码：info 取 Cyan Pulse；success/error 为暗底标准语义克制化推导）
    colorInfo: '#29ABE2',
    colorSuccess: '#34D399',
    colorWarning: '#FFC000',
    colorError: '#FF453A',
    // 白链 + Link Blue 通用 hover（「used universally for link hover states」）
    colorLink: '#FFFFFF',
    colorLinkHover: '#3860BE',
    colorLinkActive: '#3860BE',
    // 文字阶梯：Pure White / Smoke / Steel / Ash（Neutrals 四级）
    colorText: '#FFFFFF',
    colorTextHeading: '#FFFFFF',
    colorTextSecondary: '#F5F5F5',
    colorTextTertiary: '#969696',
    colorTextQuaternary: '#7D7D7D',
    // 金底黑字（Gold Accent CTA「text #000000」）
    colorTextLightSolid: '#000000',
    // 表面阶梯：画布真黑 / 容器 Charcoal / 浮层 Charcoal（海拔 = 变亮）
    colorBgBase: '#000000',
    colorBgLayout: '#000000',
    colorBgContainer: '#202020',
    colorBgElevated: '#202020',
    colorBgSpotlight: '#202020',
    // 边框：Ash（≈ 黑底上 50% 白的 ghost 边）/ 次级 Graphite / 分隔线 #202020
    colorBorder: '#7D7D7D',
    colorBorderSecondary: '#494949',
    colorSplit: '#202020',
    // focus 环 = Cyan Pulse（「interactive element highlight」）
    controlOutline: 'rgba(41, 171, 226, .45)',
    // 零圆角（「Default for everything — 0px」；徽章 2px 在 Tag 上单独钉）
    borderRadius: 0,
    borderRadiusSM: 0,
    borderRadiusLG: 0,
    // 零阴影：黑画布上阴影无意义，海拔靠表面变亮（Don't）
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // LamboType 系统近似栈（fallback Roboto/Helvetica Neue/Arial；表单语境 Open Sans）
    fontFamily:
      "'LamboType', Roboto, 'Open Sans', 'Helvetica Neue', Arial, 'PingFang SC', 'Microsoft YaHei', sans-serif",
    fontFamilyCode: '"Cascadia Code", Consolas, monospace',
    // 字重 400 主导：全站无 bold（「Weight 400 dominates」）
    fontWeightStrong: 400,
  },
  components: {
    Button: {
      // 零圆角 + 字重 400（按钮「borderRadius 0px / fontWeight 400」）
      borderRadius: 0,
      borderRadiusSM: 0,
      borderRadiusLG: 0,
      fontWeight: 400,
      // 金底黑字的主 CTA
      primaryColor: '#000000',
      // 默认按钮 = Transparent Ghost：#202020 底白字 + 白 50% 边（≈ Ash），hover 转 Teal Action
      defaultBg: '#202020',
      defaultColor: '#FFFFFF',
      defaultBorderColor: '#7D7D7D',
      defaultHoverBg: '#1EAEDB',
      defaultHoverColor: '#FFFFFF',
      defaultHoverBorderColor: '#1EAEDB',
      defaultShadow: 'none',
      primaryShadow: 'none',
      dangerShadow: 'none',
    },
    Checkbox: { colorWhite: '#000000' },
    Steps: { colorWhite: '#000000' },
    Table: {
      // 扁平表头：Dark Iron #181818 + Steel 标签；行 hover = 聚光灯白纱
      headerBg: '#181818',
      headerColor: '#969696',
      headerSplitColor: 'transparent',
      rowHoverBg: 'rgba(255, 255, 255, .05)',
      rowSelectedBg: 'rgba(255, 255, 255, .08)',
      rowSelectedHoverBg: 'rgba(255, 255, 255, .1)',
      fixedHeaderSortActiveBg: '#181818',
      borderColor: '#313131',
    },
    Modal: { contentBg: '#202020', headerBg: '#202020', footerBg: '#202020' },
    Drawer: { colorBgElevated: '#202020' },
    Tooltip: { colorBgSpotlight: '#202020' },
    Popover: { colorBgElevated: '#202020' },
    Select: { colorBgContainer: '#181818', optionSelectedBg: 'rgba(255, 255, 255, .08)' },
    Input: { colorBgContainer: '#181818' },
    InputNumber: { colorBgContainer: '#181818' },
    Form: { labelColor: '#FFFFFF' },
    Segmented: {
      trackBg: '#181818',
      itemSelectedBg: '#202020',
      // 选中项 = Gold Text 高亮标签（「inline text accents and highlighted labels」）
      itemSelectedColor: '#FFCE3E',
    },
    Tag: { borderRadiusSM: 2 }, // 徽章 2px（「Badges — barely perceptible」）
    Pagination: { itemActiveBg: 'transparent' },
  },
}
