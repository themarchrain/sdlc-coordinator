import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * bmw-m 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * BMW M 设计语言（暗色，纯黑画布 #000000）：
 * - 主操作 = 白（规格 colors.primary #ffffff）：矩形白 CTA 黑字（on-primary #000000），
 *   描边按钮 = 透明底 + 白 1px 描边 + 白字（规格 button-primary-outline）；
 * - M 三色条纹（#0066b1/#1c69d4/#e22718）只是品牌身份记号，绝不进 CTA / 主色槽位
 *   （规格「The tricolor is exclusively a brand-identity marker」）；
 * - 画布阶：canvas #000000 / soft #0d0d0d / card #1a1a1a / elevated #262626；
 * - 文字阶：ink #ffffff / body #bbbbbb / body-strong #e6e6e6 / mute #7e7e7e；
 * - 发丝线双阶 #3c3c3c（分隔）/ #262626（= surface-elevated，一阶抬升边）；
 * - 圆角一律 0（规格「The rectangular silhouette IS the brand」，圆角只给圆形图标钮，
 *   后者本仓骨架不涉及）；无投影（规格「no drop shadows」）；
 * - 语义色用规格原生值：success #0fa336 / warning #f4b400 / error 派生自 M 红 #e22718
 *   （规格「Don't introduce a brand color outside the M tricolor and bmw-blue」→ 红即 M 红）；
 * - info = bmw-blue #1c69d4 提亮为 #5e97e8（纯黑底上 9-14px 文字的可读性推导），
 *   colorInfoBg/Border 钉成暗软面 + 暗发丝线（暗色版 cursor 先例）；
 * - 字体 BMW Type Next Latin → 系统/web-safe 栈（'Helvetica Neue', Helvetica, Arial），禁外部字体。
 * algorithm = darkAlgorithm（暗色算法派生填充/禁用态等中间色）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/bmw-m.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary        (#ffffff)
 *   colorPrimaryHover  = 白压灰一阶       (#f0f0f0，推导：白 CTA hover 微暗)
 *   colorPrimaryActive = --primary-active (#e6e6e6 = 规格 body-strong)
 *   colorInfo          = --info           (#5e97e8，bmw-blue 提亮)
 *   colorInfoBg        = --info-soft 容器 (#0d0d0d = surface-soft)
 *   colorInfoBorder    = --hairline       (#3c3c3c)
 *   colorSuccess       = --success        (#0fa336)
 *   colorWarning       = --warning        (#f4b400)
 *   colorError         = --error          (#e22718 = M red)
 *   colorText          = --body           (#bbbbbb)
 *   colorTextHeading   = --ink            (#ffffff)
 *   colorBgLayout      = --canvas         (#000000)
 *   colorBgContainer   = --canvas-card    (#1a1a1a)
 *   colorBgElevated    = surface-elevated (#262626)
 *   colorBorder        = --hairline       (#3c3c3c)
 *   colorBorderSecondary / colorSplit = --hairline-soft (#262626)
 *   fontFamily         = --font-sans；fontFamilyCode = --font-mono
 */
export const bmwMAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    // 白是系统主操作色：白底黑字的矩形 CTA
    colorPrimary: '#ffffff',
    colorPrimaryHover: '#f0f0f0',
    colorPrimaryActive: '#e6e6e6',
    // info = bmw-blue 提亮（暗底可读性）；info Alert 钉成「暗软面 + 暗发丝线」
    colorInfo: '#5e97e8',
    colorInfoBg: '#0d0d0d',
    colorInfoBorder: '#3c3c3c',
    // 语义三色：success/warning 用规格原生值；error = M red（品牌唯一红）
    colorSuccess: '#0fa336',
    colorWarning: '#f4b400',
    colorError: '#e22718',
    // 文字链接 = 白（规格 text-link：白字无下划线）
    colorLink: '#ffffff',
    colorLinkHover: '#e6e6e6',
    colorLinkActive: '#bbbbbb',
    // 文字阶梯：ink / body / mute（原文三阶）
    colorText: '#bbbbbb',
    colorTextHeading: '#ffffff',
    colorTextSecondary: '#bbbbbb',
    colorTextTertiary: '#7e7e7e',
    colorTextQuaternary: '#7e7e7e',
    // 画布阶：纯黑页面 / surface-card 容器 / surface-elevated 浮层
    colorBgBase: '#000000',
    colorBgLayout: '#000000',
    colorBgContainer: '#1a1a1a',
    colorBgElevated: '#262626',
    colorBgSpotlight: '#262626',
    // 发丝线双阶：#3c3c3c 分隔 / #262626 一阶抬升边（= surface-elevated 同值）
    colorBorder: '#3c3c3c',
    colorBorderSecondary: '#262626',
    colorSplit: '#262626',
    controlOutline: 'rgba(255, 255, 255, 0.3)',
    // 圆角一律 0：矩形轮廓即品牌（「almost always 0」）
    borderRadius: 0,
    borderRadiusSM: 0,
    borderRadiusLG: 0,
    // 重展示字（700）压轻正文（400）是编辑签名
    fontWeightStrong: 700,
    // BMW Type Next Latin → Helvetica 系 web-safe 近似（禁外部字体）
    fontFamily: '"Helvetica Neue", Helvetica, Arial, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
    fontFamilyCode: '"Cascadia Code", Consolas, monospace',
  },
  components: {
    Button: {
      // 矩形轮廓即品牌：全部按钮 0px（规格「Don't use rounded buttons」）
      borderRadius: 0,
      borderRadiusSM: 0,
      borderRadiusLG: 0,
      // button-primary-outline：透明底 + 白 1px 描边 + 白字（hover 前 无填充）
      defaultBorderColor: '#ffffff',
      defaultColor: '#ffffff',
      defaultBg: 'transparent',
      // button-primary：白底黑字（on-primary）
      primaryColor: '#000000',
      // 按钮字重 700（规格 typography.button 14/700/1.5px，字距见 CSS 片段）
      fontWeight: 700,
      defaultShadow: 'none',
      primaryShadow: 'none',
      dangerShadow: 'none',
    },
    Checkbox: { colorWhite: '#000000' },
    Switch: { handleBg: '#000000', handleShadow: 'none' },
    Steps: { colorWhite: '#000000' },
    Table: {
      headerBg: '#0d0d0d',
      headerColor: '#7e7e7e',
      headerSplitColor: 'transparent',
      headerBorderRadius: 0,
      rowHoverBg: '#262626',
      rowSelectedBg: '#0d0d0d',
      rowSelectedHoverBg: '#262626',
      fixedHeaderSortActiveBg: '#0d0d0d',
      borderColor: '#262626',
    },
    Modal: { contentBg: '#1a1a1a', headerBg: '#1a1a1a', footerBg: '#1a1a1a' },
    Drawer: { colorBgElevated: '#1a1a1a' },
    Tooltip: { colorBgSpotlight: '#262626' },
    Popover: { colorBgElevated: '#1a1a1a' },
    Select: { colorBgContainer: '#1a1a1a', optionSelectedBg: '#262626' },
    Input: { colorBgContainer: '#1a1a1a' },
    InputNumber: { colorBgContainer: '#1a1a1a' },
    Form: { labelColor: '#bbbbbb' },
    Segmented: { trackBg: '#0d0d0d', itemSelectedBg: '#ffffff', itemSelectedColor: '#000000' },
    Tag: { borderRadiusSM: 0 },
    Pagination: { itemActiveBg: 'transparent' },
    Menu: { itemSelectedBg: '#0d0d0d', itemSelectedColor: '#ffffff' },
  },
}
