import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * revolut 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文（字面 hex，禁 var()）。
 *
 * Revolut 设计语言（纯黑叙事画布）：
 * - 暗面阶梯只有两级：canvas-dark #000000（真黑，规格 Don't「never near-black」）
 *   → surface-deep #0a0a0a（黑域内嵌面）→ surface-elevated #16181a（计划卡/面板，
 *   「slightly luminous, lifts plan cards off the black canvas」）；
 * - 品牌色 cobalt-violet #494fdf：规格令牌表 primary，只作「deliberate stamp」
 *   （特色卡 / 品牌图形 / 主操作），hover 提亮 #4f55f1、按压回落 #3a40c4；
 * - 白药丸是黑画布上的「loudest action」（规格 hero CTA）——管理台里保留给
 *   登录页 CTA（css 令牌 --login-primary + 专属片段把按钮文字压成黑），
 *   应用内主操作走 cobalt（规格「cobalt for secondary CTAs」的暗面对应物）；
 * - 语义/产品宽色盘（teal/light-blue/warning orange/danger red）按规格只落在
 *   「产品图形内部」——本主题把它们用于图表描线与状态（图形语汇），不做按钮面；
 * - 无投影语言（规格「no traditional drop-shadow language」，海拔靠明度阶切换）；
 * - 全部按钮药丸化 999px（规格「every button and pill」）；控件 12px、卡 20px；
 * - 字体：正文/按钮/标签 Inter（规格「never substitute Aeonik Pro for body type」）。
 * algorithm = darkAlgorithm（暗色算法派生填充/禁用态等中间色，照 spotify 暗色范例模式）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/revolut.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary        (#494fdf 规格 primary)
 *   colorPrimaryHover  = --link-hover     (#4f55f1 primary-bright，规格「inline link colour」)
 *   colorPrimaryActive = --primary-active (#3a40c4 primary-deep，规格「active/pressed state」)
 *   colorInfo          = --info           (#007BC2 accent-light-blue)
 *   colorSuccess       = --success        (#00A87E accent-teal，暗面成功色推导)
 *   colorWarning       = --warning        (#EC7E00 accent-warning)
 *   colorError         = --error          (#E23B4A accent-danger)
 *   colorLink          = --link           (#FFFFFF 规格：暗域链接靠 700 字重持对比，不换色)
 *   colorText 四阶      = 白 / faint #C9C9CD / stone #8D969E / 推导 #6F757D
 *   colorBgLayout      = --canvas         (#000000)
 *   colorBgContainer   = --canvas-card    (#16181A surface-elevated)
 *   colorBgElevated    = --canvas-card    (#16181A) / Spotlight = --tooltip-bg (#0A0A0A)
 *   colorBorder        = charcoal #3A3D40（控件边）/ Secondary #242629 ≈ hairline-dark 实底
 *   borderRadius 12    = 规格 rounded.md（inputs / download tiles）
 *   fontFamily         = --font-sans（Inter 系统栈，Aeonik Pro 只进 --font-display）
 */
export const revolutAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    // cobalt-violet 一族：base #494FDF → hover 提亮 #4F55F1 → 按压回落 #3A40C4
    colorPrimary: '#494fdf',
    colorPrimaryHover: '#4f55f1',
    colorPrimaryActive: '#3a40c4',
    // 语义色（产品宽色盘，暗面上用于状态而非按钮面）
    colorInfo: '#007bc2',
    colorSuccess: '#00a87e',
    colorWarning: '#ec7e00',
    colorError: '#e23b4a',
    // 暗域链接 = 白（规格 Typography：「Inline links inside dark photo regions step up
    // to weight 700 so they hold contrast without using the cobalt accent」）
    colorLink: '#ffffff',
    colorLinkHover: '#4f55f1',
    colorLinkActive: '#4f55f1',
    // 文字阶梯：on-dark 白 / faint / stone 三级 + 推导最暗一档
    colorText: '#ffffff',
    colorTextHeading: '#ffffff',
    colorTextSecondary: '#c9c9cd',
    colorTextTertiary: '#8d969e',
    colorTextQuaternary: '#6f757d', // 推导：stone 压暗一档（规格暗面文字只到 stone 为止）
    // cobalt 底上文字 = on-primary 白（规格令牌）
    colorTextLightSolid: '#ffffff',
    // 表面阶梯：画布真黑 / 容器 surface-elevated / 浮层同阶（规格阶梯只有两级）
    colorBgBase: '#000000',
    colorBgLayout: '#000000',
    colorBgContainer: '#16181a',
    colorBgElevated: '#16181a',
    colorBgSpotlight: '#0a0a0a',
    // 边线双阶：charcoal（规格中性阶，恰为暗底控件边明度）/ hairline-dark 实底近似
    colorBorder: '#3a3d40',
    colorBorderSecondary: '#242629',
    colorSplit: '#1f2124',
    controlOutline: 'rgba(73, 79, 223, .4)',
    // 圆角刻度：小件 8 / 控件 12 / 卡 20（规格 rounded.sm / rounded.md / rounded.lg）
    borderRadius: 12,
    borderRadiusSM: 8,
    borderRadiusLG: 20,
    // 海拔 = 无投影（规格 Elevation：「no traditional drop-shadow language」）
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // 字体：Inter 承担全部 UI 角色（规格 Do：「Use Inter for body, button labels, captions」）
    fontFamily:
      'Inter, system-ui, -apple-system, "Segoe UI", Arial, "PingFang SC", "Microsoft YaHei", sans-serif',
    fontFamilyCode: '"Cascadia Code", Consolas, monospace',
    // 字重只有 400/600（规格 Don't：「Use 400 (default) or 600 (emphatic) — never the in-between」）
    fontWeightStrong: 600,
  },
  components: {
    Button: {
      // 药丸是品牌交互形状：全部按钮 999px（规格 Shapes「Buttons, pills, badges, tabs」）
      borderRadius: 999,
      borderRadiusSM: 999,
      borderRadiusLG: 999,
      // 按钮标签 Inter 600（规格 button-md「16px / 600 / 0.24px」）
      fontWeight: 600,
      primaryColor: '#ffffff',
      // 默认按钮 = button-outline-dark 的克制版：黑底白字 + stone 边（规格原版 1px 全白边
      // 过响，取 stone #8D969E，hover 提到纯白）
      defaultBg: '#000000',
      defaultColor: '#ffffff',
      defaultBorderColor: '#8d969e',
      defaultHoverBg: '#0a0a0a',
      defaultHoverColor: '#ffffff',
      defaultHoverBorderColor: '#ffffff',
      // 按钮不带投影（海拔留给明度阶）
      defaultShadow: 'none',
      primaryShadow: 'none',
      dangerShadow: 'none',
    },
    Checkbox: { colorWhite: '#ffffff' },
    Steps: { colorWhite: '#ffffff' },
    Table: {
      // 扁平表头：内嵌面 #0A0A0A + faint 标签（黑域 well 语汇），hover 行提亮一档
      headerBg: '#0a0a0a',
      headerColor: '#c9c9cd',
      headerSplitColor: 'transparent',
      rowHoverBg: '#1d2023',
      rowSelectedBg: '#202536',
      rowSelectedHoverBg: '#242a3f',
      fixedHeaderSortActiveBg: '#1d2023',
      borderColor: '#242629',
    },
    Modal: { contentBg: '#16181a', headerBg: '#16181a', footerBg: '#16181a' },
    Drawer: { colorBgElevated: '#16181a' },
    Tooltip: { colorBgSpotlight: '#0a0a0a' },
    Popover: { colorBgElevated: '#16181a' },
    Select: {
      colorBgContainer: '#0a0a0a',
      optionSelectedBg: '#1b1e39', // 推导：cobalt veil rgba(73,79,223,.25) 落在黑上的实底
    },
    Input: { colorBgContainer: '#0a0a0a' },
    InputNumber: { colorBgContainer: '#0a0a0a' },
    Form: { labelColor: '#ffffff' },
    Segmented: { trackBg: '#0a0a0a', itemSelectedBg: '#242629' },
    Tag: { borderRadiusSM: 999 },
    Pagination: { itemActiveBg: 'transparent' },
  },
}
