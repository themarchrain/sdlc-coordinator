import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * resend 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现；resend 规格为 frontmatter `---` 令牌表格式，色值直接取表（字面 hex，禁 var()）。
 *
 * Resend 设计语言（编辑部式开发者工具，暗色）：
 * - 真黑画布 #000000（规格 Don't「The brand sits on #000000, not #0a0a0a」），
 *   面阶梯 surface-card #0a0a0c → surface-elevated #101012 → surface-deep #06060a
 *   （比画布更深、偏冷的 code well）；
 * - primary = 白 #fcfdff（规格「White is the brand on black」）：白底黑字小 CTA
 *   是画布上最亮的像素，按压转 surface-light #f1f7fe；
 * - 深度全部由半透明白发丝线承担（hairline 6% / hairline-strong 14% /
 *   divider-soft 4%），零投影语言；
 * - 六个 accent（橙/黄/蓝/绿/红）只作低透明度大气辉光与行内高亮，
 *   蓝 #3b9eff 兼链接/信息、绿 #11ff99 兼成功状态点、红 #ff2047 兼行内错误；
 * - 圆角词汇：按钮/输入 8px（rounded.md）、卡/代码井 12px（rounded.lg）、pill 9999；
 * - 字体：Inter（UI）+ ABC Favorit（正文）+ Domaine Display（编辑部衬线显示，
 *   骨架应用见 resend.css 片段）+ Geist Mono（代码）——禁外部加载，写近似栈；
 * - 字重纪律：正文 400、UI 标题最多 500（规格 Don't「Don't bump body weight to
 *   600 for emphasis」→ fontWeightStrong 500）。
 * algorithm = darkAlgorithm（暗色算法派生填充/禁用态等中间色，照 spotify 暗色范例）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/resend.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary        (#FCFDFF，规格 primary「brand's de facto accent」)
 *   colorPrimaryActive = --primary-active (#F1F7FE surface-light，规格 button-primary-pressed)
 *   colorInfo          = --info           (#3B9EFF accent-blue / link)
 *   colorSuccess       = --success        (#11FF99 accent-green「success status dots」)
 *   colorWarning       = --warning        (#FFC53D accent-yellow)
 *   colorError         = --error          (#FF2047 accent-red「inline error red」)
 *   colorLink          = --link           (#3B9EFF，规格 link 与 accent-blue 同值)
 *   colorText 四阶      = ink #FCFDFF / charcoal 70% / mute #A1A4A5 / ash #888E90
 *   colorBgLayout      = --canvas         (#000000)
 *   colorBgContainer   = --canvas-card    (#0A0A0C surface-card)
 *   colorBgElevated    = --tooltip-bg     (#101012 surface-elevated)
 *   colorBorder        = hairline-strong  (rgba(255,255,255,.14) 规格「structural 1px border」)
 *   borderRadius 8     = 规格 rounded.md（按钮/输入）；卡 12px 在 borderRadiusLG
 *   boxShadow none     = 规格「no traditional drop shadow language」
 *   fontFamily         = --font-sans（ABC Favorit → Inter 近似栈）
 */
export const resendAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    // 白即品牌：主按钮 = 白底黑字（画布上最亮的像素）
    colorPrimary: '#FCFDFF',
    colorPrimaryHover: '#FFFFFF',
    colorPrimaryActive: '#F1F7FE',
    // 语义色全部取规格 accent 表：蓝（信息/链接）/ 绿（成功）/ 黄（警示）/ 红（错误）
    colorInfo: '#3B9EFF',
    colorSuccess: '#11FF99',
    colorWarning: '#FFC53D',
    colorError: '#FF2047',
    // 链接 = accent-blue（规格 colors.link 与 accent-blue 同值；hover 为提亮一档推导值）
    colorLink: '#3B9EFF',
    colorLinkHover: '#5FB0FF',
    colorLinkActive: '#5FB0FF',
    // 文字阶梯：ink / charcoal / mute / ash（规格 Text 四级）
    colorText: '#FCFDFF',
    colorTextHeading: '#FCFDFF',
    colorTextSecondary: 'rgba(252, 253, 255, 0.7)',
    colorTextTertiary: '#A1A4A5',
    colorTextQuaternary: '#888E90',
    // 白底黑字（规格 primary-on「Black text on white pill is the brand's CTA pattern」）
    colorTextLightSolid: '#000000',
    // 表面阶梯：画布真黑 / 卡 surface-card / 浮层 surface-elevated
    colorBgBase: '#000000',
    colorBgLayout: '#000000',
    colorBgContainer: '#0A0A0C',
    colorBgElevated: '#101012',
    colorBgSpotlight: '#101012',
    // 边框 = 半透明白发丝线双轨（hairline-strong 结构线 / hairline 6% 软线）
    colorBorder: 'rgba(255, 255, 255, 0.14)',
    colorBorderSecondary: 'rgba(255, 255, 255, 0.06)',
    colorSplit: 'rgba(255, 255, 255, 0.06)',
    // focus：规格「Focus state thickens the border to ink (no separate ring colour)」→ 白环
    controlOutline: 'rgba(252, 253, 255, 0.25)',
    // 圆角词汇：按钮/输入 8px（rounded.md）；小件 6px；卡 12px（rounded.lg）
    borderRadius: 8,
    borderRadiusSM: 6,
    borderRadiusLG: 12,
    // 零投影：深度由半透明白发丝线承担（规格 Elevation「no drop shadow language」）
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // ABC Favorit（正文）→ Inter（UI）近似栈 + Geist Mono 代码，禁外部字体
    fontFamily:
      "'ABC Favorit', Inter, system-ui, -apple-system, 'Segoe UI', 'Helvetica Neue', Arial, 'PingFang SC', 'Microsoft YaHei', sans-serif",
    fontFamilyCode: '"Geist Mono", "Cascadia Code", Consolas, monospace',
    // 字重纪律：正文 400、标题最多 500（规格 Don't「Don't bump body weight to 600」）
    fontWeightStrong: 500,
  },
  components: {
    Button: {
      // 按钮 8px（rounded.md）+ Inter 500（规格 button-md）
      borderRadius: 8,
      borderRadiusSM: 8,
      borderRadiusLG: 8,
      fontWeight: 500,
      // 白底黑字主 CTA
      primaryColor: '#000000',
      // 默认按钮 = button-ghost：surface-elevated 底 + hairline-strong 边（hover 提亮一档为推导值）
      defaultBg: '#101012',
      defaultColor: '#FCFDFF',
      defaultBorderColor: 'rgba(255, 255, 255, 0.14)',
      defaultHoverBg: '#16161A',
      defaultHoverColor: '#FCFDFF',
      defaultHoverBorderColor: 'rgba(255, 255, 255, 0.22)',
      defaultShadow: 'none',
      primaryShadow: 'none',
      dangerShadow: 'none',
    },
    Checkbox: { colorWhite: '#000000' },
    Steps: { colorWhite: '#000000' },
    Table: {
      // 表头 = surface-elevated 条 + mute 标签；行 hover = divider 白纱
      headerBg: '#101012',
      headerColor: '#A1A4A5',
      headerSplitColor: 'rgba(255, 255, 255, 0.06)',
      rowHoverBg: 'rgba(255, 255, 255, 0.04)',
      rowSelectedBg: 'rgba(255, 255, 255, 0.06)',
      rowSelectedHoverBg: 'rgba(255, 255, 255, 0.08)',
      fixedHeaderSortActiveBg: 'rgba(255, 255, 255, 0.04)',
      borderColor: 'rgba(255, 255, 255, 0.06)',
    },
    Modal: { contentBg: '#101012', headerBg: '#101012', footerBg: '#101012' },
    Drawer: { colorBgElevated: '#101012' },
    Tooltip: { colorBgSpotlight: '#101012' },
    Popover: { colorBgElevated: '#101012' },
    Select: { colorBgContainer: '#0A0A0C', optionSelectedBg: 'rgba(255, 255, 255, 0.08)' },
    Input: { colorBgContainer: '#0A0A0C' },
    InputNumber: { colorBgContainer: '#0A0A0C' },
    Form: { labelColor: '#FCFDFF' },
    Segmented: { trackBg: '#101012', itemSelectedBg: 'rgba(255, 255, 255, 0.12)' },
    Tag: { borderRadiusSM: 6 }, // 代码标签/chip = rounded.sm 6px
    Pagination: { itemActiveBg: 'transparent' },
  },
}
