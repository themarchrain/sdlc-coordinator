import type { ThemeConfig } from 'antd'

/**
 * ollama 主题的 antd ThemeConfig —— 自 design/ollama main.tsx 原样搬移。
 *
 * Ollama 设计语言：单色黑 CTA 药丸（colorPrimary #000000，hover #262626 / active #090909）、
 * 语义色 = 终端红绿灯三色 + 中性 charcoal（processing 无蓝）、纯白画布端到端、
 * 发丝线双阶、一切控件 borderRadius 9999 药丸、controlHeight 36、
 * focus ring rgba(59,130,246,.5) 为全系统唯一蓝、浮层海拔只有 1px 发丝线环。
 * Modal/Popover/Dropdown/Alert/Message 以 borderRadiusLG 12 承担卡片例外；
 * 多行 TextArea 的 12px 例外在 css/ollama.css 片段（!important）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/ollama.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary        (#000000)
 *   colorPrimaryHover  = 黑药丸 hover      (#262626，login-primary-active 同源)
 *   colorPrimaryActive = --primary-active (#090909，源 --ink-deep)
 *   colorSuccess/Warning/Error = --terminal-green/yellow/red (#27c93f/#ffbd2e/#ff5f56)
 *   colorInfo          = --info           (#525252，中性 charcoal)
 *   colorText 四级     = ink/charcoal/body/mute (#000000/#525252/#737373/#a3a3a3)
 *   colorBgLayout/Container/Elevated = --canvas/--canvas-card (#ffffff)
 *   colorBorder        = --hairline-strong (#d4d4d4)
 *   colorBorderSecondary = --hairline    (#e5e5e5)
 *   controlOutline     = --focus-ring     (rgba(59,130,246,0.5) 唯一蓝)
 *   boxShadow 三阶     = 1px 发丝线环     (#e5e5e5)
 *   fontFamily         = --font-sans（系统栈）；fontFamilyCode = --font-mono
 */
export const ollamaAntdTheme: ThemeConfig = {
  token: {
    colorPrimary: '#000000',
    colorPrimaryHover: '#262626',
    colorPrimaryActive: '#090909',
    colorInfo: '#525252',
    // colorInfo 为中性炭灰时，antd 会派生实色中灰 Alert 底并压黑字（对比度差、观感发蒙）；
    // 按设计语言钉住为「软面 + 发丝线」：info 提示 = #fafafa 软面 + #e5e5e5 线。
    colorInfoBg: '#fafafa',
    colorInfoBorder: '#e5e5e5',
    colorSuccess: '#27c93f',
    colorWarning: '#ffbd2e',
    colorError: '#ff5f56',
    colorLink: '#000000',
    colorLinkHover: '#525252',
    colorText: '#000000',
    colorTextSecondary: '#525252',
    colorTextTertiary: '#737373',
    colorTextQuaternary: '#a3a3a3',
    colorTextDisabled: '#a3a3a3',
    colorBgLayout: '#ffffff',
    colorBgContainer: '#ffffff',
    colorBgElevated: '#ffffff',
    colorBgContainerDisabled: '#fafafa',
    colorBorder: '#d4d4d4',
    colorBorderSecondary: '#e5e5e5',
    borderRadius: 9999,
    controlHeight: 36,
    controlOutline: 'rgba(59,130,246,0.5)',
    controlOutlineWidth: 3,
    boxShadow: '0 0 0 1px #e5e5e5',
    boxShadowSecondary: '0 0 0 1px #e5e5e5',
    boxShadowTertiary: '0 0 0 1px #e5e5e5',
    fontFamily: "ui-sans-serif, system-ui, -apple-system, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif",
    fontFamilyCode: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace",
  },
  components: {
    Button: { borderRadius: 9999, borderRadiusSM: 9999, borderRadiusLG: 9999, primaryShadow: 'none', defaultShadow: 'none', dangerShadow: 'none', fontWeight: 500 },
    Input: { borderRadius: 9999, borderRadiusSM: 9999, borderRadiusLG: 9999, activeShadow: '0 0 0 3px rgba(59,130,246,0.5)' },
    InputNumber: { borderRadius: 9999, activeShadow: '0 0 0 3px rgba(59,130,246,0.5)' },
    Select: { borderRadius: 9999, optionSelectedBg: '#fafafa' },
    Tag: { borderRadiusSM: 9999 },
    Segmented: { trackBg: '#fafafa', itemSelectedBg: '#ffffff', itemSelectedColor: '#000000' },
    Table: { headerBg: '#fafafa', headerColor: '#525252', rowHoverBg: '#fafafa', borderColor: '#e5e5e5', headerSplitColor: 'transparent' },
    Modal: { borderRadiusLG: 12 },
    Popover: { borderRadiusLG: 12 },
    Dropdown: { borderRadiusLG: 12 },
    Alert: { borderRadiusLG: 12 },
    Message: { borderRadiusLG: 12 },
    Tooltip: { borderRadiusSM: 6 },
  },
}
