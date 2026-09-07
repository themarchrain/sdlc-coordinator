import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * voltagent 主题的 antd ThemeConfig —— 自 design/voltagent main.tsx 原样搬移（字面 hex，禁 var()）。
 *
 * Voltagent 设计语言（暗色）：唯一暗面画布 #101010（layout/container/elevated 三面同色，
 * 单连续暗面）；唯一品牌色电光绿 #00d992 仅给 CTA / 状态 pill / live 指示——Button primary
 * 是绿底近黑字（primaryColor #101010）；发丝线 #3d3a39 是唯一边色；文字四阶
 * #f2f2f2 / #bdbdbd / #8b949e + colorTextHeading #ffffff；正文链接走 primary-deep #10b981；
 * 语义暗色适配 success #10b981 / warning #fbbf24 / error #f87171；圆角基线 6。
 * algorithm = darkAlgorithm（暗色算法派生填充 / 禁用态等中间色，无白底残留）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/voltagent.css 的同名令牌手工保持同源：
 *   colorPrimary / colorInfo = --primary      (#00d992)
 *   colorSuccess             = --primary-deep / 源语义 success (#10b981)
 *   colorWarning             = --warning      (#fbbf24)
 *   colorError               = --error        (#f87171)
 *   colorLink                = --link         (#10b981 primary-deep)
 *   colorLinkHover           = --link-hover   (#2fd6a1 primary-soft)
 *   colorText                = --ink          (#f2f2f2)
 *   colorTextHeading         = --ink-strong   (#ffffff)
 *   colorTextSecondary       = --body         (#bdbdbd)
 *   colorTextTertiary/Quaternary = --mute     (#8b949e)
 *   colorBgLayout/Container/Elevated = --canvas (#101010)
 *   colorBorder / colorBorderSecondary = --hairline (#3d3a39)
 *   fontFamily = --font-sans；fontFamilyCode = --font-mono；borderRadius = 6
 */
export const voltagentAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    colorPrimary: '#00d992',
    colorInfo: '#00d992',
    colorSuccess: '#10b981',
    colorWarning: '#fbbf24',
    colorError: '#f87171',
    colorLink: '#10b981',
    colorLinkHover: '#2fd6a1',
    colorText: '#f2f2f2',
    colorTextHeading: '#ffffff',
    colorTextSecondary: '#bdbdbd',
    colorTextTertiary: '#8b949e',
    colorTextQuaternary: '#8b949e',
    colorBgLayout: '#101010',
    colorBgContainer: '#101010',
    colorBgElevated: '#101010',
    colorBorder: '#3d3a39',
    colorBorderSecondary: '#3d3a39',
    borderRadius: 6,
    fontFamily: "'Inter', system-ui, 'Segoe UI', Roboto, 'PingFang SC', 'Microsoft YaHei', sans-serif",
    fontFamilyCode: "'SFMono-Regular', Menlo, Monaco, Consolas, 'Liberation Mono', monospace",
  },
  components: {
    // Button：绿底近黑字（on-primary #101010）、三态阴影全关、600 字重
    Button: { primaryColor: '#101010', primaryShadow: 'none', defaultShadow: 'none', dangerShadow: 'none', fontWeight: 600 },
    // Table：表头 canvas-soft 底 + mute 字、无边框分割线、行 hover = canvas-soft、行边发丝线
    Table: { headerBg: '#1a1a1a', headerColor: '#8b949e', headerSplitColor: 'transparent', rowHoverBg: '#1a1a1a', borderColor: '#3d3a39' },
    // Modal：canvas 面板（重投影由 CSS 侧 --shadow-modal 承担）
    Modal: { contentBg: '#101010', headerBg: '#101010' },
    // Input / InputNumber / Select：输入面 canvas-soft（text-input 规格），选中项绿 12% 洗
    Input: { colorBgContainer: '#1a1a1a' },
    InputNumber: { colorBgContainer: '#1a1a1a' },
    Select: { colorBgContainer: '#1a1a1a', optionSelectedBg: 'rgba(0, 217, 146, 0.12)' },
    // Segmented：白 6% 轨道 + canvas-soft 选中（防暗面同色吞没）
    Segmented: { trackBg: 'rgba(255, 255, 255, 0.06)', itemSelectedBg: '#1a1a1a' },
    Tooltip: { colorBgSpotlight: '#1a1a1a' },
    Message: { contentBg: '#1a1a1a' },
  },
}
