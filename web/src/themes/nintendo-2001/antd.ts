import type { ThemeConfig } from 'antd'

/**
 * nintendo-2001 主题的 antd ThemeConfig —— 全字面 hex，禁 var()。
 *
 * Nintendo.com 2001 设计语言（Y2K 主机面板，亮色极性）：
 * - 暖色只表达「前进/提交」：colorPrimary = Signal Orange（button-submit 角色），
 *   active 压向 Nav Gold（button-primary-pressed 的加深方向）；Antd 主按钮 = 白字橙底，
 *   正是规格 button-submit 的原样组合。
 * - 链接与结构强调是 Chrome Indigo（规格 link = ink-soft #3d4f97，非红非橙）。
 * - Error 复用 Nintendo Red（规格 error 与品牌红同值）；
 *   Success 推导为 Systems Teal（规格内唯一冷青，电路板 hero 域）；
 *   Warning 为 Amber 压深的金棕（#ecab37 在白面作文字不可读）。
 * - 深度 = 硬斜切阴影：boxShadow 双阶取零模糊的铬靛硬偏移（规格禁软投影）。
 * - 圆角收紧：2px（规格 rounded.xs）/ 6px（rounded.md），锋利面板性格。
 * - algorithm 未指定 = 默认亮色算法（本主题为亮色，见 index.ts isDark 说明）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/nintendo-2001.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary        (#f68d1f signal)
 *   colorPrimaryActive = --primary-active (#e48600 nav-gold)
 *   colorInfo          = --info           (#3d4f97 chrome-indigo)
 *   colorInfoBg/Border = 软面 #eef3fa + 发丝线 #9fbee7（显式钉住：indigo 近中性，
 *                        antd 会派生灰蒙 Alert 底，先例见 themes/cursor/antd.ts）
 *   colorLink 三阶     = --link/--link-hover (#3d4f97 / #2e3c77)
 *   colorText 四阶     = ink #21242e / ink-soft #3d4f97 / muted #60619c / faint #767ba6
 *   colorBgLayout      = --canvas         (#7a8aba 拉丝佩瑞汪金属)
 *   colorBgContainer/Elevated = --canvas-card (#ffffff)
 *   colorBorder        = --hairline       (#5a5f8c)
 *   colorBorderSecondary/colorSplit = #a8b3d6（淡佩瑞汪发丝线）
 *   fontFamily         = --font-sans（Arial 系统栈）；fontFamilyCode = --font-mono（Courier New）
 */
export const nintendo2001ThemeConfig: ThemeConfig = {
  token: {
    // 前进信号：Signal Orange 只给主 CTA；active 压向 Nav Gold
    colorPrimary: '#f68d1f',
    colorPrimaryHover: '#f8a04f',
    colorPrimaryActive: '#e48600',
    // 结构 info = Chrome Indigo；Bg/Border 钉为「软面 + 发丝线」，防灰蒙 Alert 底
    colorInfo: '#3d4f97',
    colorInfoBg: '#eef3fa',
    colorInfoBorder: '#9fbee7',
    // 链接 = 规格 ink-soft（Chrome Indigo Bold 的链接语法）
    colorLink: '#3d4f97',
    colorLinkHover: '#2e3c77',
    colorLinkActive: '#2e3c77',
    // 语义色：teal 推导 success / 金棕 warning / 品牌红 error
    colorSuccess: '#206479',
    colorWarning: '#8c5c00',
    colorError: '#e60012',
    // 文字阶梯：carbon / chrome-indigo / muted-indigo / faint
    colorText: '#21242e',
    colorTextHeading: '#21242e',
    colorTextSecondary: '#3d4f97',
    colorTextTertiary: '#60619c',
    colorTextQuaternary: '#767ba6',
    // 表面：佩瑞汪画布 + 白色面板
    colorBgLayout: '#7a8aba',
    colorBgContainer: '#ffffff',
    colorBgElevated: '#ffffff',
    colorBgSpotlight: '#21242e',
    // 边框双阶： blended bevel divider + 淡发丝线
    colorBorder: '#5a5f8c',
    colorBorderSecondary: '#a8b3d6',
    colorSplit: '#a8b3d6',
    // 输入聚焦外环 = signal 暖纱（「在此行动」）
    controlOutline: 'rgba(246, 141, 31, .4)',
    // 圆角：规格 rounded.xs(2) / rounded.md(6)
    borderRadius: 2,
    borderRadiusSM: 2,
    borderRadiusLG: 6,
    // 深度 = 硬斜切：零模糊铬靛偏移阴影（规格 Don't：软模糊投影）
    boxShadow: '3px 3px 0 rgba(61, 79, 151, .3)',
    boxShadowSecondary: '4px 4px 0 rgba(61, 79, 151, .26)',
    // 字体：全系统栈（规格：Arial / Arial Black / 时代等宽 Courier New，无外部字体）
    fontFamily: 'Arial, Helvetica, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
    fontFamilyCode: '"Courier New", Consolas, monospace',
  },
  components: {
    Button: {
      // ui-label 语法：Arial Bold；阴影关闭（斜面高光见 css 专属片段）
      fontWeight: 700,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Table: {
      // 表头 = Pale Sky 次级导航条面 + carbon 墨（uppercase 见 css 片段）
      headerBg: '#9fbee7',
      headerColor: '#21242e',
      headerSplitColor: 'transparent',
      rowHoverBg: '#eef3fa',
      borderColor: '#a8b3d6',
    },
    Tooltip: { colorBgSpotlight: '#21242e' },
    Segmented: { trackBg: '#dedede', itemSelectedBg: '#ffffff' },
    Tag: { borderRadiusSM: 2 },
  },
}
